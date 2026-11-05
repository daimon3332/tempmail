package server

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"log"
	"net/http"
	"regexp"
	"strings"
	"sync"
	"time"

	"tempmail/internal/db"
	"tempmail/internal/icloud"
	"tempmail/internal/mailparse"
)

// iCloud Hide My Email: aliases are ordinary rows in `address` so every
// mailbox feature applies; hme_aliases mirrors Apple's list and inbound mail
// arrives through the account's forward-to address on one of our domains.

var (
	hmeLocks  sync.Map // account id -> *sync.Mutex
	hmeLogins sync.Map // login id -> *hmeLoginState
)

type hmeLoginState struct {
	login     *icloud.Login
	accountID int64
	name      string
	host      string
	proxy     string
	expires   time.Time
}

var (
	errHMENotFound = errors.New("iCloud account not found")
	errHMECooldown = errors.New("all iCloud accounts are cooling down")
)

func (a *App) hmeRoutes() {
	m := a.mux
	m.HandleFunc("GET /admin/hme/accounts", a.hmeListAccounts)
	m.HandleFunc("POST /admin/hme/accounts", a.hmeCreateAccount)
	m.HandleFunc("PATCH /admin/hme/accounts/{id}", a.hmeUpdateAccount)
	m.HandleFunc("PUT /admin/hme/accounts/{id}/cookies", a.hmeUpdateCookies)
	m.HandleFunc("DELETE /admin/hme/accounts/{id}", a.hmeDeleteAccount)
	m.HandleFunc("POST /admin/hme/accounts/{id}/sync", a.hmeSyncAccount)
	m.HandleFunc("POST /admin/hme/accounts/{id}/forward", a.hmeSetForward)
	m.HandleFunc("POST /admin/hme/relay", a.hmeCreateRelay)
	m.HandleFunc("POST /admin/hme/login", a.hmeLoginStart)
	m.HandleFunc("POST /admin/hme/login/{id}/sms", a.hmeLoginSMS)
	m.HandleFunc("POST /admin/hme/login/{id}/code", a.hmeLoginCode)
	m.HandleFunc("GET /admin/hme/aliases", a.hmeListAliases)
	m.HandleFunc("POST /admin/hme/aliases", a.hmeCreateAlias)
	m.HandleFunc("PATCH /admin/hme/aliases/{email}", a.hmeUpdateAlias)
	m.HandleFunc("POST /admin/hme/aliases/{email}/deactivate", a.hmeToggleAlias)
	m.HandleFunc("POST /admin/hme/aliases/{email}/reactivate", a.hmeToggleAlias)
	m.HandleFunc("DELETE /admin/hme/aliases/{email}", a.hmeDeleteAlias)
	m.HandleFunc("POST /admin/hme/aliases/{email}/pin", a.hmePinAlias)
	m.HandleFunc("GET /admin/hme/recycle", a.hmeGetRecycle)
	m.HandleFunc("POST /admin/hme/recycle", a.hmeSaveRecycle)
}

// ---- inbound routing ----

var appleDKIMPass = regexp.MustCompile(`(?i)\bdkim=pass\b[^;]*\bheader\.d=icloud\.com\b`)

// firstHeader returns the topmost occurrence, i.e. the one added by the last
// hop (Cloudflare) rather than anything the sender injected below it.
func firstHeader(m *mailparse.Mail, key string) string {
	for _, h := range m.Headers {
		if strings.EqualFold(h.Key, key) {
			return h.Value
		}
	}
	return ""
}

// RouteHME accepts mail only when Cloudflare verified Apple's DKIM signature,
// it reached an account's forward-to address, the header names that same
// target, and the alias has a mailbox; anything else stays in the forward-to
// mailbox. Only the ingest path calls it, so mail
// injected straight into port 25 is never routed.
func (a *App) RouteHME(ctx context.Context, to string, parsed *mailparse.Mail) (string, string, bool) {
	if parsed == nil || !appleDKIMPass.MatchString(firstHeader(parsed, "Authentication-Results")) {
		return "", "", false
	}
	v := firstHeader(parsed, "X-Icloud-Hme")
	if v == "" {
		return "", "", false
	}
	h := icloud.ParseHMEHeader(v)
	to = strings.ToLower(strings.TrimSpace(to))
	if h.Alias == "" || h.ForwardTo != to {
		return "", "", false
	}
	n, _ := a.db.Count(ctx, `SELECT count(*) FROM hme_aliases h JOIN hme_accounts acc ON acc.id = h.account_id
		JOIN address ad ON ad.name = h.email WHERE h.email = ? AND lower(acc.forward_to) = ?`, h.Alias, to)
	if n == 0 {
		return "", "", false
	}
	_, _ = a.db.Exec(ctx, `UPDATE hme_aliases SET last_mail_at = datetime('now'), pooled = 0 WHERE email = ?`, h.Alias)
	return h.Alias, h.Sender, true
}

// ---- account session plumbing ----

func (a *App) sealCookies(c map[string]string) (string, error) {
	b, err := json.Marshal(c)
	if err != nil {
		return "", err
	}
	return a.encryptUserPassword(string(b))
}

