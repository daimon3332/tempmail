package server

import (
	"encoding/json"
	"net/http"
	"strings"
	"time"
)

func (a *App) routes() {
	m := a.mux
	m.HandleFunc("GET /health_check", a.healthCheck)
	m.HandleFunc("GET /open_api/settings", a.openSettings)
	m.HandleFunc("POST /open_api/site_login", a.siteLogin)
	m.HandleFunc("POST /open_api/admin_login", a.adminLogin)
	m.HandleFunc("POST /open_api/credential_login", a.credentialLogin)

	// address-scoped api
	m.HandleFunc("GET /api/mails", a.apiListMails)
	m.HandleFunc("GET /api/mail/{id}", a.apiGetMail)
	m.HandleFunc("DELETE /api/mails/{id}", a.apiDeleteMail)
	m.HandleFunc("POST /api/mails/{id}/read", a.apiMarkRead)
	m.HandleFunc("GET /api/parsed_mails", a.apiListParsedMails)
	m.HandleFunc("GET /api/parsed_mail/{id}", a.apiGetParsedMail)
	m.HandleFunc("GET /api/settings", a.apiSettings)
	m.HandleFunc("POST /api/new_address", a.apiNewAddress)
	m.HandleFunc("DELETE /api/delete_address", a.apiDeleteAddress)
	m.HandleFunc("DELETE /api/clear_inbox", a.apiClearInbox)
	m.HandleFunc("DELETE /api/clear_sent_items", a.apiClearSentItems)
	m.HandleFunc("POST /api/address_change_password", a.apiChangePassword)
	m.HandleFunc("POST /api/address_login", a.apiAddressLogin)
	m.HandleFunc("GET /api/auto_reply", a.apiGetAutoReply)
	m.HandleFunc("POST /api/auto_reply", a.apiSaveAutoReply)
	m.HandleFunc("GET /api/webhook/settings", a.apiGetWebhook)
	m.HandleFunc("POST /api/webhook/settings", a.apiSaveWebhook)
	m.HandleFunc("POST /api/webhook/test", a.apiTestWebhook)
	m.HandleFunc("POST /api/request_send_mail_access", a.apiRequestSendAccess)
	m.HandleFunc("POST /api/send_mail", a.apiSendMail)
	m.HandleFunc("POST /external/api/send_mail", a.externalSendMail)
	m.HandleFunc("GET /api/sendbox", a.apiSendbox)
	m.HandleFunc("DELETE /api/sendbox/{id}", a.apiDeleteSendbox)
	m.HandleFunc("GET /api/attachment/list", func(w http.ResponseWriter, r *http.Request) { text(w, 400, "S3 is not enabled") })

	a.userRoutes()
	a.adminRoutes()
	a.extRoutes()
	m.HandleFunc("/", func(w http.ResponseWriter, r *http.Request) { text(w, 404, "Not Found") })
}

func (a *App) healthCheck(w http.ResponseWriter, r *http.Request) {
	if err := a.db.PingContext(r.Context()); err != nil {
		text(w, 500, "DB not available")
		return
	}
	text(w, 200, "OK")
}

