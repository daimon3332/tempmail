package server

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"strings"

	"tempmail/internal/mailer"
	"tempmail/internal/roles"
)

func (a *App) adminRoutes() {
	m := a.mux
	m.HandleFunc("GET /admin/address", a.adminListAddresses)
	m.HandleFunc("POST /admin/new_address", a.adminNewAddress)
	m.HandleFunc("DELETE /admin/delete_address/{id}", a.adminDeleteAddress)
	m.HandleFunc("DELETE /admin/clear_inbox/{id}", a.adminClearInbox)
	m.HandleFunc("DELETE /admin/clear_sent_items/{id}", a.adminClearSentItems)
	m.HandleFunc("GET /admin/show_password/{id}", a.adminShowPassword)
	m.HandleFunc("POST /admin/address/{id}/reset_password", a.adminResetAddressPassword)

	m.HandleFunc("GET /admin/mails", a.adminMails)
	m.HandleFunc("GET /admin/workspace_mails", a.workspaceMails)
	m.HandleFunc("GET /admin/workspace_mail/{id}", a.workspaceParsedMail)
	m.HandleFunc("PATCH /admin/workspace_mail/{id}/read", a.workspaceMarkRead)
	m.HandleFunc("DELETE /admin/workspace_mail/{id}", a.workspaceDeleteMail)
	m.HandleFunc("GET /admin/mails_unknow", a.adminUnknownMails)
	m.HandleFunc("GET /admin/mails/{id}", a.adminGetMail)
	m.HandleFunc("DELETE /admin/mails/{id}", a.adminDeleteMail)

	m.HandleFunc("GET /admin/address_sender", a.adminListSender)
	m.HandleFunc("POST /admin/address_sender", a.adminUpdateSender)
	m.HandleFunc("DELETE /admin/address_sender/{id}", a.adminDeleteSender)

	m.HandleFunc("GET /admin/sendbox", a.adminSendbox)
	m.HandleFunc("DELETE /admin/sendbox/{id}", a.adminDeleteSendbox)
	m.HandleFunc("GET /admin/statistics", a.adminStatistics)
	m.HandleFunc("GET /admin/account_settings", a.adminGetAccountSettings)
	m.HandleFunc("POST /admin/account_settings", a.adminSaveAccountSettings)
	m.HandleFunc("GET /admin/auto_reply/rules", a.adminListAutoReplyRules)
	m.HandleFunc("POST /admin/auto_reply/rules", a.adminSaveAutoReplyRule)
	m.HandleFunc("DELETE /admin/auto_reply/rules/{id}", a.adminDeleteAutoReplyRule)
	m.HandleFunc("POST /admin/cleanup", a.adminCleanup)
	m.HandleFunc("GET /admin/auto_cleanup", a.adminGetAutoCleanup)
	m.HandleFunc("POST /admin/auto_cleanup", a.adminSaveAutoCleanup)

	m.HandleFunc("GET /admin/user_settings", a.adminGetUserSettings)
	m.HandleFunc("POST /admin/user_settings", a.adminSaveUserSettings)
	m.HandleFunc("GET /admin/users", a.adminUsers)
	m.HandleFunc("GET /admin/users/{user_id}", a.adminGetUser)
	m.HandleFunc("PATCH /admin/users/{user_id}", a.adminPatchUser)
	m.HandleFunc("DELETE /admin/users/{user_id}", a.adminDeleteUser)
	m.HandleFunc("POST /admin/users", a.adminCreateUser)
	m.HandleFunc("POST /admin/users/{user_id}/reset_password", a.adminResetUserPassword)
	m.HandleFunc("GET /admin/user_limits/{user_id}", a.adminGetUserLimits)
	m.HandleFunc("PATCH /admin/user_limits/{user_id}", a.adminSaveUserLimits)
	m.HandleFunc("GET /admin/user_roles", a.adminUserRoles)
	m.HandleFunc("POST /admin/user_roles", a.adminUpdateUserRole)
	m.HandleFunc("GET /admin/role_address_config", a.adminGetRoleAddressConfig)
	m.HandleFunc("POST /admin/role_address_config", a.adminSaveRoleAddressConfig)
	m.HandleFunc("GET /admin/users/bind_address/{user_id}", a.adminBindedAddresses)
	m.HandleFunc("POST /admin/users/bind_address", a.adminBindAddress)
	m.HandleFunc("POST /admin/users/unbind_address", a.adminUnbindAddress)

	// dynamic roles (extension)
	m.HandleFunc("GET /admin/roles", a.adminListRoles)
	m.HandleFunc("POST /admin/roles", a.adminSaveRole)
	m.HandleFunc("DELETE /admin/roles/{role}", a.adminDeleteRole)

	m.HandleFunc("GET /admin/user_oauth2_settings", a.adminGetOauth2)
	m.HandleFunc("POST /admin/user_oauth2_settings", a.adminSaveOauth2)
	m.HandleFunc("GET /admin/webhook/settings", a.adminGetWebhook)
	m.HandleFunc("POST /admin/webhook/settings", a.adminSaveWebhook)
	m.HandleFunc("GET /admin/mail_webhook/settings", a.adminGetMailWebhook)
	m.HandleFunc("POST /admin/mail_webhook/settings", a.adminSaveMailWebhook)
	m.HandleFunc("POST /admin/mail_webhook/test", a.adminTestMailWebhook)
	m.HandleFunc("GET /admin/mail_webhook/deliveries", a.adminMailWebhookDeliveries)
	m.HandleFunc("GET /admin/worker/configs", a.adminWorkerConfig)
	m.HandleFunc("POST /admin/send_mail", a.adminSendMail)
	m.HandleFunc("POST /admin/send_mail_by_binding", a.adminSendMail)
	m.HandleFunc("GET /admin/db_version", a.adminDBVersion)
	m.HandleFunc("POST /admin/db_initialize", func(w http.ResponseWriter, r *http.Request) {
		jsonResp(w, 200, map[string]string{"message": "Database already initialized"})
	})
	m.HandleFunc("POST /admin/db_migration", func(w http.ResponseWriter, r *http.Request) {
		jsonResp(w, 200, map[string]any{"success": true, "message": "Database does not need migration"})
	})
	m.HandleFunc("GET /admin/config/{key}", a.adminGetConfig)
	m.HandleFunc("POST /admin/config", a.adminSaveConfig)
	m.HandleFunc("GET /admin/db_backup", a.adminDBBackup)
	m.HandleFunc("POST /admin/db_import", a.adminDBImport)
	m.HandleFunc("GET /admin/send_mail_usage", a.adminSendMailUsage)
	m.HandleFunc("POST /admin/send_mail_usage/reset", a.adminSendMailUsageReset)
	m.HandleFunc("GET /admin/address/export", a.adminAddressExport)
	m.HandleFunc("POST /admin/address/import", a.adminAddressImport)
	m.HandleFunc("POST /admin/send_test_mail", a.adminSendTestMail)
	m.HandleFunc("GET /admin/domain_status", a.adminDomainStatus)
	m.HandleFunc("GET /admin/runtime_config", a.adminGetRuntimeConfig)
	m.HandleFunc("POST /admin/runtime_config", a.adminSaveRuntimeConfig)
	m.HandleFunc("GET /admin/system_status", a.adminSystemStatus)
	m.HandleFunc("GET /admin/operation_log", a.operationLogList)
	m.HandleFunc("DELETE /admin/operation_log", a.operationLogClear)
	m.HandleFunc("GET /admin/ip_blacklist/settings", a.adminGetIPBlacklist)
	m.HandleFunc("POST /admin/ip_blacklist/settings", a.adminSaveIPBlacklist)
	m.HandleFunc("GET /admin/ai_extract/settings", a.adminGetAIExtract)
	m.HandleFunc("POST /admin/ai_extract/settings", a.adminSaveAIExtract)
}