// hmeRun executes fn with a client for the account, serialised per account,
// then persists refreshed cookies and the resulting session status.
func (a *App) hmeRun(ctx context.Context, id int64, fn func(*icloud.Client) error) error {
	mu, _ := hmeLocks.LoadOrStore(id, &sync.Mutex{})
	mu.(*sync.Mutex).Lock()
	defer mu.(*sync.Mutex).Unlock()
	row, err := a.db.QueryOne(ctx, `SELECT * FROM hme_accounts WHERE id = ?`, id)
	if err != nil {
		return err
	}
	if row == nil {
		return errHMENotFound
	}
	cookies := map[string]string{}
	if enc := row.Str("cookies"); enc != "" {
		plain, err := a.decryptUserPassword(enc)
		if err != nil {
			return errors.New("stored iCloud cookies cannot be decrypted; sign in again")
		}
		_ = json.Unmarshal([]byte(plain), &cookies)
	}
	if len(cookies) == 0 {
		return errors.New("no iCloud session; sign in or import cookies first")
	}
	c, err := icloud.NewClient(cookies, row.Str("host"), row.Str("proxy"))
	if err != nil {
		return err
	}
	c.ServiceURL, c.DSID = row.Str("service_url"), row.Str("dsid")
	err = fn(c)
	status, lastErr := row.Str("status"), ""
	switch {
	case err == nil:
		status = "active"
	case errors.Is(err, icloud.ErrUnauthorized):
		status, lastErr = "expired", "iCloud session expired; sign in again or update cookies"
		log.Printf("hme: account %d session expired", id)
		if row.Str("status") != "expired" && a.tg != nil {
			msg := fmt.Sprintf("iCloud 账号「%s」登录已失效，自动创建已暂停（已有别名照常收信）。请在 %s/icloud 重新登录或更新 Cookie。", row.Str("name"), a.origin())
			go a.tg.NotifyAdmins(context.Background(), msg)
		}
	default:
		lastErr = err.Error()
	}
	enc, encErr := a.sealCookies(c.Cookies())
	if encErr != nil {
		return encErr
	}
	_, _ = a.db.Exec(ctx, `UPDATE hme_accounts SET cookies = ?, service_url = ?, dsid = ?, status = ?, last_error = ?, updated_at = datetime('now') WHERE id = ?`,
		enc, c.ServiceURL, c.DSID, status, lastErr, id)
	return err
}

func (a *App) hmeSync(ctx context.Context, id int64) error {
	return a.hmeRun(ctx, id, func(c *icloud.Client) error {
		acc, err := c.Validate(ctx)
		if err != nil {
			return err
		}
		list, err := c.List(ctx)
		if err != nil {
			return err
		}
		opts, _ := json.Marshal(list.ForwardOptions)
		if _, err := a.db.Exec(ctx, `UPDATE hme_accounts SET apple_id = ?, forward_to = ?, forward_options = ?,
			last_validated_at = datetime('now'), last_synced_at = datetime('now') WHERE id = ?`,
			acc.AppleID, strings.ToLower(list.ForwardTo), string(opts), id); err != nil {
			return err
		}
		return a.hmeStoreAliases(ctx, id, list.Aliases)
	})
}

func (a *App) hmeKeepAlive(ctx context.Context, id int64) error {
	return a.hmeRun(ctx, id, func(c *icloud.Client) error {
		if _, err := c.Validate(ctx); err != nil {
			return err
		}
		_, err := a.db.Exec(ctx, `UPDATE hme_accounts SET last_validated_at = datetime('now') WHERE id = ?`, id)
		return err
	})
}

func appleTime(t time.Time) any {
	if t.IsZero() {
		return nil
	}
	return t.UTC().Format("2006-01-02 15:04:05")
}

const hmeUpsert = `INSERT INTO hme_aliases(email, account_id, anonymous_id, label, note, active, pooled, apple_created_at) VALUES(?, ?, ?, ?, ?, ?, ?, ?)
	ON CONFLICT(email) DO UPDATE SET account_id = excluded.account_id, anonymous_id = excluded.anonymous_id,
	label = excluded.label, note = excluded.note, active = excluded.active, apple_created_at = excluded.apple_created_at`

// hmeLinkAll gives every known alias a mailbox so it receives mail as soon as
// it exists, including aliases created outside tempmail.
func (a *App) hmeLinkAll(ctx context.Context) error {
	_, err := a.db.Exec(ctx, `INSERT OR IGNORE INTO address(name, source_meta) SELECT email, 'icloud-hme' FROM hme_aliases`)
	return err
}

// hmeStoreAliases mirrors Apple's list for one account ("unused" flags survive).
func (a *App) hmeStoreAliases(ctx context.Context, id int64, aliases []icloud.Alias) error {
	tx, err := a.db.BeginTx(ctx, nil)
	if err != nil {
		return err
	}
	defer tx.Rollback()
	keep := map[string]bool{}
	for _, al := range aliases {
		keep[al.Email] = true
		if _, err := tx.ExecContext(ctx, hmeUpsert, al.Email, id, al.AnonymousID, al.Label, al.Note, boolInt(al.Active), 0, appleTime(al.CreatedAt)); err != nil {
			return err
		}
	}
	rows, err := tx.QueryContext(ctx, `SELECT email FROM hme_aliases WHERE account_id = ?`, id)
	if err != nil {
		return err
	}
	var gone []string
	for rows.Next() {
		var e string
		if rows.Scan(&e) == nil && !keep[e] {
			gone = append(gone, e)
		}
	}
	rows.Close()
	for _, e := range gone {
		if _, err := tx.ExecContext(ctx, `DELETE FROM hme_aliases WHERE email = ?`, e); err != nil {
			return err
		}
	}
	if err := tx.Commit(); err != nil {
		return err
	}
	return a.hmeLinkAll(ctx)
}

func (a *App) hmeReserve(ctx context.Context, id int64, label, note string, pooled bool) (icloud.Alias, error) {
	var out icloud.Alias
	err := a.hmeRun(ctx, id, func(c *icloud.Client) error {
		al, err := c.Create(ctx, label, note)
		if err != nil {
			return err
		}
		out = al
		if _, err = a.db.Exec(ctx, hmeUpsert, al.Email, id, al.AnonymousID, al.Label, al.Note, 1, boolInt(pooled), appleTime(al.CreatedAt)); err != nil {
			return err
		}
		if _, err = a.db.Exec(ctx, `UPDATE hme_aliases SET managed = 1 WHERE email = ?`, al.Email); err != nil {
			return err
		}
		_, err = a.hmeEnsureAddress(ctx, al.Email, "icloud-hme")
		return err
	})
	return out, err
}

