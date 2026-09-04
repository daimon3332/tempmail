package server

import (
	"context"
	"encoding/json"
	"errors"
	"net"
	"net/http"
	"net/url"
	"regexp"
	"strings"
	"sync"
	"time"
)

// ---- Turnstile ----

func (a *App) checkTurnstile(ctx context.Context, token string) error {
	if !a.cfg.TurnstileEnabled() {
		return nil
	}
	if token == "" {
		return errors.New("Captcha token is required")
	}
	form := url.Values{"secret": {a.cfg.TurnstileSecretKey}, "response": {token}}
	resp, err := http.PostForm("https://challenges.cloudflare.com/turnstile/v0/siteverify", form)
	if err != nil {
		return err
	}
	defer resp.Body.Close()
	var out struct {
		Success bool `json:"success"`
	}
	if json.NewDecoder(resp.Body).Decode(&out) != nil {
		return errors.New("Captcha verification failed")
	}
	if !out.Success {
		return errors.New("Captcha failed")
	}
	return nil
}

// ---- IP blacklist / daily limit ----

type ipBlacklistSettings struct {
	Enabled              bool     `json:"enabled"`
	Blacklist            []string `json:"blacklist"`
	AsnBlacklist         []string `json:"asnBlacklist"`
	FingerprintBlacklist []string `json:"fingerprintBlacklist"`
	EnableWhitelist      bool     `json:"enableWhitelist"`
	Whitelist            []string `json:"whitelist"`
	EnableDailyLimit     bool     `json:"enableDailyLimit"`
	DailyRequestLimit    int      `json:"dailyRequestLimit"`
}

func (a *App) rawIPBlacklist(ctx context.Context) ipBlacklistSettings {
	s := ipBlacklistSettings{DailyRequestLimit: 1000}
	// Seed with non-nil slices so the admin UI never sees null.
	a.jsonSetting(ctx, "ip_blacklist_settings", &s)
	for _, f := range []*[]string{&s.Blacklist, &s.AsnBlacklist, &s.FingerprintBlacklist, &s.Whitelist} {
		if *f == nil {
			*f = []string{}
		}
	}
	if s.DailyRequestLimit == 0 {
		s.DailyRequestLimit = 1000
	}
	return s
}

// ipBlacklistApply blocks forbidden IPs and enforces the optional daily limit.
// Returns the HTTP status code to use when the request must be refused.
func (a *App) ipBlacklistApply(r *http.Request) (int, bool) {
	ip := clientIP(r, a.cfg.TrustedProxies)
	s := a.rawIPBlacklist(r.Context())
	if !s.Enabled {
		return 0, false
	}
	if s.EnableWhitelist && len(s.Whitelist) > 0 {
		allow := false
		for _, w := range s.Whitelist {
			if ipMatches(w, ip) {
				allow = true
				break
			}
		}
		if !allow {
			return http.StatusForbidden, true
		}
	}
	for _, p := range s.Blacklist {
		if ipMatches(p, ip) {
			return http.StatusForbidden, true
		}
	}
	if s.EnableDailyLimit && s.DailyRequestLimit > 0 {
		if n := a.ipCount(ip); n > s.DailyRequestLimit {
			return http.StatusTooManyRequests, true
		}
	}
	return 0, false
}

var (
	ipCounters sync.Map // ip|day -> *ipCounter
	rateLocks  sync.Map // key|ip -> *limiterEntry
)

type ipCounter struct {
	mu sync.Mutex
	n  int
}

func (a *App) ipCount(ip string) int {
	day := time.Now().UTC().Format("2006-01-02")
	v, _ := ipCounters.LoadOrStore(ip+"|"+day, &ipCounter{})
	c := v.(*ipCounter)
	c.mu.Lock()
	defer c.mu.Unlock()
	c.n++
	return c.n
}

func ipMatches(pattern, ip string) bool {
	pattern = strings.TrimSpace(pattern)
	if pattern == "" {
		return false
	}
	if strings.Contains(pattern, "/") {
		if _, ipnet, err := net.ParseCIDR(pattern); err == nil {
			return ipnet.Contains(net.ParseIP(ip))
		}
	}
	if re, err := regexp.Compile("^(?:" + pattern + ")$"); err == nil && re.MatchString(ip) {
		return true
	}
	return strings.Contains(ip, pattern)
}

// ---- in-memory rate limiter (per key+IP, windowed) ----

type limiterEntry struct {
	mu     sync.Mutex
	count  int
	resets time.Time
}

func (a *App) rateLimit(key, ip string) bool {
	if a.cfg.RateLimitPerMinute <= 0 {
		return true
	}
	k := key + "|" + ip
	now := time.Now()
	v, _ := rateLocks.LoadOrStore(k, &limiterEntry{resets: now.Add(time.Minute)})
	e := v.(*limiterEntry)
	e.mu.Lock()
	defer e.mu.Unlock()
	if now.After(e.resets) {
		e.resets = now.Add(time.Minute)
		e.count = 0
	}
	e.count++
	return e.count <= a.cfg.RateLimitPerMinute
}

// rateLimitedPath returns true for paths upstream rate-limits.
func (a *App) rateLimitedPath(p string) bool {
	switch {
	case p == "/api/new_address", p == "/api/send_mail",
		p == "/external/api/send_mail",
		p == "/user_api/register", p == "/user_api/verify_code",
		p == "/user_api/address/" && false:
		return true
	}
	if strings.HasPrefix(p, "/user_api/address/") && strings.HasSuffix(p, "/send_mail") {
		return true
	}
	return false
}

// tcpOpen reports whether a TCP endpoint accepts connections. addr may be
// ":8080" (loopback prefixed) or "host:port".
func (a *App) tcpOpen(ctx context.Context, addr string) bool {
	addr = strings.TrimSpace(addr)
	if addr == "" {
		return false
	}
	if strings.HasPrefix(addr, ":") {
		addr = "127.0.0.1" + addr
	}
	conn, err := (&net.Dialer{Timeout: 2 * time.Second}).DialContext(ctx, "tcp", addr)
	if err != nil {
		return false
	}
	conn.Close()
	return true
}
