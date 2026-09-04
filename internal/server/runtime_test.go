package server

import (
	"bufio"
	"context"
	"encoding/json"
	"fmt"
	"io/fs"
	"net/http"
	"net/http/httptest"
	"net/url"
	"path/filepath"
	"strings"
	"testing"
	"testing/fstest"
	"time"

	"tempmail/internal/auth"
	"tempmail/internal/config"
	"tempmail/internal/db"
	"tempmail/internal/mailer"
	"tempmail/internal/roles"
)

func newRuntimeTestApp(t *testing.T, static fs.FS) *App {
	t.Helper()
	d, err := db.Open(filepath.Join(t.TempDir(), "test.db"))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { d.Close() })
	cfg := &config.Config{
		HTTPAddr: ":0", SMTPAddr: ":0", JWTSecret: "test-secret", DBPath: filepath.Join(t.TempDir(), "unused.db"),
		Title: "Env title", DefaultLang: "zh", Domains: []string{"example.com"}, DefaultDomains: []string{"example.com"},
		MinAddressLen: 1, MaxAddressLen: 30, EnableUserCreateEmail: true, EnableUserDeleteEmail: true,
		EnableMailReadStatus: true, EnableAddressPassword: true, EnableWebhook: true, EnableAutoReply: true,
	}
	return New(context.Background(), cfg, d, auth.New(cfg.JWTSecret), mailer.New(cfg), roles.New(cfg, d), static)
}

func TestEffectiveConfigUsesEnvUntilOverrideExists(t *testing.T) {
	a := newRuntimeTestApp(t, nil)
	got := a.effective(context.Background())
	if !got.EnableUserCreateEmail || !got.EnableUserDeleteEmail || !got.EnableMailReadStatus || !got.EnableWebhook {
		t.Fatalf("env booleans were lost: %+v", got)
	}

	got.EnableUserDeleteEmail = false
	got.EnableMailReadStatus = false
	if err := a.saveRuntime(context.Background(), got); err != nil {
		t.Fatal(err)
	}
	overridden := a.effective(context.Background())
	if overridden.EnableUserDeleteEmail || overridden.EnableMailReadStatus {
		t.Fatalf("saved booleans were not applied: %+v", overridden)
	}
}

func TestRuntimePrefixCanBeCleared(t *testing.T) {
	a := newRuntimeTestApp(t, nil)
	a.cfg.Prefix = "envprefix"
	rc := a.effective(context.Background())
	rc.Prefix = ""
	rc.PrefixSet = true
	if err := a.saveRuntime(context.Background(), rc); err != nil {
		t.Fatal(err)
	}
	if got := a.effective(context.Background()).Prefix; got != "" {
		t.Fatalf("prefix=%q, want empty", got)
	}
}

func TestNewMailboxUsesJWTWithoutPassword(t *testing.T) {
	a := newRuntimeTestApp(t, nil)
	res, err := a.newAddress(context.Background(), newAddressOpts{name: "credential", domain: "example.com", allowDomains: []string{"example.com"}})
	if err != nil {
		t.Fatal(err)
	}
	if res.JWT == "" || res.Password != nil {
		t.Fatalf("jwt=%q password=%v", res.JWT, res.Password)
	}
	password, _, err := a.db.ScanString(context.Background(), `SELECT password FROM address WHERE id=?`, res.AddressID)
	if err != nil || password != "" {
		t.Fatalf("stored password=%q err=%v", password, err)
	}
}

func TestCleanupTypesRunsEverySelectedType(t *testing.T) {
	a := newRuntimeTestApp(t, nil)
	ctx := context.Background()
	if _, err := a.db.Exec(ctx, `INSERT INTO raw_mails(source,address,raw,created_at) VALUES('sender','box@example.com','raw',datetime('now','-40 day'))`); err != nil {
		t.Fatal(err)
	}
	if _, err := a.db.Exec(ctx, `INSERT INTO sendbox(address,raw,created_at) VALUES('box@example.com','raw',datetime('now','-40 day'))`); err != nil {
		t.Fatal(err)
	}
	if err := a.cleanupTypes(ctx, []string{"mails", "sendbox", "mails"}, 30); err != nil {
		t.Fatal(err)
	}
	if mails, _ := a.db.Count(ctx, `SELECT count(*) FROM raw_mails`); mails != 0 {
		t.Fatalf("mail count=%d, want 0", mails)
	}
	if sent, _ := a.db.Count(ctx, `SELECT count(*) FROM sendbox`); sent != 0 {
		t.Fatalf("sendbox count=%d, want 0", sent)
	}
}