// hmeCreateOn reserves a new alias. A full account records its observed cap;
// on-demand creation (never pool refill) may then recycle one stale alias and
// retry. Throttle responses put the account into cooldown.
func (a *App) hmeCreateOn(ctx context.Context, id int64, label, note string, pooled bool) (icloud.Alias, error) {
	out, err := a.hmeReserve(ctx, id, label, note, pooled)
	if icloud.IsQuotaFull(err) {
		_, _ = a.db.Exec(ctx, `UPDATE hme_accounts SET observed_limit = (SELECT count(*) FROM hme_aliases WHERE account_id = ?) WHERE id = ?`, id, id)
		if !pooled && a.hmeRecycleSettings(ctx).Enabled {
			if victim, rerr := a.hmeRecycleOne(ctx, id); rerr != nil {
				log.Printf("hme account %d recycle: %v", id, rerr)
			} else if victim != "" {
				out, err = a.hmeReserve(ctx, id, label, note, pooled)
			}
		}
	}
	switch {
	case err == nil:
		_, _ = a.db.Exec(ctx, `UPDATE hme_accounts SET throttle_streak = 0 WHERE id = ?`, id)
	case icloud.IsRateLimited(err):
		_, _ = a.db.Exec(ctx, hmeThrottleSQL, id)
	case icloud.IsQuotaFull(err):
		_, _ = a.db.Exec(ctx, `UPDATE hme_accounts SET cooldown_until = datetime('now', '+1 hour') WHERE id = ?`, id)
	}
	return out, err
}

// hmeThrottleSQL backs off exponentially (30, 60, 120, 240 min): Apple's
// window is not a fixed 30 minutes, so re-hitting it on schedule is avoided.
const hmeThrottleSQL = `UPDATE hme_accounts SET throttle_streak = min(throttle_streak + 1, 4),
	cooldown_until = datetime('now', '+' || (30 << min(throttle_streak, 3)) || ' minutes') WHERE id = ?`

type hmeRecycleConfig struct {
	Enabled    bool `json:"enabled"`
	UnusedDays int  `json:"unusedDays"`
	IdleDays   int  `json:"idleDays"`
}

func (a *App) hmeRecycleSettings(ctx context.Context) hmeRecycleConfig {
	s := hmeRecycleConfig{Enabled: true, UnusedDays: 30, IdleDays: 90}
	a.jsonSetting(ctx, "hme_recycle_settings", &s)
	if s.UnusedDays < 1 {
		s.UnusedDays = 30
	}
	if s.IdleDays < 1 {
		s.IdleDays = 90
	}
	return s
}

type hmeCandidate struct {
	Email       string `json:"email"`
	AccountID   int64  `json:"account_id"`
	AnonymousID string `json:"-"`
	Active      bool   `json:"active"`
	LastMailAt  string `json:"last_mail_at"`
	Reason      string `json:"reason"`
}

// hmeRecycleCandidate picks the alias whose loss costs least: deactivated
// first, then never-mailed old ones, then the longest idle. Pinned, pooled,
// user-bound and aliases not managed by tempmail (e.g. made on an iPhone)
// are never touched.
func (a *App) hmeRecycleCandidate(ctx context.Context, accountID int64) (*hmeCandidate, error) {
	s := a.hmeRecycleSettings(ctx)
	base := `SELECT email, account_id, anonymous_id, active, COALESCE(last_mail_at, '') AS last_mail_at FROM hme_aliases h
		WHERE account_id = ? AND managed = 1 AND pinned = 0 AND pooled = 0
		AND NOT EXISTS (SELECT 1 FROM address ad JOIN users_address ua ON ua.address_id = ad.id WHERE ad.name = h.email) AND `
	tiers := []struct{ cond, order, reason string }{
		{`active = 0`, `coalesce(apple_created_at, created_at)`, "inactive"},
		{`last_mail_at IS NULL AND coalesce(apple_created_at, created_at) < datetime('now', ?)`, `coalesce(apple_created_at, created_at)`, "never_received"},
		{`last_mail_at < datetime('now', ?)`, `last_mail_at`, "idle"},
	}
	for i, t := range tiers {
		args := []any{accountID}
		switch i {
		case 1:
			args = append(args, fmt.Sprintf("-%d day", s.UnusedDays))
		case 2:
			args = append(args, fmt.Sprintf("-%d day", s.IdleDays))
		}
		row, err := a.db.QueryOne(ctx, base+t.cond+` ORDER BY `+t.order+` LIMIT 1`, args...)
		if err != nil {
			return nil, err
		}
		if row != nil {
			return &hmeCandidate{Email: row.Str("email"), AccountID: row.Int("account_id"), AnonymousID: row.Str("anonymous_id"),
				Active: row.Int("active") == 1, LastMailAt: row.Str("last_mail_at"), Reason: t.reason}, nil
		}
	}
	return nil, nil
}

// hmeRecycleOne permanently deletes one candidate at Apple to free a slot;
// the local mailbox and its mail are kept.
func (a *App) hmeRecycleOne(ctx context.Context, accountID int64) (string, error) {
	c, err := a.hmeRecycleCandidate(ctx, accountID)
	if err != nil || c == nil {
		return "", err
	}
	if err := a.hmeRun(ctx, accountID, func(cl *icloud.Client) error {
		if c.Active {
			if err := cl.Deactivate(ctx, c.AnonymousID); err != nil {
				return err
			}
		}
		return cl.Delete(ctx, c.AnonymousID)
	}); err != nil {
		return "", err
	}
	_, err = a.db.Exec(ctx, `DELETE FROM hme_aliases WHERE email = ?`, c.Email)
	log.Printf("hme account %d: recycled %s (%s)", accountID, c.Email, c.Reason)
	return c.Email, err
}

