package server

import (
	"context"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"tempmail/internal/mailparse"
)

// cfAuth is the Authentication-Results Cloudflare added to a real forwarded mail.
const cfAuth = "mx.cloudflare.net; dkim=pass header.d=icloud.com header.s=1a1hai header.b=XJ2IBWK+; dkim=fail (verification failed) header.d=gmail.com header.s=20260707 header.b=uS3880q5; dmarc=pass header.from=icloud.com policy.dmarc=quarantine"

func hmeMail(header string) *mailparse.Mail {
	return &mailparse.Mail{Headers: []mailparse.Header{{Key: "Authentication-Results", Value: cfAuth}, {Key: "X-Icloud-Hme", Value: header}}}
}

func TestRouteHMEOnlyTrustsLinkedAliasesAtForwardTarget(t *testing.T) {
	a := newRuntimeTestApp(t, nil)
	ctx := context.Background()
	for _, q := range []string{
		`INSERT INTO hme_accounts(id, name, forward_to) VALUES(1, 'a', 'relay@example.com')`,
		`INSERT INTO hme_aliases(email, account_id, anonymous_id) VALUES('linked@icloud.com', 1, 'x1'), ('unlinked@icloud.com', 1, 'x2')`,
		`INSERT INTO address(name) VALUES('linked@icloud.com'), ('relay@example.com')`,
	} {
		if _, err := a.db.Exec(ctx, q); err != nil {
			t.Fatal(err)
		}
	}
	alias, sender, ok := a.RouteHME(ctx, "Relay@example.com", hmeMail("p=Hide My Email <Linked@icloud.com>; f=relay@example.com; r=to; s=shop@vendor.com"))
	if !ok || alias != "linked@icloud.com" || sender != "shop@vendor.com" {
		t.Fatalf("linked alias not routed: %q %q %v", alias, sender, ok)
	}
	for name, c := range map[string]struct {
		to     string
		header string
	}{
		"unlinked alias":     {"relay@example.com", "p=unlinked@icloud.com; f=relay@example.com"},
		"forged forward":     {"relay@example.com", "p=linked@icloud.com; f=other@example.com"},
		"not forward target": {"other@example.com", "p=linked@icloud.com; f=other@example.com"},
		"unknown alias":      {"relay@example.com", "p=nobody@icloud.com; f=relay@example.com"},
	} {
		if _, _, ok := a.RouteHME(ctx, c.to, hmeMail(c.header)); ok {
			t.Errorf("%s: routed but must stay in forward mailbox", name)
		}
	}
	if _, _, ok := a.RouteHME(ctx, "relay@example.com", &mailparse.Mail{}); ok {
		t.Error("mail without X-Icloud-Hme was routed")
	}
	genuine := "p=linked@icloud.com; f=relay@example.com; s=x@y.com"
	for name, auth := range map[string][]string{
		"no dkim result":       nil,
		"apple dkim failed":    {"mx.cloudflare.net; dkim=fail header.d=icloud.com; dmarc=fail"},
		"other domain signed":  {"mx.cloudflare.net; dkim=pass header.d=evil.com"},
		"forged below real AR": {"mx.cloudflare.net; dkim=pass header.d=evil.com", cfAuth},
	} {
		m := &mailparse.Mail{}
		for _, v := range auth {
			m.Headers = append(m.Headers, mailparse.Header{Key: "Authentication-Results", Value: v})
		}
		m.Headers = append(m.Headers, mailparse.Header{Key: "X-Icloud-Hme", Value: genuine})
		if _, _, ok := a.RouteHME(ctx, "relay@example.com", m); ok {
			t.Errorf("%s: routed without Apple DKIM verification", name)
		}
	}
	if last, _, _ := a.db.ScanString(ctx, `SELECT last_mail_at FROM hme_aliases WHERE email = 'linked@icloud.com'`); last == "" {
		t.Error("last_mail_at not recorded on routed mail")
	}
}

