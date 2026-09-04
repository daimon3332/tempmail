package server

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"strings"
	"time"

	"tempmail/internal/mailer"
)

type sendBalanceState struct {
	isNoLimit        bool
	needCheckBalance bool
	balance          int64
	hasBalance       bool
}

func (a *App) ensureDefaultBalance(ctx context.Context, address string) {
	if address == "" || a.cfg.DefaultSendBalance <= 0 {
		return
	}
	a.db.Exec(ctx, `INSERT INTO address_sender (address, balance, enabled) VALUES (?, ?, 1) ON CONFLICT(address) DO NOTHING`,
		address, a.cfg.DefaultSendBalance)
}

func (a *App) sendBalanceState(r *http.Request, address string, isAdmin, initDefault bool) sendBalanceState {
	ctx := r.Context()
	if !isAdmin {
		if u := userOf(r); u != nil {
			if l, ok := a.roles.UserLimits(ctx, claimInt(u, "user_id")); ok && !l.CanSendMail {
				return sendBalanceState{needCheckBalance: true}
			}
		}
	}
	role := userRoleOf(r)
	noLimitRole := role != "" && contains(a.cfg.NoLimitSendRole, role)
	if !noLimitRole && role != "" {
		if rc, _ := a.roles.Get(ctx, role); rc != nil && rc.Source == "db" && !rc.CanSendMail {
			return sendBalanceState{needCheckBalance: true}
		}
	}
	noLimitAddr := false
	if !noLimitRole {
		var list []string
		a.jsonSetting(ctx, "no_limit_send_address_list", &list)
		noLimitAddr = contains(list, address)
	}
	isNoLimit := noLimitRole || noLimitAddr
	need := !isAdmin && !isNoLimit
	if need && initDefault {
		a.ensureDefaultBalance(ctx, address)
	}
	if isNoLimit {
		return sendBalanceState{isNoLimit: true, balance: 99999, hasBalance: true}
	}
	bal, found, _ := a.db.ScanInt(ctx, `SELECT balance FROM address_sender where address = ? and enabled = 1`, address)
	return sendBalanceState{needCheckBalance: need, balance: bal, hasBalance: found}
}

type sendMailReq struct {
	FromName string `json:"from_name"`
	ToMail   string `json:"to_mail"`
	ToName   string `json:"to_name"`
	Subject  string `json:"subject"`
	Content  string `json:"content"`
	IsHTML   bool   `json:"is_html"`
}

type sendMailLimitConfig struct {
	DailyEnabled   bool `json:"dailyEnabled"`
	MonthlyEnabled bool `json:"monthlyEnabled"`
	DailyLimit     *int `json:"dailyLimit"`
	MonthlyLimit   *int `json:"monthlyLimit"`
}

func (a *App) sendLimitConfig(ctx context.Context) *sendMailLimitConfig {
	var c sendMailLimitConfig
	if !a.jsonSetting(ctx, "send_mail_limit_config", &c) {
		return nil
	}
	return &c
}

func dailyKey() string { return "send_mail_limit_count:daily:" + time.Now().UTC().Format("2006-01-02") }
func monthlyKey() string {
	return "send_mail_limit_count:monthly:" + time.Now().UTC().Format("2006-01")
}

func (a *App) settingInt(ctx context.Context, key string) int64 {
	v, _ := a.db.GetSetting(ctx, key)
	return atoi(v)
}

func (a *App) ensureSendLimit(ctx context.Context) error {
	c := a.sendLimitConfig(ctx)
	if c == nil || (!c.DailyEnabled && !c.MonthlyEnabled) {
		return nil
	}
	if c.DailyEnabled && c.DailyLimit != nil && *c.DailyLimit != -1 && a.settingInt(ctx, dailyKey()) >= int64(*c.DailyLimit) {
		return errors.New("Server daily send mail limit reached")
	}
	if c.MonthlyEnabled && c.MonthlyLimit != nil && *c.MonthlyLimit != -1 && a.settingInt(ctx, monthlyKey()) >= int64(*c.MonthlyLimit) {
		return errors.New("Server monthly send mail limit reached")
	}
	return nil
}

func (a *App) increaseSendLimit(ctx context.Context) {
	c := a.sendLimitConfig(ctx)
	if c == nil {
		return
	}
	inc := func(k string) {
		a.db.Exec(ctx, `INSERT INTO settings (key, value) VALUES (?, '1')
			ON CONFLICT(key) DO UPDATE SET value = CAST(COALESCE(value,'0') AS INTEGER) + 1, updated_at = datetime('now')`, k)
	}
	if c.DailyEnabled {
		inc(dailyKey())
	}
	if c.MonthlyEnabled {
		inc(monthlyKey())
	}
	a.db.Exec(ctx, `DELETE FROM settings WHERE key LIKE 'send_mail_limit_count:daily:%' AND key < ?`, dailyKey())
	a.db.Exec(ctx, `DELETE FROM settings WHERE key LIKE 'send_mail_limit_count:monthly:%' AND key < ?`, monthlyKey())
}

