package server

import (
	"context"
	"encoding/json"
	"io"
	"io/fs"
	"log"
	"net"
	"net/http"
	"path"
	"strconv"
	"strings"

	"tempmail/internal/auth"
	"tempmail/internal/config"
	"tempmail/internal/db"
	"tempmail/internal/mailer"
	"tempmail/internal/passkey"
	"tempmail/internal/roles"
	"tempmail/internal/s3store"
	"tempmail/internal/telegram"

	"github.com/golang-jwt/jwt/v5"
)

type App struct {
	cfg    *config.Config
	db     *db.DB
	jwt    *auth.Signer
	mailer *mailer.Mailer
	roles  *roles.Store
	static http.Handler
	mux    *http.ServeMux
	ingest IngestFunc
	tg     *telegram.Bot
	pass   *passkey.Store
	s3     *s3store.Store
}

// IngestFunc stores a raw inbound message; wired to the SMTP pipeline.
type IngestFunc func(ctx context.Context, from, to string, raw []byte) (int64, error)

func (a *App) SetIngest(f IngestFunc) { a.ingest = f }

type ctxKey int

const (
	ctxAddress ctxKey = iota
	ctxUser
	ctxUserRole
)

var apiPrefixes = []string{"/api/", "/open_api/", "/user_api/", "/admin/", "/telegram/", "/external/"}

func New(ctx context.Context, cfg *config.Config, d *db.DB, signer *auth.Signer, m *mailer.Mailer, rs *roles.Store, webFS fs.FS) *App {
	a := &App{cfg: cfg, db: d, jwt: signer, mailer: m, roles: rs, mux: http.NewServeMux()}
	if webFS != nil {
		a.static = spaHandler(webFS)
	}
	if cfg.TelegramBotToken != "" {
		a.tg = telegram.New(cfg, d, signer, addressAdapter{a})
	}
	if s3s, err := s3store.New(ctx, cfg); err == nil {
		a.s3 = s3s
	}
	if pk, err := passkey.New(a.rpID(), []string{a.origin()}, a.cfg.Title); err == nil {
		a.pass = pk
	}
	a.routes()
	return a
}

func (a *App) rpID() string {
	if h := hostOnly(a.cfg.FrontendURL); h != "" {
		return strings.TrimPrefix(h, "www.")
	}
	if len(a.cfg.Domains) > 0 {
		return a.cfg.Domains[0]
	}
	return "localhost"
}

func (a *App) origin() string {
	if a.cfg.FrontendURL != "" {
		return strings.TrimRight(a.cfg.FrontendURL, "/")
	}
	return "http://localhost:8080"
}

func hostOnly(u string) string {
	u = strings.TrimPrefix(strings.TrimPrefix(u, "https://"), "http://")
	if i := strings.IndexAny(u, "/:"); i >= 0 {
		u = u[:i]
	}
	return u
}

// Telegram returns the bot (nil when TELEGRAM_BOT_TOKEN is unset).
func (a *App) Telegram() *telegram.Bot { return a.tg }

type addressAdapter struct{ a *App }

func (x addressAdapter) NewAddress(ctx context.Context, name, domain string, randomSub bool, sourceMeta string) (string, string, int64, *string, error) {
	res, err := x.a.newAddress(ctx, newAddressOpts{name: name, domain: domain, enablePrefix: true, enableRandomSubdomain: randomSub,
		checkLengthByConfig: true, allowDomains: x.a.cfg.Domains, enableCheckNameRegex: true, sourceMeta: sourceMeta})
	if err != nil {
		return "", "", 0, nil, err
	}
	return res.Address, res.JWT, res.AddressID, res.Password, nil
}

func (x addressAdapter) DeleteAddress(ctx context.Context, id int64) error {
	return x.a.deleteAddressesWhere(ctx, `id = ?`, id)
}

func (x addressAdapter) RandomName() string { return x.a.generateRandomName() }

