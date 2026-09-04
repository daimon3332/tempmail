package server

import (
	"context"
	"net/http"
	"strings"
)

// audit records a write operation by an admin into operation_log.
func (a *App) audit(ctx context.Context, action, target string) {
	a.db.Exec(ctx, `INSERT INTO operation_log (time, actor, action, target, result)
		VALUES (datetime('now'), ?, ?, ?, 'ok')`, "admin", action, target)
}

func (a *App) operationLogList(w http.ResponseWriter, r *http.Request) {
	q := r.URL.Query()
	limit, offset := atoi(q.Get("limit")), atoi(q.Get("offset"))
	if limit <= 0 || limit > 200 {
		limit = 50
	}
	if offset < 0 {
		offset = 0
	}
	where := "1=1"
	params := []any{}
	if act := q.Get("action"); act != "" {
		where += " AND action = ?"
		params = append(params, act)
	}
	if tg := q.Get("target"); tg != "" {
		where += " AND target LIKE ?"
		params = append(params, "%"+tg+"%")
	}
	rows, err := a.db.Query(r.Context(),
		`SELECT id, time, actor, action, target, result FROM operation_log WHERE `+where+` ORDER BY id DESC LIMIT ? OFFSET ?`,
		append(params, limit, offset)...)
	if err != nil {
		fail(w, err)
		return
	}
	var count int64
	if offset == 0 {
		count, _ = a.db.Count(r.Context(), `SELECT COUNT(*) FROM operation_log WHERE `+where, params...)
	}
	jsonResp(w, 200, map[string]any{"results": rows, "count": count})
}

func (a *App) operationLogClear(w http.ResponseWriter, r *http.Request) {
	a.db.Exec(r.Context(), `DELETE FROM operation_log`)
	ok(w)
}

// isAPIKeyValid checks the runtime API key against a request (x-api-key or
// Authorization: Bearer). Empty key disables external API key auth.
func (a *App) isAPIKeyValid(r *http.Request) bool {
	rc := a.effective(r.Context())
	if rc.APIKey == "" {
		return true
	}
	key := r.Header.Get("x-api-key")
	if key == "" {
		if h := r.Header.Get("Authorization"); strings.HasPrefix(h, "Bearer ") {
			key = strings.TrimPrefix(h, "Bearer ")
		}
	}
	return key == rc.APIKey
}

// apiKeyGuard rejects requests that fail the runtime API key check.
func (a *App) apiKeyGuard(next http.HandlerFunc) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		if !a.isAPIKeyValid(r) {
			text(w, 401, "Invalid API key")
			return
		}
		next(w, r)
	}
}

// sitePasswords returns the effective site password list (runtime override or env).
func (a *App) sitePasswords(ctx context.Context) []string {
	rc := a.effective(ctx)
	if rc.SitePasswordEnabled && rc.SitePassword != "" {
		return []string{rc.SitePassword}
	}
	return a.cfg.Passwords
}

func (a *App) usedSitePassword() bool { return len(a.cfg.Passwords) > 0 }

// adminPasswords returns the effective admin password list.
func (a *App) adminPasswords(ctx context.Context) []string {
	rc := a.effective(ctx)
	if rc.AdminPasswordEnabled && rc.AdminPassword != "" {
		return []string{rc.AdminPassword}
	}
	return a.cfg.AdminPasswords
}

func (a *App) usedAdminPassword() bool { return len(a.cfg.AdminPasswords) > 0 }

// needAuth reports whether the site-level custom auth is active.
func (a *App) needAuth(r *http.Request) bool {
	ctx := r.Context()
	rc := a.effective(ctx)
	if rc.SitePasswordEnabled && rc.SitePassword != "" {
		return !contains([]string{rc.SitePassword}, r.Header.Get("x-custom-auth"))
	}
	return len(a.cfg.Passwords) > 0 && !contains(a.cfg.Passwords, r.Header.Get("x-custom-auth"))
}