func TestEventsRouteRequiresMatchingAddressToken(t *testing.T) {
	static := fstest.MapFS{"index.html": &fstest.MapFile{Data: []byte("spa")}}
	a := newRuntimeTestApp(t, static)
	token, _ := a.jwt.AddressToken("box@example.com", 1)

	bad := httptest.NewRecorder()
	a.Handler().ServeHTTP(bad, httptest.NewRequest(http.MethodGet, "/events?address=box@example.com&token=bad", nil))
	if bad.Code != http.StatusUnauthorized {
		t.Fatalf("bad token status = %d, want 401", bad.Code)
	}

	srv := httptest.NewServer(a.Handler())
	t.Cleanup(srv.Close)
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()
	req, _ := http.NewRequestWithContext(ctx, http.MethodGet, srv.URL+"/events?address=box%40example.com&token="+url.QueryEscape(token), nil)
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatal(err)
	}
	defer resp.Body.Close()
	if got := resp.Header.Get("Content-Type"); !strings.HasPrefix(got, "text/event-stream") {
		t.Fatalf("content type = %q, want event stream", got)
	}
	line, err := bufio.NewReader(resp.Body).ReadString('\n')
	if err != nil || line != ": connected\n" {
		t.Fatalf("first event line = %q, err=%v", line, err)
	}
}

func TestAPIKeyCanAccessNamedMailbox(t *testing.T) {
	a := newRuntimeTestApp(t, nil)
	rc := a.effective(context.Background())
	rc.APIKey = "automation-secret"
	if err := a.saveRuntime(context.Background(), rc); err != nil {
		t.Fatal(err)
	}
	if _, err := a.db.Exec(context.Background(), `INSERT INTO address(name) VALUES('box@example.com')`); err != nil {
		t.Fatal(err)
	}

	req := httptest.NewRequest(http.MethodGet, "/api/parsed_mails?limit=20&offset=0", nil)
	req.Header.Set("x-api-key", "automation-secret")
	req.Header.Set("x-address", "box@example.com")
	w := httptest.NewRecorder()
	a.Handler().ServeHTTP(w, req)
	if w.Code != http.StatusOK {
		t.Fatalf("valid API key status = %d body=%s", w.Code, w.Body.String())
	}

	bad := httptest.NewRequest(http.MethodGet, "/api/parsed_mails?limit=20&offset=0", nil)
	bad.Header.Set("x-api-key", "wrong")
	bad.Header.Set("x-address", "box@example.com")
	w = httptest.NewRecorder()
	a.Handler().ServeHTTP(w, bad)
	if w.Code != http.StatusUnauthorized {
		t.Fatalf("bad API key status = %d, want 401", w.Code)
	}

	bearer := httptest.NewRequest(http.MethodGet, "/api/parsed_mails?limit=20&offset=0", nil)
	bearer.Header.Set("Authorization", "Bearer automation-secret")
	bearer.Header.Set("x-address", "box@example.com")
	w = httptest.NewRecorder()
	a.Handler().ServeHTTP(w, bearer)
	if w.Code != http.StatusOK {
		t.Fatalf("API key bearer status = %d body=%s, want 200", w.Code, w.Body.String())
	}
}

func TestLoginIsRateLimited(t *testing.T) {
	a := newRuntimeTestApp(t, nil)
	if !a.rateLimitedPath("/user_api/login") {
		t.Fatal("user login must be covered by the request limiter")
	}
}

func TestUserLimitsApplyWithoutRole(t *testing.T) {
	a := newRuntimeTestApp(t, nil)
	_, err := a.db.Exec(context.Background(), `INSERT INTO users (user_email, username, password) VALUES ('limited@example.com', 'limited', 'pw')`)
	if err != nil {
		t.Fatal(err)
	}
	uid, _, err := a.db.ScanInt(context.Background(), `SELECT id FROM users WHERE username = 'limited'`)
	if err != nil {
		t.Fatal(err)
	}
	if err := a.roles.SaveUserLimits(context.Background(), uid, roles.UserLimits{MaxAddressCount: 1, MaxMailCount: -1, MonthlyAddressQuota: -1, MonthlyReceiveQuota: -1}); err != nil {
		t.Fatal(err)
	}
	token, _ := a.jwt.UserToken("limited@example.com", uid)
	create := func() *httptest.ResponseRecorder {
		req := httptest.NewRequest(http.MethodPost, "/api/new_address", strings.NewReader(`{"domain":"example.com"}`))
		req.Header.Set("x-user-token", token)
		w := httptest.NewRecorder()
		a.Handler().ServeHTTP(w, req)
		return w
	}
	if w := create(); w.Code != http.StatusOK {
		t.Fatalf("first address status = %d body=%s", w.Code, w.Body.String())
	}
	if w := create(); w.Code != http.StatusBadRequest {
		t.Fatalf("second address status = %d body=%s, want 400", w.Code, w.Body.String())
	}
}