func TestRecycleCandidateOrderAndProtection(t *testing.T) {
	a := newRuntimeTestApp(t, nil)
	ctx := context.Background()
	exec := func(q string) {
		t.Helper()
		if _, err := a.db.Exec(ctx, q); err != nil {
			t.Fatal(err)
		}
	}
	exec(`INSERT INTO hme_accounts(id, name) VALUES(1, 'a')`)
	exec(`INSERT INTO users(id, user_email, password) VALUES(9, 'u@example.com', 'x')`)
	old := "datetime('now', '-200 day')"
	// Protected: pinned, pooled, user-bound, not managed (made on an iPhone).
	exec(`INSERT INTO hme_aliases(email, account_id, active, managed, pinned, apple_created_at) VALUES('pinned@icloud.com', 1, 0, 1, 1, ` + old + `)`)
	exec(`INSERT INTO hme_aliases(email, account_id, active, managed, pooled, apple_created_at) VALUES('pool@icloud.com', 1, 1, 1, 1, ` + old + `)`)
	exec(`INSERT INTO hme_aliases(email, account_id, active, managed, apple_created_at) VALUES('iphone@icloud.com', 1, 0, 0, ` + old + `)`)
	exec(`INSERT INTO hme_aliases(email, account_id, active, managed, apple_created_at) VALUES('bound@icloud.com', 1, 0, 1, ` + old + `)`)
	exec(`INSERT INTO address(id, name) VALUES(50, 'bound@icloud.com')`)
	exec(`INSERT INTO users_address(user_id, address_id) VALUES(9, 50)`)
	// Recent or recently used aliases are kept.
	exec(`INSERT INTO hme_aliases(email, account_id, active, managed, apple_created_at) VALUES('fresh@icloud.com', 1, 1, 1, datetime('now', '-5 day'))`)
	exec(`INSERT INTO hme_aliases(email, account_id, active, managed, apple_created_at, last_mail_at) VALUES('busy@icloud.com', 1, 1, 1, ` + old + `, datetime('now', '-3 day'))`)

	pick := func() string {
		t.Helper()
		c, err := a.hmeRecycleCandidate(ctx, 1)
		if err != nil {
			t.Fatal(err)
		}
		if c == nil {
			return ""
		}
		return c.Email + "/" + c.Reason
	}
	if got := pick(); got != "" {
		t.Fatalf("protected alias picked: %s", got)
	}
	exec(`INSERT INTO hme_aliases(email, account_id, active, managed, apple_created_at, last_mail_at) VALUES('idle@icloud.com', 1, 1, 1, ` + old + `, datetime('now', '-120 day'))`)
	if got := pick(); got != "idle@icloud.com/idle" {
		t.Fatalf("want idle, got %s", got)
	}
	exec(`INSERT INTO hme_aliases(email, account_id, active, managed, apple_created_at) VALUES('unused@icloud.com', 1, 1, 1, ` + old + `)`)
	if got := pick(); got != "unused@icloud.com/never_received" {
		t.Fatalf("want never_received, got %s", got)
	}
	exec(`INSERT INTO hme_aliases(email, account_id, active, managed, apple_created_at) VALUES('off@icloud.com', 1, 0, 1, datetime('now', '-1 day'))`)
	if got := pick(); got != "off@icloud.com/inactive" {
		t.Fatalf("want inactive first, got %s", got)
	}
}

func TestThrottleBackoffSQL(t *testing.T) {
	a := newRuntimeTestApp(t, nil)
	ctx := context.Background()
	if _, err := a.db.Exec(ctx, `INSERT INTO hme_accounts(id, name) VALUES(1, 'a')`); err != nil {
		t.Fatal(err)
	}
	for _, want := range []int64{30, 60, 120, 240, 240} {
		if _, err := a.db.Exec(ctx, hmeThrottleSQL, 1); err != nil {
			t.Fatal(err)
		}
		got, _, _ := a.db.ScanInt(ctx, `SELECT CAST(round((julianday(cooldown_until) - julianday('now')) * 1440) AS INTEGER) FROM hme_accounts`)
		if got != want {
			t.Fatalf("cooldown %d min, want %d", got, want)
		}
	}
}

func TestCleanupKeepsHMEAliases(t *testing.T) {
	a := newRuntimeTestApp(t, nil)
	ctx := context.Background()
	for _, q := range []string{
		`INSERT INTO hme_accounts(id, name) VALUES(1, 'a')`,
		`INSERT INTO hme_aliases(email, account_id) VALUES('keep@icloud.com', 1)`,
		`INSERT INTO address(name, created_at, updated_at) VALUES('keep@icloud.com', datetime('now', '-60 day'), datetime('now', '-60 day')),
			('old@example.com', datetime('now', '-60 day'), datetime('now', '-60 day'))`,
	} {
		if _, err := a.db.Exec(ctx, q); err != nil {
			t.Fatal(err)
		}
	}
	for _, kind := range []string{"emptyAddress", "unboundAddress", "inactiveAddress", "addressCreated"} {
		if err := a.cleanup(ctx, kind, 30); err != nil {
			t.Fatalf("%s: %v", kind, err)
		}
	}
	if n, _ := a.db.Count(ctx, `SELECT count(*) FROM address WHERE name = 'keep@icloud.com'`); n != 1 {
		t.Fatal("iCloud alias mailbox was removed by cleanup")
	}
	if n, _ := a.db.Count(ctx, `SELECT count(*) FROM address WHERE name = 'old@example.com'`); n != 0 {
		t.Fatal("regular stale mailbox was kept")
	}
}

func TestPublicWebhookRejectsInternalTargets(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {}))
	defer srv.Close()
	if _, err := sendWebhook(webhookSettings{URL: srv.URL, TimeoutSeconds: 2, public: true}, map[string]any{}); err == nil || !strings.Contains(err.Error(), "public address") {
		t.Fatalf("loopback webhook was not blocked: %v", err)
	}
	if _, err := sendWebhook(webhookSettings{URL: srv.URL, TimeoutSeconds: 2}, map[string]any{}); err != nil {
		t.Fatalf("admin webhook to internal target failed: %v", err)
	}
}

func TestSecretIn(t *testing.T) {
	if !secretIn([]string{"a", "secret"}, "secret") || secretIn([]string{"secret"}, "secre") || secretIn([]string{""}, "") || secretIn(nil, "x") {
		t.Fatal("secretIn mismatch")
	}
}