// hmeAllocate hands out an unused alias first (pooled = never handed out and
// never mailed), then creates one on the least used account not cooling down.
func (a *App) hmeAllocate(ctx context.Context, accountID int64, label, note string) (string, error) {
	if label == "" {
		label = "tempmail " + time.Now().Format("2006-01-02 15:04")
	}
	q, args := `SELECT email, account_id, anonymous_id FROM hme_aliases WHERE pooled = 1 AND active = 1`, []any{}
	if accountID > 0 {
		q += ` AND account_id = ?`
		args = append(args, accountID)
	}
	if row, _ := a.db.QueryOne(ctx, q+` ORDER BY created_at LIMIT 1`, args...); row != nil {
		email := row.Str("email")
		if err := a.hmeRun(ctx, row.Int("account_id"), func(c *icloud.Client) error {
			return c.UpdateMeta(ctx, row.Str("anonymous_id"), label, note)
		}); err != nil {
			log.Printf("hme: relabel unused %s: %v", email, err)
		}
		_, err := a.db.Exec(ctx, `UPDATE hme_aliases SET pooled = 0, label = ?, note = ? WHERE email = ?`, label, note, email)
		return email, err
	}
	cq := `SELECT id FROM hme_accounts acc WHERE enabled = 1 AND status = 'active' AND (cooldown_until IS NULL OR cooldown_until <= datetime('now'))`
	if accountID > 0 {
		cq += ` AND id = ?`
	}
	rows, err := a.db.Query(ctx, cq+` ORDER BY (SELECT count(*) FROM hme_aliases h WHERE h.account_id = acc.id AND h.active = 1), id`, args...)
	if err != nil {
		return "", err
	}
	var lastErr error
	for _, r := range rows {
		al, err := a.hmeCreateOn(ctx, r.Int("id"), label, note, false)
		if err == nil {
			return al.Email, nil
		}
		lastErr = err
	}
	if lastErr != nil {
		return "", lastErr
	}
	if next, ok, _ := a.db.ScanString(ctx, `SELECT min(cooldown_until) FROM hme_accounts WHERE enabled = 1 AND status = 'active' AND cooldown_until > datetime('now')`); ok {
		return "", fmt.Errorf("%w until %s UTC", errHMECooldown, next)
	}
	return "", errors.New("no active iCloud account available")
}

func (a *App) hmeEnsureAddress(ctx context.Context, email, source string) (int64, error) {
	if _, err := a.db.Exec(ctx, `INSERT OR IGNORE INTO address(name, source_meta) VALUES(?, ?)`, email, source); err != nil {
		return 0, err
	}
	id, found, err := a.db.ScanInt(ctx, `SELECT id FROM address WHERE name = ?`, email)
	if err == nil && !found {
		err = errors.New("failed to create mailbox")
	}
	return id, err
}

// hmeFillBudget keeps background creation one below Apple's ~5 per 30
// minutes so refills rarely hit the throttle and on-demand creation keeps a slot.
const hmeFillBudget = 4

// RunHMEMaintenance keeps sessions alive, mirrors alias lists and refills
// stock: auto-fill accounts keep reserving until Apple reports the account
// full, others up to pool_size. Refills never recycle existing aliases.
func (a *App) RunHMEMaintenance(ctx context.Context) {
	if err := a.hmeLinkAll(ctx); err != nil {
		log.Printf("hme link: %v", err)
	}
	rows, err := a.db.Query(ctx, `SELECT id, pool_size, auto_fill,
		(last_synced_at IS NULL OR last_synced_at < datetime('now', '-6 hours')) AS need_sync,
		(last_validated_at IS NULL OR last_validated_at < datetime('now', '-30 minutes')) AS need_validate,
		(cooldown_until IS NULL OR cooldown_until <= datetime('now')) AS can_create,
		(observed_limit IS NOT NULL AND (SELECT count(*) FROM hme_aliases h WHERE h.account_id = acc.id) >= observed_limit) AS full,
		(SELECT count(*) FROM hme_aliases h WHERE h.account_id = acc.id AND h.pooled = 1 AND h.active = 1) AS pooled
		FROM hme_accounts acc WHERE enabled = 1 AND status IN ('active', 'pending')`)
	if err != nil {
		log.Printf("hme maintenance: %v", err)
		return
	}
	for _, r := range rows {
		id := r.Int("id")
		switch {
		case r.Int("need_sync") == 1:
			err = a.hmeSync(ctx, id)
		case r.Int("need_validate") == 1:
			err = a.hmeKeepAlive(ctx, id)
		default:
			err = nil
		}
		if err != nil {
			log.Printf("hme account %d: %v", id, err)
			continue
		}
		if r.Int("can_create") == 1 {
			a.hmeRefill(ctx, id, r.Int("auto_fill") == 1, r.Int("full") == 1, int(r.Int("pool_size")-r.Int("pooled")))
		}
	}
}

func (a *App) hmeRefill(ctx context.Context, id int64, autoFill, full bool, missing int) {
	if autoFill {
		if full {
			return
		}
		missing = hmeFillBudget
	}
	recent, _ := a.db.Count(ctx, `SELECT count(*) FROM hme_aliases WHERE account_id = ? AND apple_created_at > datetime('now', '-30 minutes')`, id)
	for n := min(missing, hmeFillBudget-int(recent)); n > 0; n-- {
		if _, err := a.hmeCreateOn(ctx, id, "tempmail pool", "", true); err != nil {
			log.Printf("hme account %d refill: %v", id, err)
			return
		}
	}
}

