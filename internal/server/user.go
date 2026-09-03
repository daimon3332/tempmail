package server

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"regexp"
	"strings"
	"time"
)

func (a *App) userRoutes() {
	m := a.mux
	m.HandleFunc("GET /user_api/open_settings", a.userOpenSettings)
	m.HandleFunc("GET /user_api/settings", a.userSettings)
	m.HandleFunc("GET /user_api/mails", a.userMails)
	m.HandleFunc("DELETE /user_api/mails/{id}", a.userDeleteMail)
	m.HandleFunc("GET /user_api/address/{address_id}/settings", a.userSendSettings)
	m.HandleFunc("POST /user_api/address/{address_id}/request_send_mail_access", a.userRequestSendAccess)
	m.HandleFunc("POST /user_api/address/{address_id}/send_mail", a.userSendMail)
	m.HandleFunc("GET /user_api/sendbox", a.userSendbox)
	m.HandleFunc("DELETE /user_api/sendbox/{id}", a.userDeleteSendbox)
	m.HandleFunc("POST /user_api/login", a.userLogin)
	m.HandleFunc("POST /user_api/verify_code", a.userVerifyCode)
	m.HandleFunc("POST /user_api/register", a.userRegister)
	m.HandleFunc("GET /user_api/oauth2/login_url", a.oauth2LoginURL)
	m.HandleFunc("POST /user_api/oauth2/callback", a.oauth2Callback)
	m.HandleFunc("GET /user_api/bind_address", a.userBindedAddresses)
	m.HandleFunc("POST /user_api/bind_address", a.userBindAddress)
	m.HandleFunc("GET /user_api/bind_address_jwt/{address_id}", a.userBindedAddressJWT)
	m.HandleFunc("POST /user_api/unbind_address", a.userUnbindAddress)
	m.HandleFunc("POST /user_api/transfer_address", a.userTransferAddress)
	m.HandleFunc("GET /user_api/passkey", func(w http.ResponseWriter, r *http.Request) { jsonResp(w, 200, map[string]any{"results": []any{}}) })
	m.HandleFunc("/user_api/passkey/", func(w http.ResponseWriter, r *http.Request) { text(w, 400, "Passkey is not supported") })
}

type oauth2Setting struct {
	Name                string   `json:"name"`
	Icon                string   `json:"icon,omitempty"`
	ClientID            string   `json:"clientID"`
	ClientSecret        string   `json:"clientSecret"`
	AuthorizationURL    string   `json:"authorizationURL"`
	AccessTokenURL      string   `json:"accessTokenURL"`
	AccessTokenFormat   string   `json:"accessTokenFormat"`
	UserInfoURL         string   `json:"userInfoURL"`
	RedirectURL         string   `json:"redirectURL"`
	LogoutURL           string   `json:"logoutURL,omitempty"`
	UserEmailKey        string   `json:"userEmailKey"`
	EnableEmailFormat   bool     `json:"enableEmailFormat,omitempty"`
	UserEmailFormat     string   `json:"userEmailFormat,omitempty"`
	UserEmailReplace    string   `json:"userEmailReplace,omitempty"`
	Scope               string   `json:"scope"`
	EnableMailAllowList bool     `json:"enableMailAllowList,omitempty"`
	MailAllowList       []string `json:"mailAllowList,omitempty"`
}

func (a *App) oauth2Settings(ctx context.Context) []oauth2Setting {
	var s []oauth2Setting
	a.jsonSetting(ctx, "oauth2_settings", &s)
	return s
}

func (a *App) userOpenSettings(w http.ResponseWriter, r *http.Request) {
	us := a.roles.UserSettings(r.Context())
	ids := []map[string]any{}
	for _, s := range a.oauth2Settings(r.Context()) {
		ids = append(ids, map[string]any{"clientID": s.ClientID, "name": s.Name, "icon": s.Icon})
	}
	jsonResp(w, 200, map[string]any{"enable": us.Enable, "enableMailVerify": us.EnableMailVerify, "oauth2ClientIDs": ids})
}

