package server

import (
	"bytes"
	"compress/gzip"
	"context"
	"crypto/hmac"
	"crypto/rand"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"math/big"
	"net/http"
	"net/url"
	"regexp"
	"strings"
	"time"

	"tempmail/internal/db"
	"tempmail/internal/mailparse"
)

var (
	defaultNameRegex = regexp.MustCompile(`[^a-z0-9]`)
	domainLabelRe    = regexp.MustCompile(`^[a-z0-9](?:[a-z0-9-]{0,61}[a-z0-9])?$`)
)

func randomString(n int, charset string) string {
	b := make([]byte, n)
	for i := range b {
		idx, _ := rand.Int(rand.Reader, big.NewInt(int64(len(charset))))
		b[i] = charset[idx.Int64()]
	}
	return string(b)
}

func (a *App) generateRandomName() string {
	rc := a.effective(context.Background())
	min, max := rc.MinAddressLen, rc.MaxAddressLen
	if min < 1 {
		min = 1
	}
	if max < 1 {
		max = 1
	}
	name := ""
	for len(name) < min {
		name += randomString(11, "abcdefghijklmnopqrstuvwxyz0123456789")
	}
	if len(name) > max {
		name = name[:max]
	}
	return name
}

func mailDomain(address string) string {
	i := strings.LastIndex(address, "@")
	if i < 0 {
		return ""
	}
	return strings.ToLower(strings.TrimSpace(address[i+1:]))
}

func isDomainOrSubdomain(domain, base string) bool {
	domain, base = strings.ToLower(domain), strings.ToLower(base)
	return domain != "" && base != "" && (domain == base || strings.HasSuffix(domain, "."+base))
}

func validLabels(labels []string) bool {
	if len(labels) == 0 {
		return false
	}
	for _, l := range labels {
		if !domainLabelRe.MatchString(l) {
			return false
		}
	}
	return true
}

func findAllowedDomain(domain string, allow []string, subdomainMatch bool) string {
	domain = strings.ToLower(strings.TrimSpace(domain))
	if len(domain) > 253 {
		return ""
	}
	labels := strings.Split(domain, ".")
	if !validLabels(labels) {
		return ""
	}
	for _, d := range allow {
		if d == domain {
			return domain
		}
	}
	if !subdomainMatch {
		return ""
	}
	best := ""
	for _, d := range allow {
		if len(d) < len(best) {
			continue
		}
		dl := strings.Split(d, ".")
		if !validLabels(dl) || len(labels) <= len(dl) {
			continue
		}
		if !validLabels(labels[:len(labels)-len(dl)]) {
			continue
		}
		if strings.Join(labels[len(labels)-len(dl):], ".") == d {
			best = d
		}
	}
	if best == "" {
		return ""
	}
	return domain
}

func (a *App) allowRandomSubdomain(ctx context.Context, domain string) bool {
	for _, d := range a.effective(ctx).RandomSubdomainDomains {
		if d == strings.ToLower(domain) {
			return true
		}
	}
	return false
}

func (a *App) jsonSetting(ctx context.Context, key string, v any) bool {
	raw, err := a.db.GetSetting(ctx, key)
	if err != nil || raw == "" {
		return false
	}
	return json.Unmarshal([]byte(raw), v) == nil
}

func (a *App) saveJSONSetting(ctx context.Context, key string, v any) error {
	b, err := json.Marshal(v)
	if err != nil {
		return err
	}
	return a.db.SaveSetting(ctx, key, string(b))
}

func (a *App) subdomainMatchEnabled(ctx context.Context) bool {
	var s struct {
		EnableSubdomainMatch *bool `json:"enableSubdomainMatch"`
	}
	if a.jsonSetting(ctx, "address_creation_settings", &s) && s.EnableSubdomainMatch != nil {
		return *s.EnableSubdomainMatch
	}
	return a.cfg.EnableCreateAddressSubdomainMatch
}

