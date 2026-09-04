package server

import (
	"bytes"
	"context"
	"crypto/hmac"
	"crypto/sha256"
	"encoding/hex"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

func TestFindAllowedDomain(t *testing.T) {
	allow := []string{"a.org", "b.xyz"}
	cases := []struct {
		in   string
		sub  bool
		want string
	}{
		{"a.org", false, "a.org"}, {"A.ORG", false, "a.org"}, {"x.a.org", false, ""}, {"x.a.org", true, "x.a.org"},
		{"c.com", true, ""}, {"-bad.a.org", true, ""}, {"", true, ""},
	}
	for _, c := range cases {
		if got := findAllowedDomain(c.in, allow, c.sub); got != c.want {
			t.Errorf("%q sub=%v: got %q want %q", c.in, c.sub, got, c.want)
		}
	}
}

func TestSHA256Hex(t *testing.T) {
	if sha256Hex("abc") != "ba7816bf8f01cfea414140de5dae2223b00361a396177a9cb410ff61f20015ad" {
		t.Fatal("sha256 mismatch")
	}
}

func TestCustomCleanupExecutesValidatedStatements(t *testing.T) {
	a := newRuntimeTestApp(t, nil)
	ctx := context.Background()
	if _, err := a.db.Exec(ctx, `INSERT INTO user_usage_monthly(user_id, month, addresses_created) VALUES(999999, 'qa', 1)`); err != nil {
		t.Fatal(err)
	}
	if err := a.saveJSONSetting(ctx, "auto_cleanup", cleanupSettings{CustomSqlCleanupList: []customSQLCleanup{{Name: "orphan", SQL: "DELETE FROM user_usage_monthly WHERE user_id = 999999", Enabled: true}}}); err != nil {
		t.Fatal(err)
	}
	if err := a.cleanup(ctx, "custom", 0); err != nil {
		t.Fatal(err)
	}
	if n, _ := a.db.Count(ctx, `SELECT count(*) FROM user_usage_monthly WHERE user_id = 999999`); n != 0 {
		t.Fatalf("orphan rows=%d", n)
	}
}

func TestDefaultCleanupSettingsOnlyEnableEmptyMailboxes(t *testing.T) {
	a := newRuntimeTestApp(t, nil)
	s := a.loadCleanupSettings(context.Background())
	if s.EnableMailsAutoCleanup || s.EnableUnknowMailsAutoCleanup || s.EnableSendBoxAutoCleanup || s.EnableAddressAutoCleanup || s.EnableInactiveAddressAutoCleanup || s.EnableUnboundAddressAutoCleanup || !s.EnableEmptyAddressAutoCleanup {
		t.Fatalf("unexpected default cleanup toggles: %+v", s)
	}
	for name, days := range map[string]int{
		"mails": s.CleanMailsDays, "unknown": s.CleanUnknowMailsDays, "sendbox": s.CleanSendBoxDays,
		"address": s.CleanAddressDays, "inactive": s.CleanInactiveAddressDays, "unbound": s.CleanUnboundAddressDays, "empty": s.CleanEmptyAddressDays,
	} {
		if days != 30 {
			t.Fatalf("%s days=%d, want 30", name, days)
		}
	}
}

func TestCleanupSettingsNormalizeMissingDays(t *testing.T) {
	a := newRuntimeTestApp(t, nil)
	ctx := context.Background()
	if err := a.saveJSONSetting(ctx, "auto_cleanup", cleanupSettings{EnableEmptyAddressAutoCleanup: true}); err != nil {
		t.Fatal(err)
	}
	s := a.loadCleanupSettings(ctx)
	if s.CleanMailsDays != 30 || s.CleanEmptyAddressDays != 30 {
		t.Fatalf("missing days were not normalized: %+v", s)
	}
}

func TestRandomSubdomainLengthUsesRuntimeConfig(t *testing.T) {
	a := newRuntimeTestApp(t, nil)
	rc := a.effective(context.Background())
	rc.RandomSubdomainDomains = []string{"example.com"}
	rc.RandomSubdomainDomainsSet = true
	rc.RandomSubdomainLength = 8
	rc.RandomSubdomainLengthSet = true
	if err := a.saveRuntime(context.Background(), rc); err != nil {
		t.Fatal(err)
	}
	res, err := a.newAddress(context.Background(), newAddressOpts{
		name: "runtime", domain: "example.com", allowDomains: []string{"example.com"}, enableRandomSubdomain: true,
	})
	if err != nil {
		t.Fatal(err)
	}
	parts := strings.Split(strings.SplitN(res.Address, "@", 2)[1], ".")
	if len(parts[0]) != 8 {
		t.Fatalf("random subdomain length=%d, want 8: %s", len(parts[0]), res.Address)
	}
}

func TestDisabledWebhookCanBeSavedWithoutURL(t *testing.T) {
	if err := validateWebhookSettings(webhookSettings{Enabled: false}); err != nil {
		t.Fatalf("disabled webhook validation failed: %v", err)
	}
	if err := validateWebhookSettings(webhookSettings{Enabled: true}); err == nil {
		t.Fatal("enabled webhook without URL passed validation")
	}
}

func TestWebhookSignatureRetryAndDeliveryLog(t *testing.T) {
	a := newRuntimeTestApp(t, nil)
	attempts := 0
	receiver := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		attempts++
		body := new(bytes.Buffer)
		_, _ = body.ReadFrom(r.Body)
		timestamp := r.Header.Get("X-Tempmail-Timestamp")
		mac := hmac.New(sha256.New, []byte("test-secret"))
		_, _ = mac.Write([]byte(timestamp + "." + body.String()))
		want := "sha256=" + hex.EncodeToString(mac.Sum(nil))
		if timestamp == "" || !hmac.Equal([]byte(r.Header.Get("X-Tempmail-Signature")), []byte(want)) {
			t.Errorf("invalid webhook signature")
		}
		if got := r.Header.Get("X-Tempmail-Event-ID"); got != "42" {
			t.Errorf("event id=%q, want 42", got)
		}
		if attempts == 1 {
			http.Error(w, "retry", http.StatusServiceUnavailable)
			return
		}
		w.WriteHeader(http.StatusAccepted)
	}))
	defer receiver.Close()

	a.deliverWebhook(context.Background(), webhookSettings{
		Enabled: true, URL: receiver.URL, Method: http.MethodPost, Body: `{"id":"${id}"}`,
		Secret: "test-secret", TimeoutSeconds: 2, MaxRetries: 1,
	}, map[string]any{"id": 42})
	if attempts != 2 {
		t.Fatalf("attempts=%d, want 2", attempts)
	}
	rows, err := a.db.Query(context.Background(), `SELECT attempt,status_code,error FROM webhook_deliveries ORDER BY id`)
	if err != nil {
		t.Fatal(err)
	}
	if len(rows) != 2 || rows[0].Int("status_code") != http.StatusServiceUnavailable || rows[1].Int("status_code") != http.StatusAccepted {
		t.Fatalf("unexpected delivery rows: %#v", rows)
	}
	if rows[0].Str("error") == "" || rows[1].Str("error") != "" {
		t.Fatalf("unexpected delivery errors: %#v", rows)
	}
}

func TestAutoReplyRuleRequiresExistingMailbox(t *testing.T) {
	a := newRuntimeTestApp(t, nil)
	req := httptest.NewRequest(http.MethodPost, "/admin/auto_reply/rules", strings.NewReader(`{"address":"missing@example.com","subject":"Hello","message":"World","enabled":true}`))
	req.Header.Set("Content-Type", "application/json")
	rec := httptest.NewRecorder()
	a.adminSaveAutoReplyRule(rec, req)
	if rec.Code != http.StatusBadRequest {
		t.Fatalf("missing mailbox status=%d, want 400", rec.Code)
	}

	if _, err := a.db.Exec(context.Background(), `INSERT INTO address(name) VALUES('box@example.com')`); err != nil {
		t.Fatal(err)
	}
	req = httptest.NewRequest(http.MethodPost, "/admin/auto_reply/rules", strings.NewReader(`{"address":"box@example.com","subject":"Hello","message":"World","enabled":true}`))
	req.Header.Set("Content-Type", "application/json")
	rec = httptest.NewRecorder()
	a.adminSaveAutoReplyRule(rec, req)
	if rec.Code != http.StatusOK {
		t.Fatalf("existing mailbox status=%d body=%q", rec.Code, rec.Body.String())
	}
}