func (a *App) userSettings(w http.ResponseWriter, r *http.Request) {
	u := userOf(r)
	ctx := r.Context()
	userID := claimInt(u, "user_id")
	if _, found, _ := a.db.ScanInt(ctx, `SELECT id FROM users where id = ?`, userID); !found {
		text(w, 400, "User not found")
		return
	}
	role, _ := a.roles.UserRole(ctx, userID)
	var roleName string
	var rolePayload any
	if role != nil {
		roleName = role.Role
		rolePayload = map[string]any{"role": role.Role, "domains": role.Domains, "prefix": role.Prefix,
			"max_address_count": role.MaxAddressCount, "monthly_address_quota": role.MonthlyAddressQuota,
			"can_custom_name": role.CanCustomName, "can_send_mail": role.CanSendMail, "name": role.Name}
	}
	isAdmin := a.cfg.AdminUserRole != "" && a.cfg.AdminUserRole == roleName
	var accessToken any
	if roleName != "" {
		accessToken, _ = a.jwt.AccessToken(claimStr(u, "user_email"), userID, roleName)
	}
	var newToken any
	if claimInt(u, "exp") <= time.Now().Unix()+7*24*3600 {
		newToken, _ = a.jwt.UserToken(claimStr(u, "user_email"), userID)
	}
	a.touchUserAddresses(ctx, userID)
	out := map[string]any{}
	for k, v := range u {
		out[k] = v
	}
	out["is_admin"] = isAdmin
	out["access_token"] = accessToken
	out["new_user_token"] = newToken
	out["user_role"] = rolePayload
	jsonResp(w, 200, out)
}

func (a *App) userMails(w http.ResponseWriter, r *http.Request) {
	u := userOf(r)
	q := r.URL.Query()
	where := []string{"ua.user_id = ?"}
	params := []any{claimInt(u, "user_id")}
	if addr := q.Get("address"); addr != "" {
		where = append(where, "rm.address = ?")
		params = append(params, addr)
	}
	from := ` FROM users_address ua JOIN address a ON a.id = ua.address_id JOIN raw_mails rm ON rm.address = a.name WHERE ` + strings.Join(where, " AND ")
	a.listQuery(w, r, `SELECT rm.*`+from, `SELECT count(*) as count`+from, params, q.Get("limit"), q.Get("offset"), "rm.id desc")
}

func (a *App) userDeleteMail(w http.ResponseWriter, r *http.Request) {
	if !a.cfg.EnableUserDeleteEmail {
		text(w, 403, "User delete email is disabled")
		return
	}
	u := userOf(r)
	_, err := a.db.Exec(r.Context(), `DELETE FROM raw_mails WHERE id = ? AND EXISTS (SELECT 1 FROM users_address ua JOIN address a ON a.id = ua.address_id WHERE ua.user_id = ? AND a.name = raw_mails.address)`,
		atoi(r.PathValue("id")), claimInt(u, "user_id"))
	jsonResp(w, 200, map[string]bool{"success": err == nil})
}

func (a *App) bindedAddress(ctx context.Context, userID, addressID int64) string {
	name, _, _ := a.db.ScanString(ctx, `SELECT a.name FROM users_address ua JOIN address a ON a.id = ua.address_id WHERE ua.user_id = ? AND ua.address_id = ?`, userID, addressID)
	return name
}

func (a *App) userSendSettings(w http.ResponseWriter, r *http.Request) {
	u := userOf(r)
	addr := a.bindedAddress(r.Context(), claimInt(u, "user_id"), atoi(r.PathValue("address_id")))
	if addr == "" {
		text(w, 400, "Address not binded")
		return
	}
	st := a.sendBalanceState(r, addr, false, true)
	jsonResp(w, 200, map[string]any{"address": addr, "send_balance": st.balance})
}