func (a *App) openSettings(w http.ResponseWriter, r *http.Request) {
	needAuth := len(a.cfg.Passwords) > 0 && !contains(a.cfg.Passwords, r.Header.Get("x-custom-auth"))
	jsonResp(w, 200, map[string]any{
		"title":                           a.cfg.Title,
		"announcement":                    a.cfg.Announcement,
		"alwaysShowAnnouncement":          false,
		"prefix":                          a.cfg.Prefix,
		"addressRegex":                    a.cfg.AddressRegex,
		"minAddressLen":                   a.cfg.MinAddressLen,
		"maxAddressLen":                   a.cfg.MaxAddressLen,
		"defaultDomains":                  a.cfg.DefaultDomains,
		"domains":                         a.cfg.Domains,
		"randomSubdomainDomains":          a.cfg.RandomSubdomainDomains,
		"domainLabels":                    orEmpty(a.cfg.DomainLabels),
		"needAuth":                        needAuth,
		"adminContact":                    a.cfg.AdminContact,
		"enableUserCreateEmail":           a.cfg.EnableUserCreateEmail,
		"disableAnonymousUserCreateEmail": a.cfg.DisableAnonymousUserCreateEmail,
		"disableCustomAddressName":        a.cfg.DisableCustomAddressName,
		"enableUserDeleteEmail":           a.cfg.EnableUserDeleteEmail,
		"enableMailReadStatus":            a.cfg.EnableMailReadStatus,
		"enableAutoReply":                 a.cfg.EnableAutoReply,
		"enableIndexAbout":                a.cfg.EnableIndexAbout,
		"copyright":                       a.cfg.Copyright,
		"cfTurnstileSiteKey":              "",
		"enableWebhook":                   a.cfg.EnableWebhook,
		"isS3Enabled":                     false,
		"enableSendMail":                  true,
		"version":                         "v1.12.0",
		"showGithub":                      !a.cfg.DisableShowGithub,
		"showGithubForUser":               !a.cfg.DisableShowGithub,
		"disableAdminPasswordCheck":       a.cfg.DisableAdminPasswordCheck,
		"enableAddressPassword":           a.cfg.EnableAddressPassword,
		"enableAgentEmailInfo":            false,
		"smtpImapProxyConfig": map[string]any{
			"smtp": map[string]any{"host": "", "port": 8025, "starttls": false},
			"imap": map[string]any{"host": "", "port": 11143, "starttls": false},
		},
		"statusUrl":                  "",
		"enableGlobalTurnstileCheck": false,
	})
}

func orEmpty(s []string) []string {
	if s == nil {
		return []string{}
	}
	return s
}

func hashedContains(list []string, hashed string) bool {
	for _, p := range list {
		if sha256Hex(p) == hashed {
			return true
		}
	}
	return false
}

func (a *App) siteLogin(w http.ResponseWriter, r *http.Request) {
	var req struct {
		Password string `json:"password"`
	}
	readJSON(r, &req)
	if !hashedContains(a.cfg.Passwords, req.Password) {
		text(w, 401, "Need Custom Auth Password")
		return
	}
	ok(w)
}

func (a *App) adminLogin(w http.ResponseWriter, r *http.Request) {
	var req struct {
		Password string `json:"password"`
	}
	readJSON(r, &req)
	if !hashedContains(a.cfg.AdminPasswords, req.Password) {
		text(w, 401, "Need admin password")
		return
	}
	ok(w)
}

func (a *App) credentialLogin(w http.ResponseWriter, r *http.Request) {
	var req struct {
		Credential string `json:"credential"`
	}
	readJSON(r, &req)
	c, err := a.jwt.Verify(req.Credential)
	if err != nil || claimStr(c, "address") == "" {
		text(w, 401, "Invalid address credential")
		return
	}
	ok(w)
}

// ---- /api ----

func (a *App) apiListMails(w http.ResponseWriter, r *http.Request) {
	address, _ := addressOf(r)
	if address == "" {
		jsonResp(w, 400, map[string]string{"error": "No address"})
		return
	}
	q := r.URL.Query()
	if atoi(q.Get("offset")) <= 0 {
		a.touchAddress(r.Context(), address)
	}
	where, params := incrementalFilter(r, `address = ?`, []any{address})
	a.listQuery(w, r, `SELECT * FROM raw_mails where `+where, `SELECT count(*) as count FROM raw_mails where `+where,
		params, q.Get("limit"), q.Get("offset"), "")
}

// incrementalFilter appends optional after_id / after (RFC3339) constraints.
func incrementalFilter(r *http.Request, where string, params []any) (string, []any) {
	q := r.URL.Query()
	if id := atoi(q.Get("after_id")); id > 0 {
		where += " and id > ?"
		params = append(params, id)
	}
	if t := q.Get("after"); t != "" {
		if ts, err := time.Parse(time.RFC3339, t); err == nil {
			where += " and created_at > ?"
			params = append(params, ts.UTC().Format("2006-01-02 15:04:05"))
		}
	}
	return where, params
}