// ---- HTTP handlers ----

func hmeFail(w http.ResponseWriter, err error) {
	switch {
	case errors.Is(err, errHMENotFound):
		text(w, 404, err.Error())
	case errors.Is(err, icloud.ErrUnauthorized):
		text(w, 409, "iCloud session expired; sign in again or update cookies")
	case icloud.IsRateLimited(err) || icloud.IsQuotaFull(err) || errors.Is(err, errHMECooldown):
		text(w, 429, err.Error())
	default:
		var apiErr *icloud.APIError
		if errors.As(err, &apiErr) {
			text(w, 400, err.Error())
			return
		}
		text(w, 502, err.Error())
	}
}

const hmeAccountSelect = `SELECT acc.*,
	(SELECT count(*) FROM hme_aliases h WHERE h.account_id = acc.id) AS alias_total,
	(SELECT count(*) FROM hme_aliases h WHERE h.account_id = acc.id AND h.active = 1) AS alias_active,
	(SELECT count(*) FROM hme_aliases h WHERE h.account_id = acc.id AND h.active = 1 AND h.pooled = 1) AS alias_pooled
	FROM hme_accounts acc`

func (a *App) hmeAccountView(ctx context.Context, row db.Row) map[string]any {
	opts := []string{}
	_ = json.Unmarshal([]byte(row.Str("forward_options")), &opts)
	fwd := row.Str("forward_to")
	local, mailbox := false, false
	if fwd != "" {
		for _, d := range a.effective(ctx).Domains {
			if isDomainOrSubdomain(mailDomain(fwd), d) {
				local = true
			}
		}
		if local {
			_, mailbox, _ = a.db.ScanInt(ctx, `SELECT id FROM address WHERE name = ?`, fwd)
		}
	}
	out := map[string]any{"has_proxy": row.Str("proxy") != "", "forward_options": opts, "forward_local": local, "forward_mailbox": mailbox,
		"enabled": row.Int("enabled") == 1, "auto_fill": row.Int("auto_fill") == 1}
	for _, k := range []string{"id", "name", "apple_id", "host", "status", "last_error", "forward_to", "cooldown_until", "pool_size", "observed_limit",
		"last_validated_at", "last_synced_at", "created_at", "alias_total", "alias_active", "alias_pooled"} {
		out[k] = row[k]
	}
	return out
}

func (a *App) hmeRespondAccount(w http.ResponseWriter, r *http.Request, id int64) {
	row, err := a.db.QueryOne(r.Context(), hmeAccountSelect+` WHERE acc.id = ?`, id)
	if err != nil {
		fail(w, err)
		return
	}
	if row == nil {
		hmeFail(w, errHMENotFound)
		return
	}
	jsonResp(w, 200, a.hmeAccountView(r.Context(), row))
}

func (a *App) hmeListAccounts(w http.ResponseWriter, r *http.Request) {
	rows, err := a.db.Query(r.Context(), hmeAccountSelect+` ORDER BY acc.id`)
	if err != nil {
		fail(w, err)
		return
	}
	out := make([]map[string]any, 0, len(rows))
	for _, row := range rows {
		out = append(out, a.hmeAccountView(r.Context(), row))
	}
	jsonResp(w, 200, map[string]any{"results": out})
}

func (a *App) hmeCreateAccount(w http.ResponseWriter, r *http.Request) {
	var req struct {
		Name    string `json:"name"`
		Host    string `json:"host"`
		Proxy   string `json:"proxy"`
		Cookies string `json:"cookies"`
	}
	if err := readJSON(r, &req); err != nil {
		text(w, 400, "Invalid input")
		return
	}
	cookies, err := icloud.ParseCookies(req.Cookies)
	if err != nil {
		text(w, 400, err.Error())
		return
	}
	id, err := a.hmeSaveSession(r.Context(), 0, req.Name, req.Host, req.Proxy, cookies)
	if err != nil {
		text(w, 400, err.Error())
		return
	}
	_ = a.hmeSync(r.Context(), id)
	a.hmeRespondAccount(w, r, id)
}

// hmeSaveSession stores a fresh session for a new (id 0) or existing account.
func (a *App) hmeSaveSession(ctx context.Context, id int64, name, host, proxy string, cookies map[string]string) (int64, error) {
	if _, err := icloud.NewClient(nil, host, proxy); err != nil {
		return 0, err
	}
	enc, err := a.sealCookies(cookies)
	if err != nil {
		return 0, err
	}
	if id > 0 {
		_, err = a.db.Exec(ctx, `UPDATE hme_accounts SET cookies = ?, service_url = NULL, dsid = NULL, status = 'pending', last_error = '', updated_at = datetime('now') WHERE id = ?`, enc, id)
		return id, err
	}
	if name = strings.TrimSpace(name); name == "" {
		name = "iCloud"
	}
	_, err = a.db.Exec(ctx, `INSERT INTO hme_accounts(name, host, proxy, cookies) VALUES(?, ?, ?, ?)`, name, icloud.NormalizeHost(host), strings.TrimSpace(proxy), enc)
	if err != nil {
		return 0, err
	}
	id, _, err = a.db.ScanInt(ctx, `SELECT last_insert_rowid()`)
	return id, err
}