func (a *App) userRequestSendAccess(w http.ResponseWriter, r *http.Request) {
	u := userOf(r)
	addr := a.bindedAddress(r.Context(), claimInt(u, "user_id"), atoi(r.PathValue("address_id")))
	if addr == "" {
		text(w, 400, "Address not binded")
		return
	}
	token, _ := a.jwt.AddressToken(addr, atoi(r.PathValue("address_id")))
	r2 := r.Clone(r.Context())
	r2.Header.Set("Authorization", "Bearer "+token)
	c, _ := a.bearer(r2)
	a.apiRequestSendAccess(w, r.WithContext(context.WithValue(r.Context(), ctxAddress, c)))
}

func (a *App) userSendMail(w http.ResponseWriter, r *http.Request) {
	u := userOf(r)
	addr := a.bindedAddress(r.Context(), claimInt(u, "user_id"), atoi(r.PathValue("address_id")))
	if addr == "" {
		text(w, 400, "Address not binded")
		return
	}
	var req sendMailReq
	readJSON(r, &req)
	if err := a.sendMail(r, addr, req, false); err != nil {
		text(w, 400, "Failed to send mail "+err.Error())
		return
	}
	jsonResp(w, 200, map[string]string{"status": "ok"})
}

func (a *App) userSendbox(w http.ResponseWriter, r *http.Request) {
	u := userOf(r)
	q := r.URL.Query()
	where := []string{"ua.user_id = ?"}
	params := []any{claimInt(u, "user_id")}
	if addr := q.Get("address"); addr != "" {
		where = append(where, "sb.address = ?")
		params = append(params, addr)
	}
	from := ` FROM users_address ua JOIN address a ON a.id = ua.address_id JOIN sendbox sb ON sb.address = a.name WHERE ` + strings.Join(where, " AND ")
	a.listQuery(w, r, `SELECT sb.*`+from, `SELECT count(*) as count`+from, params, q.Get("limit"), q.Get("offset"), "sb.id desc")
}

func (a *App) userDeleteSendbox(w http.ResponseWriter, r *http.Request) {
	if !a.cfg.EnableUserDeleteEmail {
		text(w, 403, "User delete email is disabled")
		return
	}
	u := userOf(r)
	_, err := a.db.Exec(r.Context(), `DELETE FROM sendbox WHERE id = ? AND EXISTS (SELECT 1 FROM users_address ua JOIN address a ON a.id = ua.address_id WHERE ua.user_id = ? AND a.name = sendbox.address)`,
		atoi(r.PathValue("id")), claimInt(u, "user_id"))
	jsonResp(w, 200, map[string]bool{"success": err == nil})
}

func (a *App) checkUserEmail(ctx context.Context, email string) error {
	us := a.roles.UserSettings(ctx)
	if us.EnableMailAllowList && len(us.MailAllowList) > 0 && !contains(us.MailAllowList, mailDomain(email)) {
		b, _ := json.MarshalIndent(us.MailAllowList, "", "  ")
		return fmt.Errorf("User mail domain must in %s", b)
	}
	if us.EnableEmailCheckRegex && us.EmailCheckRegex != "" {
		if re, err := regexp.Compile(us.EmailCheckRegex); err == nil && !re.MatchString(email) {
			return fmt.Errorf("User email not match regex: /%s/", us.EmailCheckRegex)
		}
	}
	return nil
}

func (a *App) userVerifyCode(w http.ResponseWriter, r *http.Request) {
	var req struct {
		Email string `json:"email"`
	}
	readJSON(r, &req)
	ctx := r.Context()
	if err := a.checkUserEmail(ctx, req.Email); err != nil {
		text(w, 400, err.Error())
		return
	}
	us := a.roles.UserSettings(ctx)
	if us.VerifyMailSender == "" {
		text(w, 400, "Verify mail sender is not set")
		return
	}
	key := "temp-mail:" + req.Email
	if v, _ := a.db.GetSetting(ctx, key); v != "" {
		var rec struct{ Exp int64 }
		if json.Unmarshal([]byte(v), &rec) == nil && rec.Exp > time.Now().Unix() {
			text(w, 400, "Code already sent")
			return
		}
	}
	code := randomString(6, "0123456789")
	raw := mailerBuild("Temp Mail Verify", us.VerifyMailSender, req.Email, "Temp Mail Verify code", "Your verify code is "+code)
	if err := a.mailer.Send(us.VerifyMailSender, req.Email, raw); err != nil {
		text(w, 500, "Failed to send verify code: "+err.Error())
		return
	}
	rec, _ := json.Marshal(map[string]any{"code": code, "exp": time.Now().Unix() + 300})
	a.db.SaveSetting(ctx, key, string(rec))
	jsonResp(w, 200, map[string]any{"success": true, "expirationTtl": 300})
}