func (a *App) apiGetMail(w http.ResponseWriter, r *http.Request) {
	address, _ := addressOf(r)
	row, err := a.db.QueryOne(r.Context(), `SELECT * FROM raw_mails where id = ? and address = ?`, atoi(r.PathValue("id")), address)
	if err != nil {
		fail(w, err)
		return
	}
	if row == nil {
		jsonResp(w, 200, nil)
		return
	}
	resolveRaw(row)
	jsonResp(w, 200, row)
}

func (a *App) apiDeleteMail(w http.ResponseWriter, r *http.Request) {
	if !a.cfg.EnableUserDeleteEmail {
		text(w, 403, "User delete email is disabled")
		return
	}
	address, _ := addressOf(r)
	_, err := a.db.Exec(r.Context(), `DELETE FROM raw_mails WHERE address = ? and id = ?`, strings.ToLower(address), atoi(r.PathValue("id")))
	jsonResp(w, 200, map[string]bool{"success": err == nil})
}

func (a *App) apiMarkRead(w http.ResponseWriter, r *http.Request) {
	if !a.cfg.EnableMailReadStatus {
		jsonResp(w, 403, map[string]string{"error": "Mail read status is disabled"})
		return
	}
	address, _ := addressOf(r)
	var req struct {
		IsUnread *bool `json:"isUnread"`
	}
	readJSON(r, &req)
	if req.IsUnread == nil {
		jsonResp(w, 400, map[string]string{"error": "isUnread must be a boolean"})
		return
	}
	v := 0
	if *req.IsUnread {
		v = 1
	}
	_, err := a.db.Exec(r.Context(), `UPDATE raw_mails SET is_unread = ? WHERE id = ? AND address = ? AND COALESCE(is_unread, 0) != ?`,
		v, atoi(r.PathValue("id")), address, v)
	jsonResp(w, 200, map[string]bool{"success": err == nil})
}

func (a *App) parsedRow(row map[string]any) map[string]any {
	resolveRaw(row)
	raw, _ := row["raw"].(string)
	delete(row, "raw")
	row["sender"], row["subject"], row["text"], row["html"], row["attachments"] = "", "", "", "", []any{}
	if raw == "" {
		return row
	}
	if m := a.parseRawFromRow(map[string]any{"raw": raw}); m != nil {
		row["sender"] = strings.TrimSpace(m.Sender)
		row["subject"] = m.Subject
		row["text"] = m.Text
		row["html"] = m.HTML
		atts := make([]map[string]any, 0, len(m.Attachments))
		for _, at := range m.Attachments {
			atts = append(atts, map[string]any{"filename": at.Filename, "mimeType": at.MimeType, "disposition": at.Disposition, "size": at.Size})
		}
		row["attachments"] = atts
	}
	return row
}

func (a *App) apiListParsedMails(w http.ResponseWriter, r *http.Request) {
	address, _ := addressOf(r)
	if address == "" {
		jsonResp(w, 400, map[string]string{"error": "No address"})
		return
	}
	q := r.URL.Query()
	limit, offset := atoi(q.Get("limit")), atoi(q.Get("offset"))
	if limit <= 0 || limit > 100 {
		text(w, 400, "Invalid limit")
		return
	}
	if q.Get("offset") == "" || offset < 0 {
		text(w, 400, "Invalid offset")
		return
	}
	if offset <= 0 {
		a.touchAddress(r.Context(), address)
	}
	rows, err := a.db.Query(r.Context(), `SELECT * FROM raw_mails where address = ? order by id desc limit ? offset ?`, address, limit, offset)
	if err != nil {
		fail(w, err)
		return
	}
	out := make([]map[string]any, 0, len(rows))
	for _, row := range rows {
		out = append(out, a.parsedRow(row))
	}
	var count int64
	if offset == 0 {
		count, _ = a.db.Count(r.Context(), `SELECT count(*) FROM raw_mails where address = ?`, address)
	}
	jsonResp(w, 200, map[string]any{"results": out, "count": count})
}