type newAddressOpts struct {
	name                  string
	domain                string
	enablePrefix          bool
	enableRandomSubdomain bool
	checkLengthByConfig   bool
	addressPrefix         *string
	allowDomains          []string
	enableCheckNameRegex  bool
	sourceMeta            string
}

type newAddressResult struct {
	JWT       string  `json:"jwt"`
	Address   string  `json:"address"`
	Password  *string `json:"password"`
	AddressID int64   `json:"address_id"`
}

func (a *App) newAddress(ctx context.Context, o newAddressOpts) (*newAddressResult, error) {
	rc := a.effective(ctx)
	name := strings.TrimSpace(o.name)
	nameRe := defaultNameRegex
	if rc.AddressRegex != "" {
		if re, err := regexp.Compile(rc.AddressRegex); err == nil {
			nameRe = re
		}
	}
	name = nameRe.ReplaceAllString(name, "")
	if o.enableCheckNameRegex {
		var block []string
		a.jsonSetting(ctx, "address_block_list", &block)
		for _, b := range block {
			if b != "" && strings.Contains(name, b) {
				return nil, fmt.Errorf("Name[%s]is blocked", name)
			}
		}
		if rc.AddressCheckRegex != "" {
			if re, err := regexp.Compile(rc.AddressCheckRegex); err == nil && !re.MatchString(name) {
				return nil, fmt.Errorf("Name not match regex: /%s/", rc.AddressCheckRegex)
			}
		}
	}
	minLen, maxLen := 1, 30
	if o.checkLengthByConfig {
		minLen, maxLen = rc.MinAddressLen, rc.MaxAddressLen
	}
	if minLen < 1 {
		minLen = 1
	}
	if maxLen < 1 {
		maxLen = 1
	}
	if len(name) < minLen {
		return nil, fmt.Errorf("Name too short (min %d)", minLen)
	}
	if len(name) > maxLen {
		return nil, fmt.Errorf("Name too long (max %d)", maxLen)
	}
	if o.addressPrefix != nil {
		name = strings.ToLower(strings.TrimSpace(*o.addressPrefix)) + name
	} else if o.enablePrefix {
		name = rc.Prefix + name
	}
	allow := o.allowDomains
	if len(allow) == 0 {
		allow = a.effective(ctx).Domains
	}
	domain := strings.ToLower(strings.TrimSpace(o.domain))
	if domain == "" && len(allow) > 0 {
		if a.cfg.CreateAddressDefaultDomainFirst {
			domain = allow[0]
		} else {
			idx, _ := rand.Int(rand.Reader, big.NewInt(int64(len(allow))))
			domain = allow[idx.Int64()]
		}
	}
	subMatch := a.subdomainMatchEnabled(ctx)
	manualSub := false
	for _, base := range allow {
		if a.allowRandomSubdomain(ctx, base) && isDomainOrSubdomain(domain, base) {
			manualSub = true
		}
	}
	if domain == "" || findAllowedDomain(domain, allow, subMatch || manualSub) == "" {
		return nil, errors.New("Invalid domain")
	}
	if o.enableRandomSubdomain && !a.allowRandomSubdomain(ctx, domain) {
		return nil, errors.New("Random subdomain is not allowed for this domain")
	}
	attempts := 1
	if o.enableRandomSubdomain {
		attempts = 5
	}
	for i := 0; i < attempts; i++ {
		addrDomain := domain
		if o.enableRandomSubdomain {
			n := rc.RandomSubdomainLength
			if n < 1 {
				n = 1
			}
			if n > 63 {
				n = 63
			}
			addrDomain = randomString(n, "abcdefghijklmnopqrstuvwxyz0123456789") + "." + domain
		}
		address := name + "@" + addrDomain
		var sourceMeta any
		if o.sourceMeta != "" {
			sourceMeta = o.sourceMeta
		}
		_, err := a.db.Exec(ctx, `INSERT INTO address(name, source_meta) VALUES(?, ?)`, address, sourceMeta)
		if err != nil {
			if strings.Contains(err.Error(), "UNIQUE") {
				if o.enableRandomSubdomain && i < attempts-1 {
					continue
				}
				return nil, errors.New("Address already exists")
			}
			return nil, errors.New("Failed to create address")
		}
		id, found, err := a.db.ScanInt(ctx, `SELECT id FROM address WHERE name = ?`, address)
		if err != nil || !found {
			return nil, errors.New("Failed to create address")
		}
		token, err := a.jwt.AddressToken(address, id)
		if err != nil {
			return nil, err
		}
		return &newAddressResult{JWT: token, Address: address, Password: nil, AddressID: id}, nil
	}
	return nil, errors.New("Failed to create address")
}