func (a *App) Handler() http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Access-Control-Allow-Origin", "*")
		w.Header().Set("Access-Control-Allow-Methods", "GET,POST,PUT,DELETE,OPTIONS")
		w.Header().Set("Access-Control-Allow-Headers", "*")
		if r.Method == http.MethodOptions {
			w.WriteHeader(http.StatusNoContent)
			return
		}
		p := r.URL.Path
		if a.cfg.OriginToken != "" && p != "/health" && p != "/health_check" && r.Header.Get("x-origin-token") != a.cfg.OriginToken {
			text(w, 403, "Direct origin access is not allowed")
			return
		}
		isAPI := false
		for _, pre := range apiPrefixes {
			if strings.HasPrefix(p, pre) {
				isAPI = true
				break
			}
		}
		if !isAPI && a.static != nil && p != "/health_check" && p != "/health" {
			a.static.ServeHTTP(w, r)
			return
		}
		a.auth(w, r)
	})
}

func (a *App) auth(w http.ResponseWriter, r *http.Request) {
	p := r.URL.Path
	if code, block := a.ipBlacklistApply(r); block {
		w.Header().Set("Retry-After", "60")
		text(w, code, "Access blocked")
		return
	}
	if a.rateLimitedPath(p) {
		if !a.rateLimit(p, clientIP(r, a.cfg.TrustedProxies)) {
			w.Header().Set("Retry-After", "60")
			text(w, 429, "Rate limit exceeded")
			return
		}
	}
	if len(a.cfg.Passwords) > 0 && !strings.HasPrefix(p, "/open_api") && !strings.HasPrefix(p, "/telegram/") {
		if !contains(a.cfg.Passwords, r.Header.Get("x-custom-auth")) {
			text(w, 401, "Need Custom Auth Password")
			return
		}
	}
	ctx := r.Context()
	switch {
	case strings.HasPrefix(p, "/api/"):
		if strings.HasPrefix(p, "/api/new_address") {
			ctx = a.withUserPayload(ctx, r)
			break
		}
		if strings.HasPrefix(p, "/api/settings") || strings.HasPrefix(p, "/api/send_mail") {
			ctx = a.withUserRolePayload(ctx, r, 0)
		}
		if strings.HasPrefix(p, "/api/address_login") {
			break
		}
		claims, ok := a.bearer(r)
		if !ok {
			text(w, 401, "Invalid address credential")
			return
		}
		ctx = context.WithValue(ctx, ctxAddress, claims)
	case strings.HasPrefix(p, "/user_api/"):
		if strings.HasPrefix(p, "/user_api/open_settings") || strings.HasPrefix(p, "/user_api/register") ||
			strings.HasPrefix(p, "/user_api/login") || strings.HasPrefix(p, "/user_api/verify_code") ||
			strings.HasPrefix(p, "/user_api/passkey/authenticate_") || strings.HasPrefix(p, "/user_api/oauth2") {
			break
		}
		claims, err := a.jwt.Verify(r.Header.Get("x-user-token"))
		if err != nil || claims["exp"] == nil {
			text(w, 401, "User token expired")
			return
		}
		ctx = context.WithValue(ctx, ctxUser, claims)
		if strings.HasPrefix(p, "/user_api/bind_address") || strings.HasPrefix(p, "/user_api/address/") {
			ctx = a.withUserRolePayload(ctx, r, auth.ClaimInt(claims, "user_id"))
		}
		if strings.HasPrefix(p, "/user_api/bind_address") && r.Method == http.MethodPost {
			ac, ok := a.bearer(r)
			if !ok {
				text(w, 401, "Invalid address credential")
				return
			}
			ctx = context.WithValue(ctx, ctxAddress, ac)
		}
	case strings.HasPrefix(p, "/admin/"):
		if !a.isAdmin(r) {
			text(w, 401, "Need admin password")
			return
		}
	}
	a.mux.ServeHTTP(w, r.WithContext(ctx))
}

func (a *App) isAdmin(r *http.Request) bool {
	if len(a.cfg.AdminPasswords) > 0 && contains(a.cfg.AdminPasswords, r.Header.Get("x-admin-auth")) {
		return true
	}
	if a.cfg.AdminUserRole != "" {
		if t := r.Header.Get("x-user-access-token"); t != "" {
			if c, err := a.jwt.Verify(t); err == nil && c["exp"] != nil && auth.ClaimStr(c, "user_role") == a.cfg.AdminUserRole {
				return true
			}
		}
	}
	return a.cfg.DisableAdminPasswordCheck
}