func (a *App) apiGetParsedMail(w http.ResponseWriter, r *http.Request) {
	address, _ := addressOf(r)
	row, err := a.db.QueryOne(r.Context(), `SELECT * FROM raw_mails where id = ? and address = ?`, atoi(r.PathValue("id")), address)
	if err != nil {
		fail(w, err)
		return
	}
	if row == nil {
		jsonResp(w, 200, nil)
		return
	}
	jsonResp(w, 200, a.parsedRow(row))
}

func (a *App) apiSettings(w http.ResponseWriter, r *http.Request) {
	address, addressID := addressOf(r)
	ctx := r.Context()
	if addressID > 0 {
		if _, found, _ := a.db.ScanInt(ctx, `SELECT id FROM address where id = ?`, addressID); !found {
			text(w, 400, "Invalid address")
			return
		}
	} else if _, found, _ := a.db.ScanInt(ctx, `SELECT id FROM address where name = ?`, address); !found {
		text(w, 400, "Invalid address")
		return
	}
	a.touchAddress(ctx, address)
	st := a.sendBalanceState(r, address, false, true)
	jsonResp(w, 200, map[string]any{"address": address, "send_balance": st.balance})
}

func (a *App) apiNewAddress(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	user := userOf(r)
	if a.cfg.DisableAnonymousUserCreateEmail && user == nil {
		text(w, 403, "Anonymous user create email is disabled")
		return
	}
	if !a.cfg.EnableUserCreateEmail {
		text(w, 403, "New address is disabled")
		return
	}
	var req struct {
		Name                  string `json:"name"`
		Domain                string `json:"domain"`
		EnableRandomSubdomain any    `json:"enableRandomSubdomain"`
	}
	readJSON(r, &req)
	roleName := ""
	if user != nil {
		if role, _ := a.roles.UserRole(ctx, claimInt(user, "user_id")); role != nil {
			roleName = role.Role
			if req.Name != "" && !role.CanCustomName {
				req.Name = ""
			}
		}
		if a.cfg.DisableAnonymousUserCreateEmail || roleName != "" {
			if reached, msg := a.roles.LimitReached(ctx, claimInt(user, "user_id"), roleName); reached {
				text(w, 400, msg)
				return
			}
		}
	}
	if req.Name == "" || a.cfg.DisableCustomAddressName {
		req.Name = a.generateRandomName()
	}
	res, err := a.newAddress(ctx, newAddressOpts{
		name: req.Name, domain: req.Domain, enablePrefix: true,
		enableRandomSubdomain: truthy(req.EnableRandomSubdomain),
		checkLengthByConfig:   true, addressPrefix: a.userRolePrefix(ctx, r),
		allowDomains: a.allowDomainsFor(ctx, r), enableCheckNameRegex: true,
		sourceMeta: clientIP(r, a.cfg.TrustedProxies),
	})
	if err != nil {
		text(w, 400, "Failed to create address: "+err.Error())
		return
	}
	if user != nil && a.cfg.DisableAnonymousUserCreateEmail {
		a.db.Exec(ctx, `INSERT OR IGNORE INTO users_address (user_id, address_id) VALUES (?, ?)`, claimInt(user, "user_id"), res.AddressID)
	}
	jsonResp(w, 200, res)
}

func truthy(v any) bool {
	switch x := v.(type) {
	case bool:
		return x
	case string:
		return x == "true"
	}
	return false
}

func (a *App) apiDeleteAddress(w http.ResponseWriter, r *http.Request) {
	if !a.cfg.EnableUserDeleteEmail {
		text(w, 403, "User delete email is disabled")
		return
	}
	address, addressID := addressOf(r)
	ctx := r.Context()
	if addressID == 0 {
		addressID, _, _ = a.db.ScanInt(ctx, `SELECT id FROM address where name = ?`, address)
	}
	if address == "" || addressID == 0 {
		text(w, 400, "Address not found")
		return
	}
	if err := a.deleteAddressesWhere(ctx, `id = ?`, addressID); err != nil {
		fail(w, err)
		return
	}
	ok(w)
}