func (a *App) hmeUpdateAccount(w http.ResponseWriter, r *http.Request) {
	id := atoi(r.PathValue("id"))
	var req struct {
		Name     *string `json:"name"`
		Proxy    *string `json:"proxy"`
		PoolSize *int    `json:"pool_size"`
		AutoFill *bool   `json:"auto_fill"`
		Enabled  *bool   `json:"enabled"`
	}
	if err := readJSON(r, &req); err != nil {
		text(w, 400, "Invalid input")
		return
	}
	ctx := r.Context()
	sets, args := []string{}, []any{}
	if req.Name != nil && strings.TrimSpace(*req.Name) != "" {
		sets, args = append(sets, "name = ?"), append(args, strings.TrimSpace(*req.Name))
	}
	if req.Proxy != nil {
		if _, err := icloud.NewClient(nil, "", *req.Proxy); err != nil {
			text(w, 400, err.Error())
			return
		}
		sets, args = append(sets, "proxy = ?"), append(args, strings.TrimSpace(*req.Proxy))
	}
	if req.PoolSize != nil {
		if *req.PoolSize < 0 || *req.PoolSize > 20 {
			text(w, 400, "pool size must be between 0 and 20")
			return
		}
		sets, args = append(sets, "pool_size = ?"), append(args, *req.PoolSize)
	}
	if req.AutoFill != nil {
		sets, args = append(sets, "auto_fill = ?"), append(args, boolInt(*req.AutoFill))
	}
	if req.Enabled != nil {
		sets, args = append(sets, "enabled = ?"), append(args, boolInt(*req.Enabled))
	}
	if len(sets) > 0 {
		if _, err := a.db.Exec(ctx, `UPDATE hme_accounts SET `+strings.Join(sets, ", ")+`, updated_at = datetime('now') WHERE id = ?`, append(args, id)...); err != nil {
			fail(w, err)
			return
		}
	}
	a.hmeRespondAccount(w, r, id)
}

func (a *App) hmeUpdateCookies(w http.ResponseWriter, r *http.Request) {
	id := atoi(r.PathValue("id"))
	var req struct {
		Cookies string `json:"cookies"`
	}
	if err := readJSON(r, &req); err != nil {
		text(w, 400, "Invalid input")
		return
	}
	cookies, err := icloud.ParseCookies(req.Cookies)
	if err != nil {
		text(w, 400, err.Error())
		return
	}
	if _, err := a.hmeSaveSession(r.Context(), id, "", "", "", cookies); err != nil {
		text(w, 400, err.Error())
		return
	}
	if err := a.hmeSync(r.Context(), id); err != nil {
		hmeFail(w, err)
		return
	}
	a.hmeRespondAccount(w, r, id)
}

func (a *App) hmeDeleteAccount(w http.ResponseWriter, r *http.Request) {
	id := atoi(r.PathValue("id"))
	ctx := r.Context()
	if _, err := a.db.Exec(ctx, `DELETE FROM hme_aliases WHERE account_id = ?`, id); err != nil {
		fail(w, err)
		return
	}
	if _, err := a.db.Exec(ctx, `DELETE FROM hme_accounts WHERE id = ?`, id); err != nil {
		fail(w, err)
		return
	}
	ok(w)
}

func (a *App) hmeSyncAccount(w http.ResponseWriter, r *http.Request) {
	id := atoi(r.PathValue("id"))
	if err := a.hmeSync(r.Context(), id); err != nil {
		hmeFail(w, err)
		return
	}
	a.hmeRespondAccount(w, r, id)
}

func (a *App) hmeSetForward(w http.ResponseWriter, r *http.Request) {
	id := atoi(r.PathValue("id"))
	var req struct {
		Email string `json:"email"`
	}
	if err := readJSON(r, &req); err != nil {
		text(w, 400, "Invalid input")
		return
	}
	ctx := r.Context()
	email := strings.ToLower(strings.TrimSpace(req.Email))
	opts, _, _ := a.db.ScanString(ctx, `SELECT forward_options FROM hme_accounts WHERE id = ?`, id)
	var list []string
	_ = json.Unmarshal([]byte(opts), &list)
	allowed := false
	for _, o := range list {
		allowed = allowed || strings.EqualFold(o, email)
	}
	if !allowed {
		text(w, 400, "address is not verified on this Apple Account; add it at account.apple.com and sync first")
		return
	}
	if err := a.hmeRun(ctx, id, func(c *icloud.Client) error { return c.SetForwardTo(ctx, email) }); err != nil {
		hmeFail(w, err)
		return
	}
	if _, err := a.db.Exec(ctx, `UPDATE hme_accounts SET forward_to = ? WHERE id = ?`, email, id); err != nil {
		fail(w, err)
		return
	}
	a.hmeRespondAccount(w, r, id)
}

// hmeCreateRelay creates a local mailbox meant to be added to the Apple
// Account and selected as the Hide My Email forward target.
func (a *App) hmeCreateRelay(w http.ResponseWriter, r *http.Request) {
	var req struct {
		Domain string `json:"domain"`
	}
	if err := readJSON(r, &req); err != nil {
		text(w, 400, "Invalid input")
		return
	}
	res, err := a.newAddress(r.Context(), newAddressOpts{
		name: "hme" + randomString(10, "abcdefghijklmnopqrstuvwxyz0123456789"), domain: req.Domain,
		allowDomains: a.effective(r.Context()).Domains, sourceMeta: "hme-relay",
	})
	if err != nil {
		text(w, 400, "Failed to create address: "+err.Error())
		return
	}
	jsonResp(w, 200, res)
}