func (a *App) bearer(r *http.Request) (jwt.MapClaims, bool) {
	h := r.Header.Get("Authorization")
	if !strings.HasPrefix(h, "Bearer ") {
		return nil, false
	}
	c, err := a.jwt.Verify(strings.TrimSpace(h[7:]))
	if err != nil {
		return nil, false
	}
	return c, true
}

func (a *App) withUserPayload(ctx context.Context, r *http.Request) context.Context {
	t := r.Header.Get("x-user-token")
	if t == "" {
		return ctx
	}
	if c, err := a.jwt.Verify(t); err == nil && c["exp"] != nil {
		return context.WithValue(ctx, ctxUser, c)
	}
	return ctx
}

func (a *App) withUserRolePayload(ctx context.Context, r *http.Request, userID int64) context.Context {
	t := r.Header.Get("x-user-access-token")
	if t == "" {
		return ctx
	}
	c, err := a.jwt.Verify(t)
	if err != nil || c["exp"] == nil {
		return ctx
	}
	role, ok := c["user_role"].(string)
	if !ok || (userID != 0 && auth.ClaimInt(c, "user_id") != userID) {
		return ctx
	}
	return context.WithValue(ctx, ctxUserRole, role)
}

func addressOf(r *http.Request) (string, int64) {
	c, _ := r.Context().Value(ctxAddress).(jwt.MapClaims)
	if c == nil {
		return "", 0
	}
	return auth.ClaimStr(c, "address"), auth.ClaimInt(c, "address_id")
}

func userOf(r *http.Request) jwt.MapClaims {
	c, _ := r.Context().Value(ctxUser).(jwt.MapClaims)
	return c
}

func userRoleOf(r *http.Request) string {
	s, _ := r.Context().Value(ctxUserRole).(string)
	return s
}

func contains(list []string, v string) bool {
	if v == "" {
		return false
	}
	for _, s := range list {
		if s == v {
			return true
		}
	}
	return false
}

func text(w http.ResponseWriter, code int, msg string) {
	w.Header().Set("Content-Type", "text/plain; charset=utf-8")
	w.WriteHeader(code)
	io.WriteString(w, msg)
}

func jsonResp(w http.ResponseWriter, code int, v any) {
	w.Header().Set("Content-Type", "application/json; charset=utf-8")
	w.WriteHeader(code)
	if err := json.NewEncoder(w).Encode(v); err != nil {
		log.Printf("json encode: %v", err)
	}
}

func ok(w http.ResponseWriter) { jsonResp(w, 200, map[string]any{"success": true}) }

func readJSON(r *http.Request, v any) error {
	body, err := io.ReadAll(io.LimitReader(r.Body, 32<<20))
	if err != nil {
		return err
	}
	if len(strings.TrimSpace(string(body))) == 0 {
		return nil
	}
	return json.Unmarshal(body, v)
}

func fail(w http.ResponseWriter, err error) {
	log.Printf("error: %v", err)
	text(w, 500, "Error "+err.Error())
}

func clientIP(r *http.Request, trusted []string) string {
	host, _, _ := net.SplitHostPort(r.RemoteAddr)
	if len(trusted) == 0 {
		return host
	}
	for _, t := range trusted {
		if t == "*" || t == host {
			if ip := r.Header.Get("CF-Connecting-IP"); ip != "" {
				return ip
			}
			if xf := r.Header.Get("X-Forwarded-For"); xf != "" {
				return strings.TrimSpace(strings.Split(xf, ",")[0])
			}
			if ip := r.Header.Get("X-Real-IP"); ip != "" {
				return ip
			}
		}
	}
	return host
}

func atoi(s string) int64 {
	n, _ := strconv.ParseInt(strings.TrimSpace(s), 10, 64)
	return n
}

// spaHandler serves the embedded frontend, falling back to index.html for
// client-side routes (mirrors upstream's ASSETS behaviour).
func spaHandler(root fs.FS) http.Handler {
	files := http.FS(root)
	fileServer := http.FileServer(files)
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		p := path.Clean("/" + r.URL.Path)
		if p != "/" && !strings.Contains(path.Base(p), ".") {
			r.URL.Path = "/"
		} else if _, err := fs.Stat(root, strings.TrimPrefix(p, "/")); err != nil && p != "/" {
			r.URL.Path = "/"
		}
		if r.URL.Path == "/" {
			w.Header().Set("Cache-Control", "no-cache")
		}
		fileServer.ServeHTTP(w, r)
	})
}