func (a *App) apiClearInbox(w http.ResponseWriter, r *http.Request) {
	if !a.cfg.EnableUserDeleteEmail {
		text(w, 403, "User delete email is disabled")
		return
	}
	address, _ := addressOf(r)
	if _, err := a.db.Exec(r.Context(), `DELETE FROM raw_mails WHERE address = ?`, address); err != nil {
		text(w, 500, "Failed to clear inbox")
		return
	}
	ok(w)
}

func (a *App) apiClearSentItems(w http.ResponseWriter, r *http.Request) {
	if !a.cfg.EnableUserDeleteEmail {
		text(w, 403, "User delete email is disabled")
		return
	}
	address, _ := addressOf(r)
	if _, err := a.db.Exec(r.Context(), `DELETE FROM sendbox WHERE address = ?`, address); err != nil {
		text(w, 500, "Failed to clear sent items")
		return
	}
	ok(w)
}

func (a *App) apiChangePassword(w http.ResponseWriter, r *http.Request) {
	if !a.cfg.EnableAddressPassword {
		text(w, 403, "Password change is disabled")
		return
	}
	var req struct {
		NewPassword string `json:"new_password"`
	}
	readJSON(r, &req)
	address, addressID := addressOf(r)
	if req.NewPassword == "" {
		text(w, 400, "New password is required")
		return
	}
	if address == "" || addressID == 0 {
		text(w, 400, "Invalid address token")
		return
	}
	if _, err := a.db.Exec(r.Context(), `UPDATE address SET password = ?, updated_at = datetime('now') WHERE id = ?`, req.NewPassword, addressID); err != nil {
		text(w, 500, "Failed to update password")
		return
	}
	ok(w)
}

func (a *App) apiAddressLogin(w http.ResponseWriter, r *http.Request) {
	if !a.cfg.EnableAddressPassword {
		text(w, 403, "Password login is disabled")
		return
	}
	var req struct {
		Email    string `json:"email"`
		Password string `json:"password"`
	}
	readJSON(r, &req)
	if req.Email == "" || req.Password == "" {
		text(w, 400, "Email and password are required")
		return
	}
	row, err := a.db.QueryOne(r.Context(), `SELECT * FROM address WHERE name = ?`, req.Email)
	if err != nil {
		fail(w, err)
		return
	}
	if row == nil {
		text(w, 404, "Address not found")
		return
	}
	if row.Str("password") != req.Password {
		text(w, 401, "Invalid email or password")
		return
	}
	token, _ := a.jwt.AddressToken(row.Str("name"), row.Int("id"))
	jsonResp(w, 200, map[string]any{"jwt": token, "address": row.Str("name")})
}

func (a *App) apiGetAutoReply(w http.ResponseWriter, r *http.Request) {
	if !a.cfg.EnableAutoReply {
		text(w, 403, "Auto reply is disabled")
		return
	}
	address, _ := addressOf(r)
	row, err := a.db.QueryOne(r.Context(), `SELECT * FROM auto_reply_mails where address = ?`, address)
	if err != nil {
		fail(w, err)
		return
	}
	if row == nil {
		jsonResp(w, 200, map[string]any{})
		return
	}
	jsonResp(w, 200, map[string]any{
		"subject": row["subject"], "message": row["message"], "enabled": row.Int("enabled") == 1,
		"source_prefix": row["source_prefix"], "name": row["name"],
	})
}