func (a *App) verifyCode(ctx context.Context, email, code string) bool {
	v, _ := a.db.GetSetting(ctx, "temp-mail:"+email)
	if v == "" {
		return false
	}
	var rec struct {
		Code string
		Exp  int64
	}
	if json.Unmarshal([]byte(v), &rec) != nil || rec.Exp < time.Now().Unix() {
		return false
	}
	return rec.Code == code
}

func (a *App) assignDefaultRole(ctx context.Context, userID int64) error {
	if a.cfg.UserDefaultRole == "" {
		return nil
	}
	if !a.roles.Exists(ctx, a.cfg.UserDefaultRole) {
		return errors.New("Invalid user default role")
	}
	_, err := a.db.Exec(ctx, `INSERT INTO user_roles (user_id, role_text) VALUES (?, ?) ON CONFLICT(user_id) DO NOTHING`, userID, a.cfg.UserDefaultRole)
	return err
}

func userInfoJSON(r *http.Request, ip, email string) string {
	b, _ := json.Marshal(map[string]any{"geoData": map[string]any{"ip": ip}, "userEmail": email})
	return string(b)
}

func (a *App) userRegister(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	us := a.roles.UserSettings(ctx)
	if !us.Enable {
		text(w, 403, "User registration is disabled")
		return
	}
	var req struct {
		Email    string `json:"email"`
		Password string `json:"password"`
		Code     string `json:"code"`
	}
	readJSON(r, &req)
	if req.Email == "" || req.Password == "" {
		text(w, 400, "Invalid email or password")
		return
	}
	if len(req.Password) > 100 {
		text(w, 400, "Invalid password")
		return
	}
	if us.EnableMailVerify && req.Code == "" {
		text(w, 400, "Invalid verify code")
		return
	}
	if err := a.checkUserEmail(ctx, req.Email); err != nil {
		text(w, 400, err.Error())
		return
	}
	if us.EnableMailVerify && !a.verifyCode(ctx, req.Email, req.Code) {
		text(w, 400, "Invalid verify code")
		return
	}
	info := userInfoJSON(r, clientIP(r, a.cfg.TrustedProxies), req.Email)
	if !us.EnableMailVerify {
		if _, err := a.db.Exec(ctx, `INSERT INTO users (user_email, password, user_info) VALUES (?, ?, ?)`, req.Email, req.Password, info); err != nil {
			if strings.Contains(err.Error(), "UNIQUE") {
				text(w, 400, "User already exists")
				return
			}
			text(w, 500, "Failed to register: "+err.Error())
			return
		}
		id, _, _ := a.db.ScanInt(ctx, `SELECT id FROM users where user_email = ?`, req.Email)
		a.assignDefaultRole(ctx, id)
		ok(w)
		return
	}
	if _, err := a.db.Exec(ctx, `INSERT INTO users (user_email, password, user_info) VALUES (?, ?, ?)
		ON CONFLICT(user_email) DO UPDATE SET password = excluded.password, user_info = excluded.user_info, updated_at = datetime('now')`,
		req.Email, req.Password, info); err != nil {
		text(w, 400, "Failed to register")
		return
	}
	a.db.DeleteSetting(ctx, "temp-mail:"+req.Email)
	id, _, _ := a.db.ScanInt(ctx, `SELECT id FROM users where user_email = ?`, req.Email)
	if err := a.assignDefaultRole(ctx, id); err != nil {
		text(w, 500, err.Error())
		return
	}
	ok(w)
}