func (a *App) hmeLoginStart(w http.ResponseWriter, r *http.Request) {
	var req struct {
		AccountID int64  `json:"account_id"`
		Name      string `json:"name"`
		AppleID   string `json:"apple_id"`
		Password  string `json:"password"`
		Host      string `json:"host"`
		Proxy     string `json:"proxy"`
	}
	if err := readJSON(r, &req); err != nil || strings.TrimSpace(req.AppleID) == "" || req.Password == "" {
		text(w, 400, "Apple ID and password are required")
		return
	}
	ctx := r.Context()
	if req.AccountID > 0 {
		row, err := a.db.QueryOne(ctx, `SELECT host, proxy FROM hme_accounts WHERE id = ?`, req.AccountID)
		if err != nil || row == nil {
			hmeFail(w, errHMENotFound)
			return
		}
		req.Host, req.Proxy = row.Str("host"), row.Str("proxy")
	}
	lctx, cancel := context.WithTimeout(ctx, 60*time.Second)
	defer cancel()
	l, needCode, err := icloud.StartLogin(lctx, req.AppleID, req.Password, req.Host, req.Proxy)
	if err != nil {
		text(w, 400, err.Error())
		return
	}
	st := &hmeLoginState{login: l, accountID: req.AccountID, name: req.Name, host: req.Host, proxy: req.Proxy, expires: time.Now().Add(10 * time.Minute)}
	if !needCode {
		a.hmeFinishLogin(w, r, st)
		return
	}
	hmeLogins.Range(func(k, v any) bool {
		if time.Now().After(v.(*hmeLoginState).expires) {
			hmeLogins.Delete(k)
		}
		return true
	})
	loginID := randomString(32, "abcdefghijklmnopqrstuvwxyz0123456789")
	hmeLogins.Store(loginID, st)
	jsonResp(w, 200, map[string]any{"login_id": loginID, "need_code": true, "phones": l.Phones})
}

func hmePendingLogin(w http.ResponseWriter, r *http.Request) (*hmeLoginState, bool) {
	v, found := hmeLogins.Load(r.PathValue("id"))
	if !found || time.Now().After(v.(*hmeLoginState).expires) {
		text(w, 410, "sign-in expired, start again")
		return nil, false
	}
	return v.(*hmeLoginState), true
}

func (a *App) hmeLoginSMS(w http.ResponseWriter, r *http.Request) {
	st, found := hmePendingLogin(w, r)
	if !found {
		return
	}
	var req struct {
		PhoneID int `json:"phone_id"`
	}
	if err := readJSON(r, &req); err != nil || req.PhoneID <= 0 {
		text(w, 400, "phone_id is required")
		return
	}
	if err := st.login.RequestSMS(r.Context(), req.PhoneID); err != nil {
		text(w, 400, err.Error())
		return
	}
	ok(w)
}

func (a *App) hmeLoginCode(w http.ResponseWriter, r *http.Request) {
	st, found := hmePendingLogin(w, r)
	if !found {
		return
	}
	var req struct {
		Code    string `json:"code"`
		PhoneID int    `json:"phone_id"`
	}
	if err := readJSON(r, &req); err != nil || strings.TrimSpace(req.Code) == "" {
		text(w, 400, "verification code is required")
		return
	}
	if err := st.login.SubmitCode(r.Context(), strings.TrimSpace(req.Code), req.PhoneID); err != nil {
		text(w, 400, err.Error())
		return
	}
	hmeLogins.Delete(r.PathValue("id"))
	a.hmeFinishLogin(w, r, st)
}

func (a *App) hmeFinishLogin(w http.ResponseWriter, r *http.Request, st *hmeLoginState) {
	ctx := r.Context()
	cookies, err := st.login.Finish(ctx)
	if err != nil {
		text(w, 400, err.Error())
		return
	}
	name := st.name
	if name == "" {
		name = st.login.AppleID
	}
	id, err := a.hmeSaveSession(ctx, st.accountID, name, st.host, st.proxy, cookies)
	if err != nil {
		text(w, 400, err.Error())
		return
	}
	_ = a.hmeSync(ctx, id)
	a.hmeRespondAccount(w, r, id)
}

func (a *App) hmeListAliases(w http.ResponseWriter, r *http.Request) {
	q := r.URL.Query()
	where, args := []string{"1 = 1"}, []any{}
	if id := atoi(q.Get("account_id")); id > 0 {
		where, args = append(where, "h.account_id = ?"), append(args, id)
	}
	switch q.Get("state") {
	case "active":
		where = append(where, "h.active = 1")
	case "inactive":
		where = append(where, "h.active = 0")
	case "unused":
		where = append(where, "h.pooled = 1")
	case "used":
		where = append(where, "h.pooled = 0")
	}
	if s := strings.TrimSpace(q.Get("query")); s != "" {
		where, args = append(where, "(h.email LIKE ? OR h.label LIKE ?)"), append(args, "%"+s+"%", "%"+s+"%")
	}
	base := ` FROM hme_aliases h JOIN hme_accounts acc ON acc.id = h.account_id LEFT JOIN address ad ON ad.name = h.email WHERE ` + strings.Join(where, " AND ")
	a.listQuery(w, r, `SELECT h.email, h.account_id, acc.name AS account_name, h.anonymous_id, h.label, h.note, h.active, h.pooled,
		h.pinned, h.managed, h.last_mail_at, h.apple_created_at, h.created_at, ad.id AS address_id, (SELECT count(*) FROM raw_mails rm WHERE rm.address = h.email) AS mail_count`+base,
		`SELECT count(*)`+base, args, q.Get("limit"), q.Get("offset"), "coalesce(h.apple_created_at, h.created_at) desc")
}

func (a *App) hmeIssueMailbox(ctx context.Context, w http.ResponseWriter, email string) {
	id, err := a.hmeEnsureAddress(ctx, email, "icloud-hme")
	if err != nil {
		fail(w, err)
		return
	}
	token, err := a.jwt.AddressToken(email, id)
	if err != nil {
		fail(w, err)
		return
	}
	jsonResp(w, 200, newAddressResult{JWT: token, Address: email, AddressID: id})
}

func (a *App) hmeCreateAlias(w http.ResponseWriter, r *http.Request) {
	var req struct {
		AccountID int64  `json:"account_id"`
		Label     string `json:"label"`
		Note      string `json:"note"`
	}
	if err := readJSON(r, &req); err != nil {
		text(w, 400, "Invalid input")
		return
	}
	email, err := a.hmeAllocate(r.Context(), req.AccountID, strings.TrimSpace(req.Label), strings.TrimSpace(req.Note))
	if err != nil {
		hmeFail(w, err)
		return
	}
	a.hmeIssueMailbox(r.Context(), w, email)
}