func (a *App) adminMailWebhookDeliveries(w http.ResponseWriter, r *http.Request) {
	limit := atoi(r.URL.Query().Get("limit"))
	if limit < 1 || limit > 100 {
		limit = 20
	}
	rows, err := a.db.Query(r.Context(), `SELECT id,event_id,endpoint,attempt,status_code,error,created_at FROM webhook_deliveries ORDER BY id DESC LIMIT ?`, limit)
	if err != nil {
		fail(w, err)
		return
	}
	jsonResp(w, 200, map[string]any{"results": rows, "count": len(rows)})
}

func (a *App) adminListAutoReplyRules(w http.ResponseWriter, r *http.Request) {
	rows, err := a.db.Query(r.Context(), `SELECT id, address, name, source_prefix, subject, message, enabled, created_at FROM auto_reply_mails ORDER BY address`)
	if err != nil {
		fail(w, err)
		return
	}
	jsonResp(w, 200, map[string]any{"results": rows, "count": len(rows)})
}

func (a *App) adminSaveAutoReplyRule(w http.ResponseWriter, r *http.Request) {
	var req struct {
		ID           int64  `json:"id"`
		Address      string `json:"address"`
		Name         string `json:"name"`
		SourcePrefix string `json:"source_prefix"`
		Subject      string `json:"subject"`
		Message      string `json:"message"`
		Enabled      bool   `json:"enabled"`
	}
	if err := readJSON(r, &req); err != nil || strings.TrimSpace(req.Address) == "" {
		text(w, 400, "address is required")
		return
	}
	if len(req.Subject) > 255 || len(req.Message) > 255 {
		text(w, 400, "Subject or message too long")
		return
	}
	address := strings.ToLower(strings.TrimSpace(req.Address))
	if _, found, _ := a.db.ScanInt(r.Context(), `SELECT id FROM address WHERE name=?`, address); !found {
		text(w, 400, "Mailbox does not exist")
		return
	}
	if req.ID > 0 {
		_, err := a.db.Exec(r.Context(), `UPDATE auto_reply_mails SET address=?, name=?, source_prefix=?, subject=?, message=?, enabled=? WHERE id=?`, address, req.Name, req.SourcePrefix, req.Subject, req.Message, boolInt(req.Enabled), req.ID)
		if err != nil {
			fail(w, err)
			return
		}
	} else {
		_, err := a.db.Exec(r.Context(), `INSERT INTO auto_reply_mails(address,name,source_prefix,subject,message,enabled) VALUES(?,?,?,?,?,?) ON CONFLICT(address) DO UPDATE SET name=excluded.name,source_prefix=excluded.source_prefix,subject=excluded.subject,message=excluded.message,enabled=excluded.enabled`, address, req.Name, req.SourcePrefix, req.Subject, req.Message, boolInt(req.Enabled))
		if err != nil {
			fail(w, err)
			return
		}
	}
	ok(w)
}

func (a *App) adminDeleteAutoReplyRule(w http.ResponseWriter, r *http.Request) {
	_, err := a.db.Exec(r.Context(), `DELETE FROM auto_reply_mails WHERE id=?`, atoi(r.PathValue("id")))
	jsonResp(w, 200, map[string]bool{"success": err == nil})
}

func boolInt(v bool) int {
	if v {
		return 1
	}
	return 0
}

func mailerBuild(fromName, from, to, subject, content string) []byte {
	return mailer.Build(mailer.Message{FromName: fromName, From: from, To: to, Subject: subject, Content: content})
}

// storeInternalMail mirrors upstream sendAdminInternalMail: a notification
// written directly to the recipient's inbox.
func (a *App) storeInternalMail(ctx context.Context, to, subject, body string) {
	raw := mailerBuild("Admin", "admin@internal", to, subject, body)
	id := fmt.Sprintf("%x", randomString(12, "abcdefghijklmnopqrstuvwxyz0123456789"))
	a.db.Exec(ctx, `INSERT INTO raw_mails (source, address, raw, message_id) VALUES (?, ?, ?, ?)`, "admin@internal", to, string(raw), id)
}

var addressSortColumns = map[string]string{
	"id": "a.id", "name": "a.name", "created_at": "a.created_at", "updated_at": "a.updated_at",
	"source_meta": "a.source_meta", "mail_count": "mail_count", "send_count": "send_count", "unread_count": "unread_count", "last_mail_at": "last_mail_at",
}

func (a *App) adminListAddresses(w http.ResponseWriter, r *http.Request) {
	q := r.URL.Query()
	col, okc := addressSortColumns[q.Get("sort_by")]
	if !okc {
		col = "a.id"
	}
	dir := "desc"
	if q.Get("sort_order") == "asc" || q.Get("sort_order") == "ascend" {
		dir = "asc"
	}
	sel := `SELECT a.*, (SELECT COUNT(*) FROM raw_mails WHERE address = a.name) AS mail_count, (SELECT COUNT(*) FROM raw_mails WHERE address = a.name AND COALESCE(is_unread,0)=1) AS unread_count, (SELECT MAX(created_at) FROM raw_mails WHERE address = a.name) AS last_mail_at, (SELECT COUNT(*) FROM sendbox WHERE address = a.name) AS send_count, (SELECT GROUP_CONCAT(u.user_email) FROM users u JOIN users_address ua ON ua.user_id=u.id WHERE ua.address_id=a.id) AS owner_emails FROM address a`
	where := []string{"1 = 1"}
	params := []any{}
	if query := strings.TrimSpace(q.Get("query")); query != "" {
		where = append(where, "lower(a.name) LIKE lower(?)")
		params = append(params, "%"+query+"%")
	}
	if domain := strings.TrimSpace(strings.TrimPrefix(q.Get("domain"), "@")); domain != "" {
		where = append(where, "lower(substr(a.name, instr(a.name, '@') + 1)) = lower(?)")
		params = append(params, domain)
	}
	if q.Get("has_mail") == "1" || q.Get("has_mail") == "true" {
		where = append(where, "EXISTS (SELECT 1 FROM raw_mails rm WHERE rm.address=a.name)")
	}
	if q.Get("unread") == "1" || q.Get("unread") == "true" {
		where = append(where, "EXISTS (SELECT 1 FROM raw_mails rm WHERE rm.address=a.name AND COALESCE(rm.is_unread,0)=1)")
	}
	baseWhere := strings.Join(where, " AND ")
	countQuery := `SELECT count(*) as count FROM address a WHERE ` + baseWhere
	a.listQuery(w, r, sel+` WHERE `+baseWhere, countQuery, params, q.Get("limit"), q.Get("offset"), col+" "+dir, "password")
}