func (a *App) touchAddress(ctx context.Context, address string) {
	if address == "" {
		return
	}
	a.db.Exec(ctx, `UPDATE address SET updated_at = datetime('now') WHERE name = ?
		AND (updated_at IS NULL OR updated_at < datetime('now', '-1 day'))`, address)
}

func (a *App) touchUserAddresses(ctx context.Context, userID int64) {
	a.db.Exec(ctx, `UPDATE address SET updated_at = datetime('now')
		WHERE id IN (SELECT address_id FROM users_address WHERE user_id = ?)
		AND (updated_at IS NULL OR updated_at < datetime('now', '-1 day'))`, userID)
}

// listQuery returns a stable total count so clients can render pagination.
func (a *App) listQuery(w http.ResponseWriter, r *http.Request, query, countQuery string, params []any,
	limitStr, offsetStr, orderBy string, hidden ...string) {
	limit, offset := atoi(limitStr), atoi(offsetStr)
	if limit <= 0 || limit > 100 {
		text(w, 400, "Invalid limit")
		return
	}
	if strings.TrimSpace(offsetStr) == "" || offset < 0 {
		text(w, 400, "Invalid offset")
		return
	}
	if orderBy == "" {
		orderBy = "id desc"
	}
	ctx := r.Context()
	rows, err := a.db.Query(ctx, fmt.Sprintf("%s order by %s limit ? offset ?", query, orderBy), append(append([]any{}, params...), limit, offset)...)
	if err != nil {
		fail(w, err)
		return
	}
	for _, row := range rows {
		for _, h := range hidden {
			delete(row, h)
		}
		resolveRaw(row)
	}
	var count int64
	count, _ = a.db.Count(ctx, countQuery, params...)
	jsonResp(w, 200, map[string]any{"results": rows, "count": count})
}

func resolveRaw(row db.Row) {
	if blob, ok := row["raw_blob"]; ok {
		if s, isStr := blob.(string); isStr && s != "" {
			if zr, err := gzip.NewReader(bytes.NewReader([]byte(s))); err == nil {
				if b, err := io.ReadAll(zr); err == nil {
					row["raw"] = string(b)
				}
			}
		}
		delete(row, "raw_blob")
	}
}

func (a *App) parseRawFromRow(row db.Row) *mailparse.Mail {
	if row == nil {
		return nil
	}
	resolveRaw(row)
	m, err := mailparse.Parse([]byte(row.Str("raw")))
	if err != nil {
		return nil
	}
	return m
}

func (a *App) userRolePrefix(ctx context.Context, r *http.Request) *string {
	rc := a.effective(ctx)
	u := userOf(r)
	if u == nil {
		p := rc.Prefix
		return &p
	}
	role, _ := a.roles.UserRole(ctx, claimInt(u, "user_id"))
	if role != nil && role.Prefix != nil {
		p := strings.ToLower(strings.TrimSpace(*role.Prefix))
		return &p
	}
	p := rc.Prefix
	return &p
}

func (a *App) allowDomainsFor(ctx context.Context, r *http.Request) []string {
	u := userOf(r)
	if u == nil {
		return a.effective(ctx).DefaultDomains
	}
	role, _ := a.roles.UserRole(ctx, claimInt(u, "user_id"))
	if role != nil && len(role.Domains) > 0 {
		return role.Domains
	}
	return a.effective(ctx).DefaultDomains
}

