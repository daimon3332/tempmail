package server

import (
	"context"
	"io"
	"net/http"

	"tempmail/internal/telegram"
)

func (a *App) telegramRoutes() {
	m := a.mux
	m.HandleFunc("POST /telegram/webhook", a.tgWebhook)
	m.HandleFunc("POST /admin/telegram/init", a.tgInit)
	m.HandleFunc("GET /admin/telegram/status", a.tgStatus)
	m.HandleFunc("GET /admin/telegram/settings", a.tgGetSettings)
	m.HandleFunc("POST /admin/telegram/settings", a.tgSaveSettings)
	m.HandleFunc("POST /telegram/get_bind_address", a.tgGetBindAddress)
	m.HandleFunc("POST /telegram/new_address", a.tgNewAddress)
	m.HandleFunc("POST /telegram/bind_address", a.tgBind)
	m.HandleFunc("POST /telegram/unbind_address", a.tgUnbind)
	m.HandleFunc("POST /telegram/get_mail", a.tgGetMail)
}

func (a *App) requireTelegram(w http.ResponseWriter) bool {
	if a.tg == nil || a.cfg.TelegramBotToken == "" {
		text(w, 400, "Telegram bot is not configured")
		return false
	}
	return true
}

func (a *App) tgWebhook(w http.ResponseWriter, r *http.Request) {
	if !a.requireTelegram(w) {
		return
	}
	body, err := io.ReadAll(io.LimitReader(r.Body, 1<<20))
	if err != nil {
		text(w, 400, "Invalid body")
		return
	}
	a.tg.HandleUpdate(r.Context(), body)
	w.WriteHeader(http.StatusOK)
}

func (a *App) tgInit(w http.ResponseWriter, r *http.Request) {
	if !a.requireTelegram(w) {
		return
	}
	host := r.Header.Get("Host")
	if host == "" {
		text(w, 400, "Host header is required")
		return
	}
	proto := r.Header.Get("X-Forwarded-Proto")
	if proto == "" {
		proto = schemeOf(a.cfg.FrontendURL)
	}
	if err := a.tg.SetWebhook(proto + "://" + host + "/telegram/webhook"); err != nil {
		text(w, 500, "Failed to set webhook: "+err.Error())
		return
	}
	a.tg.SetCommands()
	jsonResp(w, 200, map[string]string{"message": "webhook set successfully"})
}

func (a *App) tgStatus(w http.ResponseWriter, r *http.Request) {
	if !a.requireTelegram(w) {
		return
	}
	info, err := a.tg.Status()
	if err != nil {
		text(w, 500, err.Error())
		return
	}
	jsonResp(w, 200, info)
}

func (a *App) tgGetSettings(w http.ResponseWriter, r *http.Request) {
	if !a.requireTelegram(w) {
		return
	}
	jsonResp(w, 200, a.tg.Settings(r.Context()))
}

func (a *App) tgSaveSettings(w http.ResponseWriter, r *http.Request) {
	if !a.requireTelegram(w) {
		return
	}
	var s telegram.Settings
	readJSON(r, &s)
	if s.AllowList == nil {
		s.AllowList = []string{}
	}
	if s.GlobalMailPushList == nil {
		s.GlobalMailPushList = []string{}
	}
	if err := a.tg.SaveSettings(r.Context(), s); err != nil {
		fail(w, err)
		return
	}
	ok(w)
}

// ---- miniapp (initData auth) ----

func (a *App) tgGetBindAddress(w http.ResponseWriter, r *http.Request) {
	if !a.requireTelegram(w) {
		return
	}
	var req struct {
		InitData string `json:"initData"`
	}
	readJSON(r, &req)
	uid, err := a.tg.CheckInitData(req.InitData)
	if err != nil {
		text(w, 400, err.Error())
		return
	}
	jsonResp(w, 200, a.tg.UserAddresses(r.Context(), uid))
}

func (a *App) tgNewAddress(w http.ResponseWriter, r *http.Request) {
	if !a.requireTelegram(w) {
		return
	}
	var req struct {
		InitData              string `json:"initData"`
		Address               string `json:"address"`
		EnableRandomSubdomain bool   `json:"enableRandomSubdomain"`
		CfToken               string `json:"cf_token"`
	}
	readJSON(r, &req)
	if err := a.checkTurnstile(r.Context(), req.CfToken); err != nil {
		text(w, 400, "Captcha failed")
		return
	}
	uid, err := a.tg.CheckInitData(req.InitData)
	if err != nil {
		text(w, 400, err.Error())
		return
	}
	addr, jwt, pw, err := a.tg.NewAddress(r.Context(), uid, req.Address, req.EnableRandomSubdomain)
	if err != nil {
		text(w, 400, err.Error())
		return
	}
	jsonResp(w, 200, map[string]any{"address": addr, "jwt": jwt, "password": pw})
}

func (a *App) tgBind(w http.ResponseWriter, r *http.Request) {
	if !a.requireTelegram(w) {
		return
	}
	var req struct {
		InitData string `json:"initData"`
		JWT      string `json:"jwt"`
	}
	readJSON(r, &req)
	uid, err := a.tg.CheckInitData(req.InitData)
	if err != nil {
		text(w, 400, err.Error())
		return
	}
	if _, err := a.tg.Bind(r.Context(), uid, req.JWT); err != nil {
		text(w, 400, err.Error())
		return
	}
	ok(w)
}

func (a *App) tgUnbind(w http.ResponseWriter, r *http.Request) {
	if !a.requireTelegram(w) {
		return
	}
	var req struct {
		InitData string `json:"initData"`
		Address  string `json:"address"`
	}
	readJSON(r, &req)
	uid, err := a.tg.CheckInitData(req.InitData)
	if err != nil {
		text(w, 400, err.Error())
		return
	}
	a.tg.Unbind(r.Context(), uid, req.Address)
	ok(w)
}

func (a *App) tgGetMail(w http.ResponseWriter, r *http.Request) {
	if !a.requireTelegram(w) {
		return
	}
	var req struct {
		InitData string `json:"initData"`
		MailID   int64  `json:"mailId"`
	}
	readJSON(r, &req)
	ctx := r.Context()
	if contains(a.cfg.AdminPasswords, r.Header.Get("x-admin-auth")) {
		a.tgGetMailByID(w, ctx, req.MailID)
		return
	}
	uid, err := a.tg.CheckInitData(req.InitData)
	if err != nil {
		text(w, 400, err.Error())
		return
	}
	row, _ := a.db.QueryOne(ctx, `SELECT * FROM raw_mails WHERE id = ?`, req.MailID)
	if row == nil {
		jsonResp(w, 200, nil)
		return
	}
	if !a.tg.OwnsAddress(ctx, uid, row.Str("address")) && !a.tg.IsSuperUser(ctx, uid) {
		text(w, 403, "No permission to view this mail")
		return
	}
	resolveRaw(row)
	jsonResp(w, 200, row)
}

func (a *App) tgGetMailByID(w http.ResponseWriter, ctx context.Context, mailID int64) {
	row, _ := a.db.QueryOne(ctx, `SELECT * FROM raw_mails WHERE id = ?`, mailID)
	if row == nil {
		jsonResp(w, 200, nil)
		return
	}
	resolveRaw(row)
	jsonResp(w, 200, row)
}

func schemeOf(u string) string {
	if len(u) >= 8 && u[:8] == "https://" {
		return "https"
	}
	if len(u) >= 7 && u[:7] == "http://" {
		return "http"
	}
	return "https"
}