func (a *App) userLogin(w http.ResponseWriter, r *http.Request) {
	var req struct {
		Email    string `json:"email"`
		Password string `json:"password"`
	}
	readJSON(r, &req)
	if req.Email == "" || req.Password == "" {
		text(w, 400, "Invalid email or password")
		return
	}
	row, _ := a.db.QueryOne(r.Context(), `SELECT id, password FROM users where user_email = ?`, req.Email)
	if row == nil || row.Str("password") == "" {
		text(w, 400, "User not found")
		return
	}
	if row.Str("password") != req.Password {
		text(w, 400, "Invalid email or password")
		return
	}
	token, _ := a.jwt.UserToken(req.Email, row.Int("id"))
	jsonResp(w, 200, map[string]string{"jwt": token})
}

func (a *App) userBindedAddresses(w http.ResponseWriter, r *http.Request) {
	u := userOf(r)
	q := r.URL.Query()
	limit, offset := q.Get("limit"), q.Get("offset")
	if limit == "" {
		limit = "20"
	}
	if offset == "" {
		offset = "0"
	}
	from := ` FROM address a JOIN users_address ua ON ua.address_id = a.id WHERE ua.user_id = ?`
	a.listQuery(w, r,
		`SELECT a.*, (SELECT COUNT(*) FROM raw_mails WHERE address = a.name) AS mail_count, (SELECT COUNT(*) FROM sendbox WHERE address = a.name) AS send_count`+from,
		`SELECT COUNT(*) AS count`+from, []any{claimInt(u, "user_id")}, limit, offset, "a.id DESC", "password")
}

func (a *App) bindByID(w http.ResponseWriter, r *http.Request, userID, addressID int64, roleName string) {
	ctx := r.Context()
	if addressID == 0 || userID == 0 {
		text(w, 400, "No address or user token")
		return
	}
	if _, found, _ := a.db.ScanInt(ctx, `SELECT id FROM address where id = ?`, addressID); !found {
		text(w, 400, "Address not found")
		return
	}
	if _, found, _ := a.db.ScanInt(ctx, `SELECT id FROM users where id = ?`, userID); !found {
		text(w, 400, "User not found")
		return
	}
	if _, found, _ := a.db.ScanInt(ctx, `SELECT user_id FROM users_address where user_id = ? and address_id = ?`, userID, addressID); found {
		ok(w)
		return
	}
	if reached, msg := a.roles.LimitReached(ctx, userID, roleName); reached {
		text(w, 400, msg)
		return
	}
	if _, err := a.db.Exec(ctx, `INSERT INTO users_address (user_id, address_id) VALUES (?, ?)`, userID, addressID); err != nil {
		if strings.Contains(err.Error(), "UNIQUE") {
			text(w, 400, "Address already binded")
			return
		}
		text(w, 500, "Operation failed")
		return
	}
	ok(w)
}

func (a *App) userBindAddress(w http.ResponseWriter, r *http.Request) {
	u := userOf(r)
	_, addressID := addressOf(r)
	a.bindByID(w, r, claimInt(u, "user_id"), addressID, userRoleOf(r))
}

func (a *App) userBindedAddressJWT(w http.ResponseWriter, r *http.Request) {
	u := userOf(r)
	addressID := atoi(r.PathValue("address_id"))
	name := a.bindedAddress(r.Context(), claimInt(u, "user_id"), addressID)
	if name == "" {
		text(w, 400, "Address not binded")
		return
	}
	token, _ := a.jwt.AddressToken(name, addressID)
	jsonResp(w, 200, map[string]string{"jwt": token})
}

