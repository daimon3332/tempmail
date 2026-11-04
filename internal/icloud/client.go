// Package icloud talks to the iCloud web endpoints behind Hide My Email.
package icloud

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strings"
	"sync"
	"time"
)

const (
	userAgent     = "Mozilla/5.0 (Windows NT 10.0; Win64; x64) AppleWebKit/537.36 (KHTML, like Gecko) Chrome/147.0.0.0 Safari/537.36"
	hmeBuild      = "2624Build22"
	setupBuild    = "2624Build13"
	rateLimitCode = "-41015"
)

var ErrUnauthorized = errors.New("iCloud session expired")

// APIError is a business error reported by the Hide My Email service.
type APIError struct {
	Code    string
	Message string
}

func (e *APIError) Error() string {
	if e.Code != "" {
		return fmt.Sprintf("iCloud %s: %s", e.Code, e.Message)
	}
	return "iCloud: " + e.Message
}

// IsRateLimited reports Apple's "too many addresses right now" throttle.
func IsRateLimited(err error) bool {
	var e *APIError
	return errors.As(err, &e) && (e.Code == rateLimitCode || strings.Contains(strings.ToLower(e.Message), "right now"))
}

// IsQuotaFull reports Apple's hard cap on existing addresses.
func IsQuotaFull(err error) bool {
	var e *APIError
	return errors.As(err, &e) && strings.Contains(strings.ToLower(e.Message), "maximum number")
}

type Alias struct {
	Email       string
	AnonymousID string
	Label       string
	Note        string
	Active      bool
	CreatedAt   time.Time
}

type List struct {
	Aliases        []Alias
	ForwardTo      string
	ForwardOptions []string
}

type Account struct {
	AppleID string
	DSID    string
}

// Client is bound to one iCloud account session. Cookies keep their raw
// (quoted) form because iCloud compares them byte for byte.
type Client struct {
	host string
	http *http.Client
	jar  *rawJar

	mu         sync.Mutex
	ServiceURL string
	DSID       string
	clientID   string
	validated  bool
}

func NewClient(cookies map[string]string, host, proxy string) (*Client, error) {
	hc, err := httpClient(proxy, nil)
	if err != nil {
		return nil, err
	}
	jar := newRawJar()
	for k, v := range cookies {
		jar.m[k] = v
	}
	return &Client{host: NormalizeHost(host), http: hc, jar: jar, clientID: newUUID()}, nil
}

func NormalizeHost(host string) string {
	if strings.HasSuffix(strings.ToLower(strings.TrimSpace(host)), "icloud.com.cn") {
		return "icloud.com.cn"
	}
	return "icloud.com"
}

func httpClient(proxy string, jar http.CookieJar) (*http.Client, error) {
	tr := http.DefaultTransport.(*http.Transport).Clone()
	if proxy = strings.TrimSpace(proxy); proxy != "" {
		u, err := url.Parse(proxy)
		if err != nil || u.Host == "" {
			return nil, errors.New("invalid proxy URL")
		}
		switch u.Scheme {
		case "http", "https", "socks5", "socks5h":
		default:
			return nil, errors.New("proxy must be http, https or socks5")
		}
		tr.Proxy = http.ProxyURL(u)
	}
	return &http.Client{Transport: tr, Jar: jar, Timeout: 30 * time.Second,
		CheckRedirect: func(*http.Request, []*http.Request) error { return http.ErrUseLastResponse }}, nil
}

func (c *Client) Cookies() map[string]string { return c.jar.snapshot() }

func originFor(rawURL string) string {
	if u, err := url.Parse(rawURL); err == nil && strings.HasSuffix(u.Hostname(), "icloud.com.cn") {
		return "https://www.icloud.com.cn"
	}
	return "https://www.icloud.com"
}