func (a *App) adminNewAddress(w http.ResponseWriter, r *http.Request) {
	var req struct {
		Name                  string `json:"name"`
		Domain                string `json:"domain"`
		EnablePrefix          any    `json:"enablePrefix"`
		EnableRandomSubdomain any    `json:"enableRandomSubdomain"`
	}
	readJSON(r, &req)
	if req.Name == "" {
		text(w, 400, "Required field missing")
		return
	}
	res, err := a.newAddress(r.Context(), newAddressOpts{
		name: req.Name, domain: req.Domain, enablePrefix: truthy(req.EnablePrefix),
		enableRandomSubdomain: truthy(req.EnableRandomSubdomain), allowDomains: a.effective(r.Context()).Domains, sourceMeta: "admin",
	})
	if err != nil {
		text(w, 400, "Failed to create address: "+err.Error())
		return
	}
	jsonResp(w, 200, res)
}

func (a *App) adminDeleteAddress(w http.ResponseWriter, r *http.Request) {
	if err := a.deleteAddressesWhere(r.Context(), `id = ?`, atoi(r.PathValue("id"))); err != nil {
		text(w, 500, "Operation failed")
		return
	}
	ok(w)
}

func (a *App) adminClearInbox(w http.ResponseWriter, r *http.Request) {
	_, err := a.db.Exec(r.Context(), `DELETE FROM raw_mails WHERE address IN (select name from address where id = ?)`, atoi(r.PathValue("id")))
	if err != nil {
		text(w, 500, "Operation failed")
		return
	}
	ok(w)
}

func (a *App) adminClearSentItems(w http.ResponseWriter, r *http.Request) {
	_, err := a.db.Exec(r.Context(), `DELETE FROM sendbox WHERE address IN (select name from address where id = ?)`, atoi(r.PathValue("id")))
	if err != nil {
		text(w, 500, "Operation failed")
		return
	}
	ok(w)
}

func (a *App) adminShowPassword(w http.ResponseWriter, r *http.Request) {
	id := atoi(r.PathValue("id"))
	name, _, _ := a.db.ScanString(r.Context(), `SELECT name FROM address WHERE id = ?`, id)
	token, _ := a.jwt.AddressToken(name, id)
	jsonResp(w, 200, map[string]string{"jwt": token})
}

func (a *App) adminResetAddressPassword(w http.ResponseWriter, r *http.Request) {
	if !a.effective(r.Context()).EnableAddressPassword {
		text(w, 403, "Password change is disabled")
		return
	}
	var req struct {
		Password string `json:"password"`
	}
	readJSON(r, &req)
	if req.Password == "" {
		text(w, 400, "New password is required")
		return
	}
	if _, err := a.db.Exec(r.Context(), `UPDATE address SET password = ?, updated_at = datetime('now') WHERE id = ?`, req.Password, atoi(r.PathValue("id"))); err != nil {
		text(w, 500, "Failed to update password")
		return
	}
	ok(w)
}

func (a *App) adminMails(w http.ResponseWriter, r *http.Request) {
	q := r.URL.Query()
	if addr := q.Get("address"); addr != "" {
		where, params := incrementalFilter(r, `address = ?`, []any{addr})
		a.listQuery(w, r, `SELECT * FROM raw_mails where `+where, `SELECT count(*) as count FROM raw_mails where `+where,
			params, q.Get("limit"), q.Get("offset"), "")
		return
	}
	where, params := incrementalFilter(r, `1 = 1`, nil)
	a.listQuery(w, r, `SELECT * FROM raw_mails where `+where, `SELECT count(*) as count FROM raw_mails where `+where, params, q.Get("limit"), q.Get("offset"), "")
}

func (a *App) adminUnknownMails(w http.ResponseWriter, r *http.Request) {
	q := r.URL.Query()
	a.listQuery(w, r, `SELECT * FROM raw_mails where address NOT IN (select name from address)`,
		`SELECT count(*) as count FROM raw_mails where address NOT IN (select name from address)`, nil, q.Get("limit"), q.Get("offset"), "")
}