func (a *App) sendMail(r *http.Request, address string, req sendMailReq, isAdmin bool) error {
	ctx := r.Context()
	if address == "" {
		return errors.New("Address not found")
	}
	domain := mailDomain(address)
	if !contains(a.cfg.Domains, domain) {
		return errors.New("Invalid domain")
	}
	st := a.sendBalanceState(r, address, isAdmin, true)
	if st.needCheckBalance && (!st.hasBalance || st.balance <= 0) {
		return errors.New("No send balance")
	}
	if req.ToMail == "" {
		return errors.New("Invalid to mail")
	}
	var block []string
	a.jsonSetting(ctx, "send_block_list", &block)
	for _, b := range block {
		if b != "" && strings.Contains(req.ToMail, b) {
			return errors.New("Address is blocked")
		}
	}
	if req.Subject == "" {
		return errors.New("Subject is empty")
	}
	if req.Content == "" {
		return errors.New("Content is empty")
	}
	if err := a.ensureSendLimit(ctx); err != nil {
		return err
	}
	raw := mailer.Build(mailer.Message{FromName: req.FromName, From: address, ToName: req.ToName, To: req.ToMail,
		Subject: req.Subject, Content: req.Content, IsHTML: req.IsHTML})
	if err := a.mailer.Send(address, req.ToMail, raw); err != nil {
		return err
	}
	a.increaseSendLimit(ctx)
	if st.needCheckBalance {
		a.db.Exec(ctx, `UPDATE address_sender SET balance = balance - 1 where address = ?`, address)
	}
	a.touchAddress(ctx, address)
	body, _ := json.Marshal(map[string]any{
		"version": "v2", "from_name": req.FromName, "to_mail": req.ToMail, "to_name": req.ToName,
		"subject": req.Subject, "content": req.Content, "is_html": req.IsHTML,
		"geoData": map[string]any{"ip": clientIP(r, a.cfg.TrustedProxies)},
	})
	a.db.Exec(ctx, `INSERT INTO sendbox (address, raw) VALUES (?, ?)`, address, string(body))
	return nil
}

func (a *App) apiRequestSendAccess(w http.ResponseWriter, r *http.Request) {
	address, _ := addressOf(r)
	if address == "" {
		text(w, 400, "Address not found")
		return
	}
	ctx := r.Context()
	if a.cfg.DefaultSendBalance > 0 {
		a.ensureDefaultBalance(ctx, address)
		st := a.sendBalanceState(r, address, false, false)
		if st.balance > 0 {
			jsonResp(w, 200, map[string]string{"status": "ok"})
			return
		}
		text(w, 400, "Already requested")
		return
	}
	_, err := a.db.Exec(ctx, `INSERT INTO address_sender (address, balance, enabled) VALUES (?, 0, 0)`, address)
	if err != nil {
		if strings.Contains(err.Error(), "UNIQUE") {
			text(w, 400, "Already requested")
			return
		}
		text(w, 500, "Operation failed")
		return
	}
	jsonResp(w, 200, map[string]string{"status": "ok"})
}

func (a *App) apiSendMail(w http.ResponseWriter, r *http.Request) {
	address, _ := addressOf(r)
	var req sendMailReq
	readJSON(r, &req)
	if err := a.sendMail(r, address, req, false); err != nil {
		text(w, 400, "Failed to send mail "+err.Error())
		return
	}
	jsonResp(w, 200, map[string]string{"status": "ok"})
}

func (a *App) externalSendMail(w http.ResponseWriter, r *http.Request) {
	var req struct {
		sendMailReq
		Token string `json:"token"`
	}
	readJSON(r, &req)
	c, err := a.jwt.Verify(req.Token)
	if err != nil || claimStr(c, "address") == "" {
		text(w, 400, "Address not found")
		return
	}
	if err := a.sendMail(r, claimStr(c, "address"), req.sendMailReq, false); err != nil {
		text(w, 400, "Failed to send mail "+err.Error())
		return
	}
	jsonResp(w, 200, map[string]string{"status": "ok"})
}

func (a *App) apiSendbox(w http.ResponseWriter, r *http.Request) {
	address, _ := addressOf(r)
	if address == "" {
		jsonResp(w, 400, map[string]string{"error": "No address"})
		return
	}
	q := r.URL.Query()
	a.listQuery(w, r, `SELECT * FROM sendbox where address = ?`, `SELECT count(*) as count FROM sendbox where address = ?`,
		[]any{address}, q.Get("limit"), q.Get("offset"), "")
}

func (a *App) apiDeleteSendbox(w http.ResponseWriter, r *http.Request) {
	if !a.effective(r.Context()).EnableUserDeleteEmail {
		text(w, 403, "User delete email is disabled")
		return
	}
	address, _ := addressOf(r)
	_, err := a.db.Exec(r.Context(), `DELETE FROM sendbox WHERE address = ? and id = ?`, address, atoi(r.PathValue("id")))
	jsonResp(w, 200, map[string]bool{"success": err == nil})
}