func (a *App) userUnbindAddress(w http.ResponseWriter, r *http.Request) {
	u := userOf(r)
	var req struct {
		AddressID int64 `json:"address_id"`
	}
	readJSON(r, &req)
	userID := claimInt(u, "user_id")
	if req.AddressID == 0 || userID == 0 {
		text(w, 400, "Invalid address or user token")
		return
	}
	if _, err := a.db.Exec(r.Context(), `DELETE FROM users_address where user_id = ? and address_id = ?`, userID, req.AddressID); err != nil {
		text(w, 500, "Operation failed")
		return
	}
	ok(w)
}

func (a *App) userTransferAddress(w http.ResponseWriter, r *http.Request) {
	u := userOf(r)
	ctx := r.Context()
	var req struct {
		AddressID       int64  `json:"address_id"`
		TargetUserEmail string `json:"target_user_email"`
	}
	readJSON(r, &req)
	userID := claimInt(u, "user_id")
	name, found, _ := a.db.ScanString(ctx, `SELECT name FROM address where id = ?`, req.AddressID)
	if !found {
		text(w, 400, "Address not found")
		return
	}
	targetID, found, _ := a.db.ScanInt(ctx, `SELECT id FROM users where user_email = ?`, req.TargetUserEmail)
	if !found {
		text(w, 400, "Target user not found")
		return
	}
	targetRole, _ := a.roles.UserRole(ctx, targetID)
	roleName := ""
	if targetRole != nil {
		roleName = targetRole.Role
	}
	if reached, msg := a.roles.LimitReached(ctx, targetID, roleName); reached {
		text(w, 400, msg)
		return
	}
	if _, found, _ := a.db.ScanInt(ctx, `SELECT user_id FROM users_address where user_id = ? and address_id = ?`, userID, req.AddressID); !found {
		text(w, 400, "Address not binded")
		return
	}
	// Upstream deletes and recreates the address row (new id, mails kept by name).
	a.db.Exec(ctx, `DELETE FROM users_address where user_id = ? and address_id = ?`, userID, req.AddressID)
	a.db.Exec(ctx, `DELETE FROM address WHERE id = ?`, req.AddressID)
	if _, err := a.db.Exec(ctx, `INSERT INTO address(name) VALUES(?)`, name); err != nil {
		text(w, 500, "Failed to create address")
		return
	}
	newID, _, _ := a.db.ScanInt(ctx, `SELECT id FROM address WHERE name = ?`, name)
	if _, err := a.db.Exec(ctx, `INSERT INTO users_address (user_id, address_id) VALUES (?, ?)`, targetID, newID); err != nil {
		text(w, 500, "Operation failed")
		return
	}
	ok(w)
}

func (a *App) oauth2LoginURL(w http.ResponseWriter, r *http.Request) {
	q := r.URL.Query()
	for _, s := range a.oauth2Settings(r.Context()) {
		if s.ClientID == q.Get("clientID") {
			u := fmt.Sprintf("%s?client_id=%s&response_type=code&redirect_uri=%s&scope=%s&state=%s",
				s.AuthorizationURL, s.ClientID, s.RedirectURL, s.Scope, q.Get("state"))
			jsonResp(w, 200, map[string]string{"url": u})
			return
		}
	}
	text(w, 400, "OAuth2 client ID not found")
}