// ---- webhook ----

type webhookSettings struct {
	Enabled        bool   `json:"enabled"`
	URL            string `json:"url"`
	Method         string `json:"method"`
	Headers        string `json:"headers"`
	Body           string `json:"body"`
	Secret         string `json:"secret,omitempty"`
	TimeoutSeconds int    `json:"timeout_seconds,omitempty"`
	MaxRetries     int    `json:"max_retries,omitempty"`
}

func defaultWebhookSettings() webhookSettings {
	body, _ := json.MarshalIndent(map[string]string{
		"id": "${id}", "url": "${url}", "from": "${from}", "to": "${to}", "subject": "${subject}",
		"raw": "${raw}", "parsedText": "${parsedText}", "parsedHtml": "${parsedHtml}",
		"aiExtractType": "${aiExtractType}", "aiExtractResult": "${aiExtractResult}", "aiExtractResultText": "${aiExtractResultText}",
	}, "", "  ")
	return webhookSettings{Method: "POST", Headers: "{\n  \"Content-Type\": \"application/json\"\n}", Body: string(body), TimeoutSeconds: 10, MaxRetries: 3}
}

type adminWebhookSettings struct {
	EnableAllowList bool     `json:"enableAllowList"`
	AllowList       []string `json:"allowList"`
}

func validateWebhookSettings(s webhookSettings) error {
	endpoint := strings.TrimSpace(s.URL)
	if endpoint == "" && s.Enabled {
		return errors.New("webhook URL is required")
	}
	if endpoint != "" {
		u, err := url.Parse(endpoint)
		if err != nil || u.Host == "" || (u.Scheme != "http" && u.Scheme != "https") {
			return errors.New("webhook URL must be http or https")
		}
	}
	method := strings.ToUpper(strings.TrimSpace(s.Method))
	if method == "" {
		method = "POST"
	}
	if method != "POST" && method != "PUT" && method != "PATCH" {
		return errors.New("webhook method must be POST, PUT or PATCH")
	}
	if s.Headers != "" {
		var headers map[string]string
		if json.Unmarshal([]byte(s.Headers), &headers) != nil {
			return errors.New("webhook headers must be valid JSON")
		}
	}
	if len(s.Body) > 256*1024 {
		return errors.New("webhook body template is too large")
	}
	if s.TimeoutSeconds < 0 || s.TimeoutSeconds > 60 {
		return errors.New("webhook timeout must be between 1 and 60 seconds")
	}
	if s.MaxRetries < 0 || s.MaxRetries > 5 {
		return errors.New("webhook retries must be between 0 and 5")
	}
	return nil
}

func sendWebhook(s webhookSettings, values map[string]any) (int, error) {
	body := s.Body
	for k, v := range values {
		enc, _ := json.Marshal(v)
		str := string(enc)
		if len(str) >= 2 && str[0] == '"' {
			str = str[1 : len(str)-1]
		}
		body = strings.ReplaceAll(body, "${"+k+"}", str)
	}
	headers := map[string]string{}
	_ = json.Unmarshal([]byte(s.Headers), &headers)
	method := s.Method
	if method == "" {
		method = "POST"
	}
	req, err := http.NewRequest(method, s.URL, strings.NewReader(body))
	if err != nil {
		return 0, err
	}
	for k, v := range headers {
		req.Header.Set(k, v)
	}
	if s.Secret != "" {
		ts := fmt.Sprint(time.Now().Unix())
		sig := hmac.New(sha256.New, []byte(s.Secret))
		_, _ = sig.Write([]byte(ts + "." + body))
		req.Header.Set("X-Tempmail-Timestamp", ts)
		req.Header.Set("X-Tempmail-Signature", "sha256="+hex.EncodeToString(sig.Sum(nil)))
	}
	req.Header.Set("X-Tempmail-Event-ID", fmt.Sprint(values["id"]))
	timeout := s.TimeoutSeconds
	if timeout < 1 {
		timeout = 10
	}
	client := &http.Client{Timeout: time.Duration(timeout) * time.Second}
	resp, err := client.Do(req)
	if err != nil {
		return 0, err
	}
	defer resp.Body.Close()
	if resp.StatusCode >= 300 {
		return resp.StatusCode, fmt.Errorf("send webhook error: %s", resp.Status)
	}
	return resp.StatusCode, nil
}