func (a *App) adminGetMail(w http.ResponseWriter, r *http.Request) {
	row, err := a.db.QueryOne(r.Context(), `SELECT * FROM raw_mails WHERE id = ?`, atoi(r.PathValue("id")))
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

func (a *App) adminDeleteMail(w http.ResponseWriter, r *http.Request) {
	_, err := a.db.Exec(r.Context(), `DELETE FROM raw_mails WHERE id = ?`, atoi(r.PathValue("id")))
	jsonResp(w, 200, map[string]bool{"success": err == nil})
}

func (a *App) adminListSender(w http.ResponseWriter, r *http.Request) {
	q := r.URL.Query()
	if addr := q.Get("address"); addr != "" {
		a.listQuery(w, r, `SELECT * FROM address_sender where address = ?`, `SELECT count(*) as count FROM address_sender where address = ?`,
			[]any{addr}, q.Get("limit"), q.Get("offset"), "")
		return
	}
	a.listQuery(w, r, `SELECT * FROM address_sender`, `SELECT count(*) as count FROM address_sender`, nil, q.Get("limit"), q.Get("offset"), "")
}

func (a *App) adminUpdateSender(w http.ResponseWriter, r *http.Request) {
	var req struct {
		Address   string `json:"address"`
		AddressID int64  `json:"address_id"`
		Balance   int64  `json:"balance"`
		Enabled   bool   `json:"enabled"`
	}
	readJSON(r, &req)
	if req.AddressID == 0 {
		text(w, 400, "Invalid address id")
		return
	}
	enabled := 0
	if req.Enabled {
		enabled = 1
	}
	if _, err := a.db.Exec(r.Context(), `UPDATE address_sender SET enabled = ?, balance = ? WHERE id = ?`, enabled, req.Balance, req.AddressID); err != nil {
		text(w, 500, "Operation failed")
		return
	}
	state := "disabled"
	if req.Enabled {
		state = "enabled"
	}
	a.storeInternalMail(r.Context(), req.Address, "Account Send Access Updated", fmt.Sprintf("Your send access has been %s, balance: %d", state, req.Balance))
	ok(w)
}

func (a *App) adminDeleteSender(w http.ResponseWriter, r *http.Request) {
	_, err := a.db.Exec(r.Context(), `DELETE FROM address_sender WHERE id = ?`, atoi(r.PathValue("id")))
	jsonResp(w, 200, map[string]bool{"success": err == nil})
}

func (a *App) adminSendbox(w http.ResponseWriter, r *http.Request) {
	q := r.URL.Query()
	if addr := q.Get("address"); addr != "" {
		a.listQuery(w, r, `SELECT * FROM sendbox where address = ?`, `SELECT count(*) as count FROM sendbox where address = ?`,
			[]any{addr}, q.Get("limit"), q.Get("offset"), "")
		return
	}
	a.listQuery(w, r, `SELECT * FROM sendbox`, `SELECT count(*) as count FROM sendbox`, nil, q.Get("limit"), q.Get("offset"), "")
}

func (a *App) adminDeleteSendbox(w http.ResponseWriter, r *http.Request) {
	_, err := a.db.Exec(r.Context(), `DELETE FROM sendbox WHERE id = ?`, atoi(r.PathValue("id")))
	jsonResp(w, 200, map[string]bool{"success": err == nil})
}

func (a *App) adminStatistics(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	count := func(q string) int64 { n, _ := a.db.Count(ctx, q); return n }
	jsonResp(w, 200, map[string]any{
		"mailCount":                count(`SELECT count(*) FROM raw_mails`),
		"addressCount":             count(`SELECT count(*) FROM address`),
		"activeAddressCount7days":  count(`SELECT count(*) FROM address where updated_at > datetime('now', '-7 day')`),
		"activeAddressCount30days": count(`SELECT count(*) FROM address where updated_at > datetime('now', '-30 day')`),
		"userCount":                count(`SELECT count(*) FROM users`),
		"sendMailCount":            count(`SELECT count(*) FROM sendbox`),
		"mail_count":               count(`SELECT count(*) FROM raw_mails`),
		"address_count":            count(`SELECT count(*) FROM address`),
		"user_count":               count(`SELECT count(*) FROM users`),
		"sendbox_count":            count(`SELECT count(*) FROM sendbox`),
		"unread_mail_count":        count(`SELECT count(*) FROM raw_mails WHERE COALESCE(is_unread,0)=1`),
	})
}

func (a *App) adminGetAccountSettings(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	var block, sendBlock, verified, fromBlock, noLimit []string
	a.jsonSetting(ctx, "address_block_list", &block)
	a.jsonSetting(ctx, "send_block_list", &sendBlock)
	a.jsonSetting(ctx, "verified_address_list", &verified)
	a.jsonSetting(ctx, "temp-mail-email-black-list", &fromBlock)
	a.jsonSetting(ctx, "no_limit_send_address_list", &noLimit)
	var emailRule map[string]any
	if !a.jsonSetting(ctx, "email_rule_settings", &emailRule) {
		emailRule = map[string]any{}
	}
	var acs struct {
		EnableSubdomainMatch *bool `json:"enableSubdomainMatch"`
	}
	a.jsonSetting(ctx, "address_creation_settings", &acs)
	addrCreation := map[string]any{}
	if acs.EnableSubdomainMatch != nil {
		addrCreation["enableSubdomainMatch"] = *acs.EnableSubdomainMatch
	}
	jsonResp(w, 200, map[string]any{
		"blockList": orEmpty(block), "sendBlockList": orEmpty(sendBlock), "verifiedAddressList": orEmpty(verified),
		"fromBlockList": orEmpty(fromBlock), "noLimitSendAddressList": orEmpty(noLimit), "emailRuleSettings": emailRule,
		"addressCreationSettings": addrCreation,
		"addressCreationSubdomainMatchStatus": map[string]any{
			"envConfigured": true, "envEnabled": a.cfg.EnableCreateAddressSubdomainMatch,
			"storedEnabled": acs.EnableSubdomainMatch, "effectiveEnabled": a.subdomainMatchEnabled(ctx),
		},
		"sendMailLimitConfig": a.sendLimitConfig(ctx),
	})
}

func (a *App) adminSaveAccountSettings(w http.ResponseWriter, r *http.Request) {
	var req struct {
		BlockList               []string        `json:"blockList"`
		SendBlockList           []string        `json:"sendBlockList"`
		VerifiedAddressList     []string        `json:"verifiedAddressList"`
		FromBlockList           []string        `json:"fromBlockList"`
		NoLimitSendAddressList  []string        `json:"noLimitSendAddressList"`
		EmailRuleSettings       json.RawMessage `json:"emailRuleSettings"`
		AddressCreationSettings json.RawMessage `json:"addressCreationSettings"`
		SendMailLimitConfig     json.RawMessage `json:"sendMailLimitConfig"`
	}
	if err := readJSON(r, &req); err != nil || req.BlockList == nil || req.SendBlockList == nil || req.VerifiedAddressList == nil {
		text(w, 400, "Invalid input")
		return
	}
	ctx := r.Context()
	a.saveJSONSetting(ctx, "address_block_list", req.BlockList)
	a.saveJSONSetting(ctx, "send_block_list", req.SendBlockList)
	a.saveJSONSetting(ctx, "verified_address_list", req.VerifiedAddressList)
	a.saveJSONSetting(ctx, "temp-mail-email-black-list", orEmpty(req.FromBlockList))
	a.saveJSONSetting(ctx, "no_limit_send_address_list", orEmpty(req.NoLimitSendAddressList))
	if len(req.EmailRuleSettings) > 0 && string(req.EmailRuleSettings) != "null" {
		a.db.SaveSetting(ctx, "email_rule_settings", string(req.EmailRuleSettings))
	} else {
		a.db.SaveSetting(ctx, "email_rule_settings", "{}")
	}
	if len(req.AddressCreationSettings) > 0 && string(req.AddressCreationSettings) != "null" {
		var acs map[string]any
		if json.Unmarshal(req.AddressCreationSettings, &acs) == nil {
			if v, present := acs["enableSubdomainMatch"]; present {
				if v == nil {
					a.db.DeleteSetting(ctx, "address_creation_settings")
				} else if b, isBool := v.(bool); isBool {
					a.saveJSONSetting(ctx, "address_creation_settings", map[string]bool{"enableSubdomainMatch": b})
				}
			}
		}
	}
	if len(req.SendMailLimitConfig) > 0 && string(req.SendMailLimitConfig) != "null" {
		var c sendMailLimitConfig
		if json.Unmarshal(req.SendMailLimitConfig, &c) == nil {
			if !c.DailyEnabled {
				c.DailyLimit = nil
			}
			if !c.MonthlyEnabled {
				c.MonthlyLimit = nil
			}
			a.saveJSONSetting(ctx, "send_mail_limit_config", c)
		}
	}
	ok(w)
}

func (a *App) adminCleanup(w http.ResponseWriter, r *http.Request) {
	var req struct {
		CleanType  string   `json:"cleanType"`
		CleanTypes []string `json:"cleanTypes"`
		CleanDays  int      `json:"cleanDays"`
	}
	readJSON(r, &req)
	var err error
	if len(req.CleanTypes) > 0 {
		err = a.cleanupTypes(r.Context(), req.CleanTypes, req.CleanDays)
	} else {
		err = a.cleanup(r.Context(), req.CleanType, req.CleanDays)
	}
	if err != nil {
		text(w, 500, "Operation failed: "+err.Error())
		return
	}
	ok(w)
}

func (a *App) adminGetAutoCleanup(w http.ResponseWriter, r *http.Request) {
	s := a.loadCleanupSettings(r.Context())
	jsonResp(w, 200, s)
}

func (a *App) adminSaveAutoCleanup(w http.ResponseWriter, r *http.Request) {
	var s cleanupSettings
	if err := readJSON(r, &s); err != nil {
		text(w, 400, "Invalid input")
		return
	}
	for _, c := range s.CustomSqlCleanupList {
		if c.SQL != "" {
			if err := validateCustomSQL(c.SQL); err != nil {
				text(w, 400, fmt.Sprintf("[%s]: %s", c.Name, err.Error()))
				return
			}
		}
	}
	normalizeCleanupSettings(&s)
	if err := a.saveJSONSetting(r.Context(), "auto_cleanup", s); err != nil {
		fail(w, err)
		return
	}
	ok(w)
}

func (a *App) adminGetUserSettings(w http.ResponseWriter, r *http.Request) {
	jsonResp(w, 200, a.roles.UserSettings(r.Context()))
}

func (a *App) adminSaveUserSettings(w http.ResponseWriter, r *http.Request) {
	var s roles.UserSettings
	if err := readJSON(r, &s); err != nil {
		text(w, 400, "Invalid input")
		return
	}
	if s.EnableMailVerify && s.VerifyMailSender == "" {
		text(w, 400, "Verify mail sender is not set")
		return
	}
	if s.EnableMailVerify && !contains(a.effective(r.Context()).Domains, mailDomain(s.VerifyMailSender)) {
		text(w, 400, "Verify mail sender domain is invalid")
		return
	}
	if s.MaxAddressCount < 0 {
		text(w, 400, "Invalid max address count")
		return
	}
	if s.MailAllowList == nil {
		s.MailAllowList = []string{}
	}
	if err := a.saveJSONSetting(r.Context(), "user_settings", s); err != nil {
		fail(w, err)
		return
	}
	ok(w)
}

func (a *App) adminUsers(w http.ResponseWriter, r *http.Request) {
	q := r.URL.Query()
	limit, offset := atoi(q.Get("limit")), atoi(q.Get("offset"))
	if q.Get("limit") == "" {
		limit = 100
	}
	if limit <= 0 || limit > 100 || (q.Get("offset") != "" && offset < 0) {
		text(w, 400, "Invalid pagination")
		return
	}
	where, params := "1 = 1", []any{}
	if query := strings.TrimSpace(q.Get("query")); query != "" {
		where = "instr(u.user_email, ?) > 0 OR instr(COALESCE(u.username, ''), ?) > 0"
		params = []any{query, query}
	}
	rows, err := a.db.Query(r.Context(), `SELECT u.id, u.user_email, u.username, u.password_ciphertext, u.created_at, u.updated_at,
		ur.role_text, (SELECT COUNT(*) FROM users_address WHERE user_id = u.id) AS address_count,
		(SELECT COUNT(*) FROM raw_mails rm JOIN address aa ON aa.name = rm.address JOIN users_address uaa ON uaa.address_id = aa.id WHERE uaa.user_id = u.id) AS mail_count
		FROM users u LEFT JOIN user_roles ur ON u.id = ur.user_id WHERE `+where+` ORDER BY u.id DESC LIMIT ? OFFSET ?`, append(params, limit, offset)...)
	if err != nil {
		fail(w, err)
		return
	}
	for _, row := range rows {
		role := row.Str("role_text")
		ciphertext := row.Str("password_ciphertext")
		delete(row, "password_ciphertext")
		if role == a.cfg.AdminUserRole {
			row["password"] = nil
		} else if plain, err := a.decryptUserPassword(ciphertext); err == nil {
			row["password"] = plain
		} else {
			row["password"] = nil
		}
	}
	count, _ := a.db.Count(r.Context(), `SELECT count(*) FROM users u WHERE `+where, params...)
	jsonResp(w, 200, map[string]any{"results": rows, "count": count})
}

func (a *App) adminGetUser(w http.ResponseWriter, r *http.Request) {
	id := atoi(r.PathValue("user_id"))
	row, _ := a.db.QueryOne(r.Context(), `SELECT u.id, u.user_email, u.username, u.password_ciphertext, u.created_at, u.updated_at, ur.role_text FROM users u LEFT JOIN user_roles ur ON ur.user_id = u.id WHERE u.id = ?`, id)
	if row == nil {
		text(w, 404, "User not found")
		return
	}
	role := row.Str("role_text")
	ciphertext := row.Str("password_ciphertext")
	delete(row, "password_ciphertext")
	if role == a.cfg.AdminUserRole {
		row["password"] = nil
	} else if plain, err := a.decryptUserPassword(ciphertext); err == nil {
		row["password"] = plain
	}
	limits, _ := a.roles.UserLimits(r.Context(), id)
	row["limits"] = limits
	jsonResp(w, 200, row)
}

func (a *App) adminPatchUser(w http.ResponseWriter, r *http.Request) {
	id := atoi(r.PathValue("user_id"))
	var req struct {
		Username      string            `json:"username"`
		Password      string            `json:"password"`
		PlainPassword string            `json:"password_plain"`
		Limits        *roles.UserLimits `json:"limits"`
	}
	if err := readJSON(r, &req); err != nil {
		text(w, 400, "Invalid input")
		return
	}
	if req.Username != "" {
		if _, err := a.db.Exec(r.Context(), `UPDATE users SET username = ?, updated_at = datetime('now') WHERE id = ?`, strings.TrimSpace(req.Username), id); err != nil {
			text(w, 400, "Username already exists")
			return
		}
	}
	if req.Password != "" {
		plain := req.PlainPassword
		if plain == "" {
			plain = req.Password
		}
		ciphertext, _ := a.encryptUserPassword(plain)
		if _, err := a.db.Exec(r.Context(), `UPDATE users SET password = ?, password_ciphertext = ?, updated_at = datetime('now') WHERE id = ?`, req.Password, ciphertext, id); err != nil {
			text(w, 500, "Failed to update password")
			return
		}
	}
	if req.Limits != nil {
		l := *req.Limits
		if l.MaxAddressCount < -1 || l.MaxMailCount < -1 || l.MonthlyAddressQuota < -1 || l.MonthlyReceiveQuota < -1 {
			text(w, 400, "Invalid limits")
			return
		}
		if err := a.roles.SaveUserLimits(r.Context(), id, l); err != nil {
			text(w, 500, "Failed to save limits")
			return
		}
	}
	ok(w)
}

func (a *App) adminCreateUser(w http.ResponseWriter, r *http.Request) {
	var req struct {
		Email         string            `json:"email"`
		Username      string            `json:"username"`
		Password      string            `json:"password"`
		PlainPassword string            `json:"password_plain"`
		Limits        *roles.UserLimits `json:"limits"`
	}
	readJSON(r, &req)
	if req.Email == "" && req.Username == "" || req.Password == "" || len(req.Password) > 100 {
		text(w, 400, "Invalid email or password")
		return
	}
	if req.Email == "" {
		req.Email = req.Username
	}
	if req.Username == "" {
		req.Username = req.Email
	}
	info := userInfoJSON(r, clientIP(r, a.cfg.TrustedProxies), req.Email)
	plain := req.PlainPassword
	if plain == "" {
		plain = req.Password
	}
	ciphertext, _ := a.encryptUserPassword(plain)
	res, err := a.db.ExecContext(r.Context(), `INSERT INTO users (user_email, username, password, password_ciphertext, user_info) VALUES (?, ?, ?, ?, ?)`, req.Email, req.Username, req.Password, ciphertext, info)
	if err != nil {
		if strings.Contains(err.Error(), "UNIQUE") {
			text(w, 400, "User already exists")
			return
		}
		text(w, 500, "Failed to register: "+err.Error())
		return
	}
	if req.Limits != nil {
		id, _ := res.LastInsertId()
		if err := a.roles.SaveUserLimits(r.Context(), id, *req.Limits); err != nil {
			text(w, 500, "Failed to save limits")
			return
		}
	} else {
		id, _ := res.LastInsertId()
		_ = a.assignDefaultRole(r.Context(), id)
	}
	ok(w)
}

func (a *App) adminGetUserLimits(w http.ResponseWriter, r *http.Request) {
	l, ok := a.roles.UserLimits(r.Context(), atoi(r.PathValue("user_id")))
	if !ok {
		l = roles.UserLimits{MaxAddressCount: -1, MaxMailCount: -1, MonthlyAddressQuota: -1, MonthlyReceiveQuota: -1}
	}
	jsonResp(w, 200, l)
}

func (a *App) adminSaveUserLimits(w http.ResponseWriter, r *http.Request) {
	var l roles.UserLimits
	if err := readJSON(r, &l); err != nil || l.MaxAddressCount < -1 || l.MaxMailCount < -1 || l.MonthlyAddressQuota < -1 || l.MonthlyReceiveQuota < -1 {
		text(w, 400, "Invalid limits")
		return
	}
	if err := a.roles.SaveUserLimits(r.Context(), atoi(r.PathValue("user_id")), l); err != nil {
		text(w, 500, "Failed to save limits")
		return
	}
	ok(w)
}

func (a *App) adminDeleteUser(w http.ResponseWriter, r *http.Request) {
	id := atoi(r.PathValue("user_id"))
	ctx := r.Context()
	var username, email string
	_ = a.db.QueryRowContext(ctx, `SELECT username, user_email FROM users WHERE id = ?`, id).Scan(&username, &email)
	if username == "admin" || email == "admin" {
		text(w, 400, "The admin account cannot be deleted")
		return
	}
	// Remove dependent records as well; otherwise a reused SQLite user id could
	// inherit stale monthly usage and quota data from a deleted account.
	a.db.Exec(ctx, `DELETE FROM users_address WHERE user_id = ?`, id)
	a.db.Exec(ctx, `DELETE FROM user_roles WHERE user_id = ?`, id)
	a.db.Exec(ctx, `DELETE FROM user_limits WHERE user_id = ?`, id)
	a.db.Exec(ctx, `DELETE FROM user_usage_monthly WHERE user_id = ?`, id)
	a.db.Exec(ctx, `DELETE FROM user_passkeys WHERE user_id = ?`, id)
	a.db.Exec(ctx, `DELETE FROM users WHERE id = ?`, id)
	ok(w)
}

func (a *App) adminResetUserPassword(w http.ResponseWriter, r *http.Request) {
	var req struct {
		Password      string `json:"password"`
		PlainPassword string `json:"password_plain"`
	}
	readJSON(r, &req)
	if req.Password == "" || len(req.Password) > 100 {
		text(w, 500, "Failed to update password: Invalid password")
		return
	}
	plain := req.PlainPassword
	if plain == "" {
		plain = req.Password
	}
	ciphertext, _ := a.encryptUserPassword(plain)
	if _, err := a.db.Exec(r.Context(), `UPDATE users SET password = ?, password_ciphertext = ?, updated_at = datetime('now') WHERE id = ?`, req.Password, ciphertext, atoi(r.PathValue("user_id"))); err != nil {
		text(w, 500, "Failed to update password")
		return
	}
	ok(w)
}

func (a *App) adminUserRoles(w http.ResponseWriter, r *http.Request) {
	list, err := a.roles.List(r.Context())
	if err != nil {
		fail(w, err)
		return
	}
	jsonResp(w, 200, list)
}

func (a *App) adminUpdateUserRole(w http.ResponseWriter, r *http.Request) {
	var req struct {
		UserID   int64  `json:"user_id"`
		RoleText string `json:"role_text"`
	}
	readJSON(r, &req)
	if req.UserID == 0 {
		text(w, 400, "Invalid user id")
		return
	}
	ctx := r.Context()
	if req.RoleText == "" {
		a.db.Exec(ctx, `DELETE FROM user_roles WHERE user_id = ?`, req.UserID)
		ok(w)
		return
	}
	if !a.roles.Exists(ctx, req.RoleText) {
		text(w, 400, "Invalid role text")
		return
	}
	if _, err := a.db.Exec(ctx, `INSERT INTO user_roles (user_id, role_text) VALUES (?, ?) ON CONFLICT(user_id) DO UPDATE SET role_text = excluded.role_text, updated_at = datetime('now')`, req.UserID, req.RoleText); err != nil {
		text(w, 500, "Failed to update user role")
		return
	}
	ok(w)
}

func (a *App) adminGetRoleAddressConfig(w http.ResponseWriter, r *http.Request) {
	var cfg map[string]any
	if !a.jsonSetting(r.Context(), "role_address_config", &cfg) {
		cfg = map[string]any{}
	}
	jsonResp(w, 200, map[string]any{"configs": cfg})
}

func (a *App) adminSaveRoleAddressConfig(w http.ResponseWriter, r *http.Request) {
	var req struct {
		Configs map[string]struct {
			MaxAddressCount *int `json:"maxAddressCount"`
		} `json:"configs"`
	}
	if err := readJSON(r, &req); err != nil || req.Configs == nil {
		text(w, 400, "Invalid max address count")
		return
	}
	for _, c := range req.Configs {
		if c.MaxAddressCount != nil && *c.MaxAddressCount < 0 {
			text(w, 400, "Invalid max address count")
			return
		}
	}
	a.saveJSONSetting(r.Context(), "role_address_config", req.Configs)
	ok(w)
}

func (a *App) adminBindedAddresses(w http.ResponseWriter, r *http.Request) {
	rows, err := a.db.Query(r.Context(),
		`SELECT a.*, (SELECT COUNT(*) FROM raw_mails WHERE address = a.name) AS mail_count, (SELECT COUNT(*) FROM sendbox WHERE address = a.name) AS send_count FROM address a JOIN users_address ua ON ua.address_id = a.id WHERE ua.user_id = ? ORDER BY a.id DESC`,
		atoi(r.PathValue("user_id")))
	if err != nil {
		fail(w, err)
		return
	}
	for _, row := range rows {
		delete(row, "password")
	}
	jsonResp(w, 200, map[string]any{"results": rows})
}

func (a *App) adminBindAddress(w http.ResponseWriter, r *http.Request) {
	var req struct {
		UserEmail string `json:"user_email"`
		Address   string `json:"address"`
		UserID    int64  `json:"user_id"`
		AddressID int64  `json:"address_id"`
	}
	readJSON(r, &req)
	ctx := r.Context()
	if req.UserID == 0 {
		req.UserID, _, _ = a.db.ScanInt(ctx, `SELECT id FROM users WHERE user_email = ?`, req.UserEmail)
	}
	if req.AddressID == 0 {
		req.AddressID, _, _ = a.db.ScanInt(ctx, `SELECT id FROM address WHERE name = ?`, req.Address)
	}
	a.bindByID(w, r, req.UserID, req.AddressID, "")
}

func (a *App) adminUnbindAddress(w http.ResponseWriter, r *http.Request) {
	var req struct {
		UserID    int64 `json:"user_id"`
		AddressID int64 `json:"address_id"`
	}
	if err := readJSON(r, &req); err != nil || req.UserID <= 0 || req.AddressID <= 0 {
		text(w, 400, "Invalid user or address")
		return
	}
	if _, err := a.db.Exec(r.Context(), `DELETE FROM users_address WHERE user_id = ? AND address_id = ?`, req.UserID, req.AddressID); err != nil {
		text(w, 500, "Operation failed")
		return
	}
	ok(w)
}

func (a *App) adminListRoles(w http.ResponseWriter, r *http.Request) {
	list, err := a.roles.List(r.Context())
	if err != nil {
		fail(w, err)
		return
	}
	jsonResp(w, 200, map[string]any{"results": list})
}

func (a *App) adminSaveRole(w http.ResponseWriter, r *http.Request) {
	var role roles.Role
	role.MaxAddressCount, role.MonthlyAddressQuota, role.CanCustomName, role.CanSendMail = -1, -1, true, true
	if err := readJSON(r, &role); err != nil || strings.TrimSpace(role.Role) == "" {
		text(w, 400, "Invalid role")
		return
	}
	if err := a.roles.Save(r.Context(), role); err != nil {
		fail(w, err)
		return
	}
	ok(w)
}

func (a *App) adminDeleteRole(w http.ResponseWriter, r *http.Request) {
	name := r.PathValue("role")
	ctx := r.Context()
	if err := a.roles.Delete(ctx, name); err != nil {
		fail(w, err)
		return
	}
	if !a.roles.Exists(ctx, name) {
		a.db.Exec(ctx, `DELETE FROM user_roles WHERE role_text = ?`, name)
	}
	ok(w)
}

func (a *App) adminGetOauth2(w http.ResponseWriter, r *http.Request) {
	s := a.oauth2Settings(r.Context())
	if s == nil {
		s = []oauth2Setting{}
	}
	jsonResp(w, 200, s)
}

func (a *App) adminSaveOauth2(w http.ResponseWriter, r *http.Request) {
	var s []oauth2Setting
	if err := readJSON(r, &s); err != nil {
		text(w, 400, "Invalid input")
		return
	}
	a.saveJSONSetting(r.Context(), "oauth2_settings", s)
	ok(w)
}

func (a *App) adminGetWebhook(w http.ResponseWriter, r *http.Request) {
	s := adminWebhookSettings{AllowList: []string{}}
	a.jsonSetting(r.Context(), "temp-mail-webhook-settings", &s)
	if s.AllowList == nil {
		s.AllowList = []string{}
	}
	jsonResp(w, 200, s)
}

func (a *App) adminSaveWebhook(w http.ResponseWriter, r *http.Request) {
	var s adminWebhookSettings
	readJSON(r, &s)
	a.saveJSONSetting(r.Context(), "temp-mail-webhook-settings", s)
	ok(w)
}

func (a *App) adminGetMailWebhook(w http.ResponseWriter, r *http.Request) {
	s := defaultWebhookSettings()
	a.jsonSetting(r.Context(), "temp-mail-webhook-admin-mail-settings", &s)
	jsonResp(w, 200, s)
}

func (a *App) adminSaveMailWebhook(w http.ResponseWriter, r *http.Request) {
	var s webhookSettings
	if err := readJSON(r, &s); err != nil {
		text(w, 400, "Invalid input")
		return
	}
	if err := validateWebhookSettings(s); err != nil {
		text(w, 400, err.Error())
		return
	}
	a.saveJSONSetting(r.Context(), "temp-mail-webhook-admin-mail-settings", s)
	ok(w)
}

func (a *App) adminTestMailWebhook(w http.ResponseWriter, r *http.Request) {
	var s webhookSettings
	if err := readJSON(r, &s); err != nil {
		text(w, 400, "Invalid input")
		return
	}
	if err := validateWebhookSettings(s); err != nil {
		text(w, 400, err.Error())
		return
	}
	row, _ := a.db.QueryOne(r.Context(), `SELECT * FROM raw_mails ORDER BY RANDOM() LIMIT 1`)
	var id any = "0"
	raw := "test raw email"
	if row != nil {
		resolveRaw(row)
		id, raw = row["id"], row.Str("raw")
	}
	vals := a.webhookValues(id, "test@test.com", "admin@test.com", raw, a.parseRawFromRow(row), nil)
	if vals["subject"] == "" {
		vals["subject"] = "test subject"
	}
	if _, err := sendWebhook(s, vals); err != nil {
		text(w, 400, err.Error())
		return
	}
	ok(w)
}

func (a *App) adminWorkerConfig(w http.ResponseWriter, r *http.Request) {
	list, _ := a.roles.List(r.Context())
	rc := a.effective(r.Context())
	jsonResp(w, 200, map[string]any{
		"DEFAULT_LANG": a.cfg.DefaultLang, "TITLE": a.cfg.Title,
		"HAS_PASSWORD": len(a.cfg.Passwords), "HAS_ADMIN_PASSWORDS": len(a.cfg.AdminPasswords),
		"ANNOUNCEMENT": a.cfg.Announcement, "ALWAYS_SHOW_ANNOUNCEMENT": false,
		"PREFIX": a.cfg.Prefix, "ADDRESS_CHECK_REGEX": a.cfg.AddressCheckRegex, "ADDRESS_REGEX": a.cfg.AddressRegex,
		"MIN_ADDRESS_LEN": a.cfg.MinAddressLen, "MAX_ADDRESS_LEN": a.cfg.MaxAddressLen,
		"FORWARD_ADDRESS_LIST": orEmpty(a.cfg.ForwardAddressList), "SUBDOMAIN_FORWARD_ADDRESS_LIST": nil,
		"DEFAULT_DOMAINS": rc.DefaultDomains, "DOMAINS": rc.Domains,
		"ENABLE_CREATE_ADDRESS_SUBDOMAIN_MATCH": a.cfg.EnableCreateAddressSubdomainMatch,
		"RANDOM_SUBDOMAIN_DOMAINS":              a.cfg.RandomSubdomainDomains, "RANDOM_SUBDOMAIN_LENGTH": a.cfg.RandomSubdomainLength,
		"DOMAIN_LABELS": orEmpty(a.cfg.DomainLabels), "HAS_JWT_SECRET": true,
		"ADMIN_USER_ROLE": a.cfg.AdminUserRole, "USER_DEFAULT_ROLE": a.cfg.UserDefaultRole, "USER_ROLES": list,
		"NO_LIMIT_SEND_ROLE": orEmpty(a.cfg.NoLimitSendRole), "ADMIN_CONTACT": a.cfg.AdminContact,
		"ENABLE_USER_CREATE_EMAIL": a.cfg.EnableUserCreateEmail, "DISABLE_ANONYMOUS_USER_CREATE_EMAIL": a.cfg.DisableAnonymousUserCreateEmail,
		"ENABLE_USER_DELETE_EMAIL": a.cfg.EnableUserDeleteEmail, "ENABLE_MAIL_READ_STATUS": a.cfg.EnableMailReadStatus,
		"ENABLE_AUTO_REPLY": a.cfg.EnableAutoReply, "COPYRIGHT": a.cfg.Copyright, "ENABLE_WEBHOOK": a.cfg.EnableWebhook,
		"S3_ENABLED": a.cfg.S3Enabled(), "VERSION": "v1.12.0", "DISABLE_SHOW_GITHUB": a.cfg.DisableShowGithub,
		"DISABLE_SHOW_GITHUB_FOR_USER": a.cfg.DisableShowGithub, "DISABLE_ADMIN_PASSWORD_CHECK": a.cfg.DisableAdminPasswordCheck,
		"ENABLE_CHECK_JUNK_MAIL": a.cfg.EnableCheckJunkMail, "JUNK_MAIL_CHECK_LIST": orEmpty(a.cfg.JunkMailCheckList),
		"JUNK_MAIL_FORCE_PASS_LIST": []string{}, "REMOVE_EXCEED_SIZE_ATTACHMENT": false, "REMOVE_ALL_ATTACHMENT": false,
		"ENABLE_ANOTHER_WORKER": false, "ANOTHER_WORKER_LIST": []any{},
	})
}

func (a *App) adminSendMail(w http.ResponseWriter, r *http.Request) {
	var req struct {
		sendMailReq
		FromMail string `json:"from_mail"`
	}
	readJSON(r, &req)
	if req.FromMail == "" {
		text(w, 400, "Invalid from mail")
		return
	}
	if err := a.sendMail(r, req.FromMail, req.sendMailReq, true); err != nil {
		text(w, 400, "Failed to send mail "+err.Error())
		return
	}
	jsonResp(w, 200, map[string]string{"status": "ok"})
}

func (a *App) adminDBVersion(w http.ResponseWriter, r *http.Request) {
	v, _ := a.db.GetSetting(r.Context(), "db_version")
	jsonResp(w, 200, map[string]any{
		"need_initialization": v == "", "need_migration": false,
		"current_db_version": v, "code_db_version": v, "database_size": nil,
	})
}

var configKeyRe = regexpMustCompile(`^[a-zA-Z0-9][a-zA-Z0-9._-]{0,63}$`)

func (a *App) adminGetConfig(w http.ResponseWriter, r *http.Request) {
	key := r.PathValue("key")
	if !configKeyRe.MatchString(key) {
		text(w, 400, "Invalid config key")
		return
	}
	v, _ := a.db.GetSetting(r.Context(), "admin-config:"+key)
	var value any
	if v != "" {
		value = v
	}
	jsonResp(w, 200, map[string]any{"key": key, "value": value})
}

func (a *App) adminSaveConfig(w http.ResponseWriter, r *http.Request) {
	var req struct {
		Key   string `json:"key"`
		Value any    `json:"value"`
	}
	readJSON(r, &req)
	if !configKeyRe.MatchString(req.Key) {
		text(w, 400, "Invalid config key")
		return
	}
	val, isStr := req.Value.(string)
	if !isStr {
		text(w, 400, "Config value must be a string")
		return
	}
	a.db.SaveSetting(r.Context(), "admin-config:"+req.Key, val)
	jsonResp(w, 200, map[string]any{"success": true, "key": req.Key, "value": val})
}

func (a *App) adminGetIPBlacklist(w http.ResponseWriter, r *http.Request) {
	var s any
	if !a.jsonSetting(r.Context(), "ip_blacklist_settings", &s) {
		s = map[string]any{"enabled": false, "blacklist": []string{}, "asnBlacklist": []string{}, "fingerprintBlacklist": []string{},
			"enableWhitelist": false, "whitelist": []string{}, "enableDailyLimit": false, "dailyRequestLimit": 1000}
	}
	jsonResp(w, 200, s)
}

func (a *App) adminSaveIPBlacklist(w http.ResponseWriter, r *http.Request) {
	var s map[string]any
	if err := readJSON(r, &s); err != nil {
		text(w, 400, "Invalid IP blacklist setting")
		return
	}
	a.saveJSONSetting(r.Context(), "ip_blacklist_settings", s)
	ok(w)
}

func (a *App) adminGetAIExtract(w http.ResponseWriter, r *http.Request) {
	var s any
	if !a.jsonSetting(r.Context(), "ai_extract_settings", &s) {
		s = map[string]any{"enabled": false}
	}
	jsonResp(w, 200, s)
}

func (a *App) adminSaveAIExtract(w http.ResponseWriter, r *http.Request) {
	var s map[string]any
	if err := readJSON(r, &s); err != nil {
		text(w, 400, "Invalid input")
		return
	}
	a.saveJSONSetting(r.Context(), "ai_extract_settings", s)
	ok(w)
}