func (a *App) oauth2Callback(w http.ResponseWriter, r *http.Request) {
	var req struct {
		ClientID string `json:"clientID"`
		Code     string `json:"code"`
	}
	readJSON(r, &req)
	if req.ClientID == "" || req.Code == "" {
		text(w, 400, "OAuth2 client ID or code missing")
		return
	}
	var setting *oauth2Setting
	for _, s := range a.oauth2Settings(r.Context()) {
		if s.ClientID == req.ClientID {
			s := s
			setting = &s
		}
	}
	if setting == nil {
		text(w, 400, "OAuth2 client ID not found")
		return
	}
	params := map[string]string{"code": req.Code, "client_id": setting.ClientID, "client_secret": setting.ClientSecret,
		"grant_type": "authorization_code", "redirect_uri": setting.RedirectURL}
	var body io.Reader
	ctype := "application/x-www-form-urlencoded"
	if setting.AccessTokenFormat == "json" {
		b, _ := json.Marshal(params)
		body, ctype = strings.NewReader(string(b)), "application/json"
	} else {
		form := url.Values{}
		for k, v := range params {
			form.Set(k, v)
		}
		body = strings.NewReader(form.Encode())
	}
	client := &http.Client{Timeout: 20 * time.Second}
	treq, _ := http.NewRequest("POST", setting.AccessTokenURL, body)
	treq.Header.Set("Content-Type", ctype)
	treq.Header.Set("Accept", "application/json")
	resp, err := client.Do(treq)
	if err != nil || resp.StatusCode >= 300 {
		text(w, 400, "Failed to get access token")
		return
	}
	var tok struct {
		AccessToken string `json:"access_token"`
		TokenType   string `json:"token_type"`
	}
	json.NewDecoder(resp.Body).Decode(&tok)
	resp.Body.Close()
	if tok.TokenType == "" {
		tok.TokenType = "Bearer"
	}
	ureq, _ := http.NewRequest("GET", setting.UserInfoURL, nil)
	ureq.Header.Set("Authorization", tok.TokenType+" "+tok.AccessToken)
	ureq.Header.Set("Accept", "application/json")
	ureq.Header.Set("User-Agent", "tempmail")
	uresp, err := client.Do(ureq)
	if err != nil || uresp.StatusCode >= 300 {
		text(w, 400, "Failed to get user info")
		return
	}
	var info map[string]any
	json.NewDecoder(uresp.Body).Decode(&info)
	uresp.Body.Close()
	email := lookupEmail(info, setting.UserEmailKey)
	if email == "" {
		text(w, 400, "Failed to get user email")
		return
	}
	if len(email) > 256 {
		email = email[:256]
	}
	email = strings.TrimSpace(email)
	if setting.EnableEmailFormat && setting.UserEmailFormat != "" {
		if re, err := regexp.Compile(setting.UserEmailFormat); err == nil {
			repl := setting.UserEmailReplace
			if repl == "" {
				repl = "$1"
			}
			email = strings.TrimSpace(re.ReplaceAllString(email, jsToGoReplacement(repl)))
		}
	}
	if setting.EnableMailAllowList && !contains(setting.MailAllowList, mailDomain(email)) {
		text(w, 400, "User mail domain must in allow list")
		return
	}
	ctx := r.Context()
	infoJSON, _ := json.Marshal(info)
	if _, err := a.db.Exec(ctx, `INSERT INTO users (user_email, password, user_info) VALUES (?, '', ?) ON CONFLICT(user_email) DO UPDATE SET updated_at = datetime('now')`, email, string(infoJSON)); err != nil {
		text(w, 500, "Failed to register")
		return
	}
	userID, found, _ := a.db.ScanInt(ctx, `SELECT id FROM users where user_email = ?`, email)
	if !found {
		text(w, 400, "User not found")
		return
	}
	if err := a.assignDefaultRole(ctx, userID); err != nil {
		text(w, 500, err.Error())
		return
	}
	token, _ := a.jwt.UserToken(email, userID)
	jsonResp(w, 200, map[string]string{"jwt": token})
}

// lookupEmail supports a plain key or a simple "$.a.b" JSON path.
func lookupEmail(info map[string]any, key string) string {
	if !strings.HasPrefix(key, "$") {
		s, _ := info[key].(string)
		return s
	}
	var cur any = info
	for _, part := range strings.Split(strings.TrimPrefix(strings.TrimPrefix(key, "$"), "."), ".") {
		if part == "" {
			continue
		}
		m, ok := cur.(map[string]any)
		if !ok {
			return ""
		}
		cur = m[part]
	}
	s, _ := cur.(string)
	return s
}

var jsGroupRe = regexp.MustCompile(`\$(\d+)`)

// jsToGoReplacement converts JavaScript "$1" references to Go "${1}".
func jsToGoReplacement(s string) string { return jsGroupRe.ReplaceAllString(s, "${$1}") }