func TestParsedMailCountIsStableOnLaterPages(t *testing.T) {
	a := newRuntimeTestApp(t, nil)
	ctx := context.Background()
	rc := a.effective(ctx)
	rc.APIKey = "count-key"
	if err := a.saveRuntime(ctx, rc); err != nil {
		t.Fatal(err)
	}
	if _, err := a.db.Exec(ctx, `INSERT INTO address(name) VALUES('count@example.com')`); err != nil {
		t.Fatal(err)
	}
	for i := 0; i < 2; i++ {
		if _, err := a.db.Exec(ctx, `INSERT INTO raw_mails(source, address, raw, message_id) VALUES(?, ?, ?, ?)`, "sender@example.com", "count@example.com", "Subject: test\n\nbody", fmt.Sprintf("count-%d", i)); err != nil {
			t.Fatal(err)
		}
	}
	req := httptest.NewRequest(http.MethodGet, "/api/parsed_mails?limit=1&offset=1", nil)
	req.Header.Set("x-api-key", "count-key")
	req.Header.Set("x-address", "count@example.com")
	w := httptest.NewRecorder()
	a.Handler().ServeHTTP(w, req)
	if w.Code != http.StatusOK {
		t.Fatalf("status=%d body=%s", w.Code, w.Body.String())
	}
	var got struct {
		Count int `json:"count"`
	}
	if err := json.Unmarshal(w.Body.Bytes(), &got); err != nil {
		t.Fatal(err)
	}
	if got.Count != 2 {
		t.Fatalf("count=%d, want 2", got.Count)
	}
}

func TestUserDeleteAddressRefundsMonthlyQuotaAndEnforcesOwnership(t *testing.T) {
	a := newRuntimeTestApp(t, nil)
	ctx := context.Background()
	if _, err := a.db.Exec(ctx, `INSERT INTO users(user_email, username, password) VALUES('owner@example.com','owner','pw'),('other@example.com','other','pw')`); err != nil {
		t.Fatal(err)
	}
	owner, _, _ := a.db.ScanInt(ctx, `SELECT id FROM users WHERE username='owner'`)
	other, _, _ := a.db.ScanInt(ctx, `SELECT id FROM users WHERE username='other'`)
	if _, err := a.db.Exec(ctx, `INSERT INTO address(name) VALUES('owned@example.com')`); err != nil {
		t.Fatal(err)
	}
	aid, _, _ := a.db.ScanInt(ctx, `SELECT id FROM address WHERE name='owned@example.com'`)
	if _, err := a.db.Exec(ctx, `INSERT INTO users_address(user_id,address_id) VALUES(?,?)`, owner, aid); err != nil {
		t.Fatal(err)
	}
	month := time.Now().UTC().Format("2006-01")
	if _, err := a.db.Exec(ctx, `INSERT INTO user_usage_monthly(user_id,month,addresses_created) VALUES(?,?,1)`, owner, month); err != nil {
		t.Fatal(err)
	}
	otherToken, _ := a.jwt.UserToken("other@example.com", other)
	bad := httptest.NewRequest(http.MethodDelete, "/user_api/address/"+fmt.Sprint(aid), nil)
	bad.Header.Set("x-user-token", otherToken)
	w := httptest.NewRecorder()
	a.Handler().ServeHTTP(w, bad)
	if w.Code != http.StatusBadRequest && w.Code != http.StatusNotFound {
		t.Fatalf("other user delete status=%d", w.Code)
	}
	ownerToken, _ := a.jwt.UserToken("owner@example.com", owner)
	okReq := httptest.NewRequest(http.MethodDelete, "/user_api/address/"+fmt.Sprint(aid), nil)
	okReq.Header.Set("x-user-token", ownerToken)
	w = httptest.NewRecorder()
	a.Handler().ServeHTTP(w, okReq)
	if w.Code != http.StatusOK {
		t.Fatalf("owner delete status=%d body=%s", w.Code, w.Body.String())
	}
	var used int64
	_ = a.db.QueryRowContext(ctx, `SELECT addresses_created FROM user_usage_monthly WHERE user_id=? AND month=?`, owner, month).Scan(&used)
	if used != 0 {
		t.Fatalf("addresses_created=%d, want 0", used)
	}
}