func (a *App) apiSaveAutoReply(w http.ResponseWriter, r *http.Request) {
	if !a.cfg.EnableAutoReply {
		text(w, 403, "Auto reply is disabled")
		return
	}
	address, _ := addressOf(r)
	var req struct {
		AutoReply struct {
			Name, Subject, SourcePrefix, Message string
			Enabled                              bool
		} `json:"auto_reply"`
	}
	var raw map[string]json.RawMessage
	readJSON(r, &raw)
	if ar, ok := raw["auto_reply"]; ok {
		var tmp struct {
			Name         string `json:"name"`
			Subject      string `json:"subject"`
			SourcePrefix string `json:"source_prefix"`
			Message      string `json:"message"`
			Enabled      bool   `json:"enabled"`
		}
		json.Unmarshal(ar, &tmp)
		req.AutoReply.Name, req.AutoReply.Subject, req.AutoReply.SourcePrefix, req.AutoReply.Message, req.AutoReply.Enabled =
			tmp.Name, tmp.Subject, tmp.SourcePrefix, tmp.Message, tmp.Enabled
	}
	ar := req.AutoReply
	if (ar.Subject == "" || ar.Message == "") && ar.Enabled {
		text(w, 400, "Subject and message are required when enabled")
		return
	}
	if len(ar.Subject) > 255 || len(ar.Message) > 255 {
		text(w, 400, "Subject or message too long")
		return
	}
	enabled := 0
	if ar.Enabled {
		enabled = 1
	}
	_, err := a.db.Exec(r.Context(), `INSERT OR REPLACE INTO auto_reply_mails (name, address, source_prefix, subject, message, enabled) VALUES (?, ?, ?, ?, ?, ?)`,
		ar.Name, address, ar.SourcePrefix, ar.Subject, ar.Message, enabled)
	if err != nil {
		text(w, 500, "Operation failed")
		return
	}
	ok(w)
}

func (a *App) webhookAllowed(r *http.Request, address string) bool {
	var s adminWebhookSettings
	a.jsonSetting(r.Context(), "temp-mail-webhook-settings", &s)
	return !s.EnableAllowList || contains(s.AllowList, address)
}

func (a *App) apiGetWebhook(w http.ResponseWriter, r *http.Request) {
	if !a.cfg.EnableWebhook {
		text(w, 403, "Webhook is not enabled")
		return
	}
	address, _ := addressOf(r)
	if !a.webhookAllowed(r, address) {
		text(w, 403, "Webhook is not allowed for this address")
		return
	}
	s := defaultWebhookSettings()
	a.jsonSetting(r.Context(), "temp-mail-webhook-user-settings:"+address, &s)
	jsonResp(w, 200, s)
}

func (a *App) apiSaveWebhook(w http.ResponseWriter, r *http.Request) {
	if !a.cfg.EnableWebhook {
		text(w, 403, "Webhook is not enabled")
		return
	}
	address, _ := addressOf(r)
	if !a.webhookAllowed(r, address) {
		text(w, 403, "Webhook is not allowed for this address")
		return
	}
	var s webhookSettings
	readJSON(r, &s)
	if err := a.saveJSONSetting(r.Context(), "temp-mail-webhook-user-settings:"+address, s); err != nil {
		fail(w, err)
		return
	}
	ok(w)
}

func (a *App) apiTestWebhook(w http.ResponseWriter, r *http.Request) {
	if !a.cfg.EnableWebhook {
		text(w, 403, "Webhook is not enabled")
		return
	}
	address, _ := addressOf(r)
	var s webhookSettings
	readJSON(r, &s)
	row, _ := a.db.QueryOne(r.Context(), `SELECT * FROM raw_mails WHERE address = ? ORDER BY RANDOM() LIMIT 1`, address)
	var id any = "0"
	raw := "test raw email"
	if row != nil {
		resolveRaw(row)
		id, raw = row["id"], row.Str("raw")
	}
	vals := a.webhookValues(id, "test@test.com", address, raw, a.parseRawFromRow(row))
	if vals["subject"] == "" {
		vals["subject"] = "test subject"
	}
	if err := sendWebhook(s, vals); err != nil {
		text(w, 400, err.Error())
		return
	}
	ok(w)
}