func (a *App) deliverWebhook(ctx context.Context, s webhookSettings, values map[string]any) {
	attempts := s.MaxRetries + 1
	if attempts < 1 {
		attempts = 1
	}
	delays := []time.Duration{time.Second, 5 * time.Second, 30 * time.Second, 2 * time.Minute, 5 * time.Minute}
	for attempt := 1; attempt <= attempts; attempt++ {
		status, err := sendWebhook(s, values)
		errText := ""
		if err != nil {
			errText = err.Error()
		}
		_, _ = a.db.Exec(ctx, `INSERT INTO webhook_deliveries(event_id,endpoint,attempt,status_code,error) VALUES(?,?,?,?,?)`, fmt.Sprint(values["id"]), s.URL, attempt, status, errText)
		if err == nil {
			return
		}
		if attempt < attempts {
			delay := delays[attempt-1]
			select {
			case <-ctx.Done():
				return
			case <-time.After(delay):
			}
		}
	}
}

func (a *App) webhookValues(id any, from, to string, raw string, parsed *mailparse.Mail, ai *ExtractResult) map[string]any {
	url := ""
	if a.cfg.FrontendURL != "" {
		url = fmt.Sprintf("%s?mail_id=%v", a.cfg.FrontendURL, id)
	}
	v := map[string]any{
		"id": id, "url": url, "from": from, "to": to, "subject": "", "raw": raw,
		"parsedText": "", "parsedHtml": "", "aiExtract": nil, "aiExtractType": "", "aiExtractResult": "", "aiExtractResultText": "",
	}
	if parsed != nil {
		v["subject"] = parsed.Subject
		v["parsedText"] = parsed.Text
		v["parsedHtml"] = parsed.HTML
		if from == "" {
			v["from"] = parsed.Sender
		}
	}
	if ai != nil {
		v["aiExtract"], v["aiExtractType"], v["aiExtractResult"], v["aiExtractResultText"] = ai, ai.Type, ai.Result, ai.ResultText
	}
	return v
}

// TriggerWebhook is invoked by the inbound SMTP pipeline after a mail is stored.
func (a *App) TriggerWebhook(ctx context.Context, address string, mailID int64, raw string, parsed *mailparse.Mail, metadata string) {
	if !a.effective(ctx).EnableWebhook {
		return
	}
	var hooks []webhookSettings
	var adminHook webhookSettings
	if a.jsonSetting(ctx, "temp-mail-webhook-admin-mail-settings", &adminHook) && adminHook.Enabled {
		hooks = append(hooks, adminHook)
	}
	var adminSettings adminWebhookSettings
	a.jsonSetting(ctx, "temp-mail-webhook-settings", &adminSettings)
	if !adminSettings.EnableAllowList || contains(adminSettings.AllowList, address) {
		var userHook webhookSettings
		if a.jsonSetting(ctx, "temp-mail-webhook-user-settings:"+address, &userHook) && userHook.Enabled {
			hooks = append(hooks, userHook)
		}
	}
	from := ""
	if parsed != nil {
		from = parsed.Sender
	}
	for _, h := range hooks {
		a.deliverWebhook(context.Background(), h, a.webhookValues(mailID, from, address, raw, parsed, aiFromMetadata(metadata)))
	}
}

func claimInt(c map[string]any, key string) int64 {
	switch v := c[key].(type) {
	case float64:
		return int64(v)
	case int64:
		return v
	}
	return 0
}

func claimStr(c map[string]any, key string) string {
	s, _ := c[key].(string)
	return s
}