func (c *Client) do(ctx context.Context, method, rawURL string, body any) ([]byte, error) {
	u, err := url.Parse(rawURL)
	if err != nil {
		return nil, err
	}
	build := setupBuild
	if strings.Contains(u.Host, "maildomainws") {
		build = hmeBuild
	}
	q := u.Query()
	q.Set("clientBuildNumber", build)
	q.Set("clientMasteringNumber", build)
	q.Set("clientId", c.clientID)
	c.mu.Lock()
	if c.DSID != "" {
		q.Set("dsid", c.DSID)
	}
	c.mu.Unlock()
	u.RawQuery = q.Encode()

	var rd io.Reader
	if body != nil {
		b, err := json.Marshal(body)
		if err != nil {
			return nil, err
		}
		rd = bytes.NewReader(b)
	}
	req, err := http.NewRequestWithContext(ctx, method, u.String(), rd)
	if err != nil {
		return nil, err
	}
	origin := originFor(rawURL)
	req.Header.Set("Origin", origin)
	req.Header.Set("Referer", origin+"/")
	req.Header.Set("Accept", "*/*")
	req.Header.Set("Content-Type", "text/plain")
	req.Header.Set("User-Agent", userAgent)
	req.Header.Set("Cookie", c.jar.header())
	resp, err := c.http.Do(req)
	if err != nil {
		return nil, fmt.Errorf("iCloud request failed: %w", err)
	}
	defer resp.Body.Close()
	c.jar.absorb(resp.Header.Values("Set-Cookie"))
	data, _ := io.ReadAll(io.LimitReader(resp.Body, 8<<20))
	switch {
	case resp.StatusCode == 401 || resp.StatusCode == 403 || resp.StatusCode == 421:
		return nil, ErrUnauthorized
	case resp.StatusCode >= 300:
		return nil, fmt.Errorf("iCloud HTTP %d", resp.StatusCode)
	}
	return data, nil
}

// Validate refreshes the session cookies and resolves the Hide My Email
// service endpoint for this account.
func (c *Client) Validate(ctx context.Context) (*Account, error) {
	urls := []string{"https://setup." + c.host + "/setup/ws/1/validate"}
	if c.host == "icloud.com.cn" {
		urls = append(urls, "https://setup.icloud.com/setup/ws/1/validate")
	}
	var data []byte
	var err error
	for _, u := range urls {
		if data, err = c.do(ctx, http.MethodPost, u, nil); err == nil {
			break
		}
	}
	if err != nil {
		return nil, err
	}
	var v struct {
		DsInfo struct {
			Dsid         string `json:"dsid"`
			AppleID      string `json:"appleId"`
			PrimaryEmail string `json:"primaryEmail"`
		} `json:"dsInfo"`
		Webservices map[string]struct {
			URL string `json:"url"`
		} `json:"webservices"`
	}
	if err := json.Unmarshal(data, &v); err != nil {
		return nil, errors.New("iCloud validate returned invalid JSON")
	}
	svc := strings.TrimSuffix(strings.TrimRight(v.Webservices["premiummailsettings"].URL, "/"), ":443")
	if svc == "" {
		return nil, errors.New("Hide My Email is unavailable (iCloud+ required)")
	}
	c.mu.Lock()
	c.ServiceURL, c.DSID, c.validated = svc, v.DsInfo.Dsid, true
	c.mu.Unlock()
	appleID := v.DsInfo.AppleID
	if appleID == "" {
		appleID = v.DsInfo.PrimaryEmail
	}
	return &Account{AppleID: appleID, DSID: v.DsInfo.Dsid}, nil
}

func (c *Client) service(ctx context.Context) (string, error) {
	c.mu.Lock()
	svc := c.ServiceURL
	c.mu.Unlock()
	if svc != "" {
		return svc, nil
	}
	if _, err := c.Validate(ctx); err != nil {
		return "", err
	}
	return c.ServiceURL, nil
}

type hmeEmail struct {
	Hme             string `json:"hme"`
	AnonymousID     string `json:"anonymousId"`
	Label           string `json:"label"`
	Note            string `json:"note"`
	IsActive        bool   `json:"isActive"`
	CreateTimestamp int64  `json:"createTimestamp"`
}

func (e hmeEmail) alias() Alias {
	a := Alias{Email: strings.ToLower(strings.TrimSpace(e.Hme)), AnonymousID: e.AnonymousID, Label: e.Label, Note: e.Note, Active: e.IsActive}
	if e.CreateTimestamp > 0 {
		a.CreatedAt = time.UnixMilli(e.CreateTimestamp).UTC()
	}
	return a
}