func (a *App) hmeAlias(w http.ResponseWriter, r *http.Request) db.Row {
	row, err := a.db.QueryOne(r.Context(), `SELECT * FROM hme_aliases WHERE email = ?`, strings.ToLower(r.PathValue("email")))
	if err != nil {
		fail(w, err)
		return nil
	}
	if row == nil {
		text(w, 404, "alias not found")
	}
	return row
}

func (a *App) hmeUpdateAlias(w http.ResponseWriter, r *http.Request) {
	row := a.hmeAlias(w, r)
	if row == nil {
		return
	}
	var req struct {
		Label string `json:"label"`
		Note  string `json:"note"`
	}
	if err := readJSON(r, &req); err != nil || strings.TrimSpace(req.Label) == "" {
		text(w, 400, "label is required")
		return
	}
	ctx := r.Context()
	label, note := strings.TrimSpace(req.Label), strings.TrimSpace(req.Note)
	if err := a.hmeRun(ctx, row.Int("account_id"), func(c *icloud.Client) error {
		return c.UpdateMeta(ctx, row.Str("anonymous_id"), label, note)
	}); err != nil {
		hmeFail(w, err)
		return
	}
	if _, err := a.db.Exec(ctx, `UPDATE hme_aliases SET label = ?, note = ? WHERE email = ?`, label, note, row.Str("email")); err != nil {
		fail(w, err)
		return
	}
	ok(w)
}

func (a *App) hmeToggleAlias(w http.ResponseWriter, r *http.Request) {
	row := a.hmeAlias(w, r)
	if row == nil {
		return
	}
	ctx := r.Context()
	activate := strings.HasSuffix(r.URL.Path, "/reactivate")
	if err := a.hmeRun(ctx, row.Int("account_id"), func(c *icloud.Client) error {
		if activate {
			return c.Reactivate(ctx, row.Str("anonymous_id"))
		}
		return c.Deactivate(ctx, row.Str("anonymous_id"))
	}); err != nil {
		hmeFail(w, err)
		return
	}
	if _, err := a.db.Exec(ctx, `UPDATE hme_aliases SET active = ? WHERE email = ?`, boolInt(activate), row.Str("email")); err != nil {
		fail(w, err)
		return
	}
	ok(w)
}

// hmeDeleteAlias permanently deletes the alias at Apple (freeing a slot in
// the account's cap) together with its mailbox and mail. Mailboxes always
// mirror aliases, so there is no local-only removal.
func (a *App) hmeDeleteAlias(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	row := a.hmeAlias(w, r)
	if row == nil {
		return
	}
	email := row.Str("email")
	if err := a.hmeRun(ctx, row.Int("account_id"), func(c *icloud.Client) error {
		if row.Int("active") == 1 {
			if err := c.Deactivate(ctx, row.Str("anonymous_id")); err != nil {
				return err
			}
		}
		return c.Delete(ctx, row.Str("anonymous_id"))
	}); err != nil {
		hmeFail(w, err)
		return
	}
	if _, err := a.db.Exec(ctx, `DELETE FROM hme_aliases WHERE email = ?`, email); err != nil {
		fail(w, err)
		return
	}
	// A freed slot ends a "maximum reached" cooldown; throttle cooldowns
	// (throttle_streak > 0) must run their course.
	_, _ = a.db.Exec(ctx, `UPDATE hme_accounts SET cooldown_until = NULL WHERE id = ? AND throttle_streak = 0`, row.Int("account_id"))
	if err := a.deleteAddressesWhere(ctx, `name = ?`, email); err != nil {
		fail(w, err)
		return
	}
	ok(w)
}

func (a *App) hmePinAlias(w http.ResponseWriter, r *http.Request) {
	row := a.hmeAlias(w, r)
	if row == nil {
		return
	}
	var req struct {
		Pinned bool `json:"pinned"`
	}
	if err := readJSON(r, &req); err != nil {
		text(w, 400, "Invalid input")
		return
	}
	if _, err := a.db.Exec(r.Context(), `UPDATE hme_aliases SET pinned = ? WHERE email = ?`, boolInt(req.Pinned), row.Str("email")); err != nil {
		fail(w, err)
		return
	}
	ok(w)
}

// hmeGetRecycle returns the recycle settings plus, per account, the alias that
// would be freed next.
func (a *App) hmeGetRecycle(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	rows, err := a.db.Query(ctx, `SELECT id, name FROM hme_accounts ORDER BY id`)
	if err != nil {
		fail(w, err)
		return
	}
	preview := []map[string]any{}
	for _, acc := range rows {
		c, err := a.hmeRecycleCandidate(ctx, acc.Int("id"))
		if err != nil {
			fail(w, err)
			return
		}
		preview = append(preview, map[string]any{"account_id": acc.Int("id"), "account_name": acc.Str("name"), "candidate": c})
	}
	jsonResp(w, 200, map[string]any{"settings": a.hmeRecycleSettings(ctx), "preview": preview})
}

func (a *App) hmeSaveRecycle(w http.ResponseWriter, r *http.Request) {
	var s hmeRecycleConfig
	if err := readJSON(r, &s); err != nil || s.UnusedDays < 1 || s.UnusedDays > 3650 || s.IdleDays < 1 || s.IdleDays > 3650 {
		text(w, 400, "days must be between 1 and 3650")
		return
	}
	if err := a.saveJSONSetting(r.Context(), "hme_recycle_settings", s); err != nil {
		fail(w, err)
		return
	}
	a.hmeGetRecycle(w, r)
}