// call performs a premiummailsettings request and decodes result into out.
func (c *Client) call(ctx context.Context, method, path string, body, out any) error {
	svc, err := c.service(ctx)
	if err != nil {
		return err
	}
	data, err := c.do(ctx, method, svc+path, body)
	c.mu.Lock()
	stale := !c.validated
	c.mu.Unlock()
	if errors.Is(err, ErrUnauthorized) && stale {
		// A cached service URL may be outdated; revalidate once before
		// declaring the session expired.
		if _, err = c.Validate(ctx); err != nil {
			return err
		}
		data, err = c.do(ctx, method, c.ServiceURL+path, body)
	}
	if err != nil {
		return err
	}
	var env struct {
		Success      bool            `json:"success"`
		Result       json.RawMessage `json:"result"`
		ErrorCode    any             `json:"errorCode"`
		ErrorMessage string          `json:"errorMessage"`
		Error        *struct {
			ErrorCode    any    `json:"errorCode"`
			ErrorMessage string `json:"errorMessage"`
		} `json:"error"`
	}
	if err := json.Unmarshal(data, &env); err != nil {
		return errors.New("iCloud returned invalid JSON")
	}
	if !env.Success {
		e := &APIError{Message: env.ErrorMessage}
		if env.ErrorCode != nil {
			e.Code = fmt.Sprint(env.ErrorCode)
		}
		if env.Error != nil {
			if env.Error.ErrorCode != nil {
				e.Code = fmt.Sprint(env.Error.ErrorCode)
			}
			if env.Error.ErrorMessage != "" {
				e.Message = env.Error.ErrorMessage
			}
		}
		if e.Message == "" {
			e.Message = "request rejected"
		}
		return e
	}
	if out != nil && len(env.Result) > 0 {
		return json.Unmarshal(env.Result, out)
	}
	return nil
}

func (c *Client) List(ctx context.Context) (*List, error) {
	var r struct {
		HmeEmails         []hmeEmail `json:"hmeEmails"`
		SelectedForwardTo string     `json:"selectedForwardTo"`
		ForwardToEmails   []string   `json:"forwardToEmails"`
	}
	if err := c.call(ctx, http.MethodGet, "/v2/hme/list", nil, &r); err != nil {
		return nil, err
	}
	out := &List{ForwardTo: r.SelectedForwardTo, ForwardOptions: r.ForwardToEmails, Aliases: make([]Alias, 0, len(r.HmeEmails))}
	for _, e := range r.HmeEmails {
		if a := e.alias(); strings.Contains(a.Email, "@") {
			out.Aliases = append(out.Aliases, a)
		}
	}
	return out, nil
}

// Create generates a candidate address and reserves it.
func (c *Client) Create(ctx context.Context, label, note string) (Alias, error) {
	var gen struct {
		Hme string `json:"hme"`
	}
	if err := c.call(ctx, http.MethodPost, "/v1/hme/generate", map[string]string{"langCode": "en-us"}, &gen); err != nil {
		return Alias{}, err
	}
	if gen.Hme == "" {
		return Alias{}, errors.New("iCloud generated an empty address")
	}
	var res struct {
		Hme hmeEmail `json:"hme"`
	}
	if err := c.call(ctx, http.MethodPost, "/v1/hme/reserve", map[string]string{"hme": gen.Hme, "label": label, "note": note}, &res); err != nil {
		return Alias{}, err
	}
	if res.Hme.Hme == "" {
		res.Hme = hmeEmail{Hme: gen.Hme, Label: label, Note: note, IsActive: true}
	}
	a := res.Hme.alias()
	if a.CreatedAt.IsZero() {
		a.CreatedAt = time.Now().UTC()
	}
	return a, nil
}

func (c *Client) Deactivate(ctx context.Context, anonymousID string) error {
	return c.call(ctx, http.MethodPost, "/v1/hme/deactivate", map[string]string{"anonymousId": anonymousID}, nil)
}

func (c *Client) Reactivate(ctx context.Context, anonymousID string) error {
	return c.call(ctx, http.MethodPost, "/v1/hme/reactivate", map[string]string{"anonymousId": anonymousID}, nil)
}

// Delete permanently removes an address; Apple only deletes inactive ones.
func (c *Client) Delete(ctx context.Context, anonymousID string) error {
	return c.call(ctx, http.MethodPost, "/v1/hme/delete", map[string]string{"anonymousId": anonymousID}, nil)
}

func (c *Client) UpdateMeta(ctx context.Context, anonymousID, label, note string) error {
	return c.call(ctx, http.MethodPost, "/v1/hme/updateMetaData", map[string]string{"anonymousId": anonymousID, "label": label, "note": note}, nil)
}

func (c *Client) SetForwardTo(ctx context.Context, email string) error {
	return c.call(ctx, http.MethodPost, "/v1/hme/updateForwardTo", map[string]string{"forwardToEmail": email}, nil)
}
