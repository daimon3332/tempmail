package icloud

import (
	"bytes"
	"context"
	"encoding/base64"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/http/cookiejar"
	"net/url"
	"strings"
)

const (
	widgetKey   = "d39ba9916b7251055b22c7f910e2ea796ee65e98b2ddecea8f5dde8d9d1a815d"
	authAgent   = "Mozilla/5.0 (Macintosh; Intel Mac OS X 10_15_7) AppleWebKit/605.1.15 (KHTML, like Gecko) Version/18.3.1 Safari/605.1.15"
	errBadLogin = "Apple ID or password is incorrect"
)

type Phone struct {
	ID     int    `json:"id"`
	Number string `json:"numberWithDialCode"`
}

// Login is an in-progress Apple ID sign-in; it survives between the password
// step and the two-factor code step.
type Login struct {
	AppleID string
	Phones  []Phone

	auth, home, setup                  string
	http                               *http.Client
	icloud                             *rawJar
	state                              string
	scnt, sessionID, authAttr, country string
	sessionToken, trustToken           string
}

func endpoints(host string) (auth, home, setup string) {
	if host == "icloud.com.cn" {
		return "https://idmsa.apple.com.cn/appleauth/auth", "https://www.icloud.com.cn", "https://setup.icloud.com.cn/setup/ws/1"
	}
	return "https://idmsa.apple.com/appleauth/auth", "https://www.icloud.com", "https://setup.icloud.com/setup/ws/1"
}

// StartLogin runs SRP sign-in. needCode reports a pending two-factor step.
func StartLogin(ctx context.Context, appleID, password, host, proxy string) (l *Login, needCode bool, err error) {
	jar, _ := cookiejar.New(nil)
	hc, err := httpClient(proxy, jar)
	if err != nil {
		return nil, false, err
	}
	l = &Login{AppleID: strings.TrimSpace(appleID), http: hc, icloud: newRawJar(), state: "auth-" + newUUID()}
	l.auth, l.home, l.setup = endpoints(NormalizeHost(host))

	q := url.Values{"frame_id": {l.state}, "skVersion": {"7"}, "iframeId": {l.state}, "client_id": {widgetKey},
		"response_type": {"code"}, "redirect_uri": {l.home}, "response_mode": {"web_message"}, "state": {l.state}, "authVersion": {"latest"}}
	if _, _, err := l.do(ctx, http.MethodGet, l.auth+"/authorize/signin?"+q.Encode(), nil, "text/html"); err != nil {
		return nil, false, err
	}
	srp, err := newSRPClient()
	if err != nil {
		return nil, false, err
	}
	status, body, err := l.do(ctx, http.MethodPost, l.auth+"/signin/init", map[string]any{
		"a": base64.StdEncoding.EncodeToString(srp.A.Bytes()), "accountName": l.AppleID, "protocols": []string{"s2k", "s2k_fo"},
	}, "")
	if err != nil {
		return nil, false, err
	}
	if status != http.StatusOK {
		return nil, false, errors.New(errBadLogin)
	}
	var init struct {
		Iteration int    `json:"iteration"`
		Salt      string `json:"salt"`
		Protocol  string `json:"protocol"`
		B         string `json:"b"`
		C         string `json:"c"`
	}
	if err := json.Unmarshal(body, &init); err != nil {
		return nil, false, errors.New("Apple sign-in returned invalid data")
	}
	salt, err1 := base64.StdEncoding.DecodeString(init.Salt)
	b, err2 := base64.StdEncoding.DecodeString(init.B)
	if err1 != nil || err2 != nil || init.Iteration <= 0 {
		return nil, false, errors.New("Apple sign-in returned invalid data")
	}
	key, err := passwordKey(password, init.Protocol, salt, init.Iteration)
	if err != nil {
		return nil, false, err
	}
	m1, m2, err := srp.process(l.AppleID, key, salt, b)
	if err != nil {
		return nil, false, err
	}
	status, _, err = l.do(ctx, http.MethodPost, l.auth+"/signin/complete?isRememberMeEnabled=true", map[string]any{
		"accountName": l.AppleID, "c": init.C, "m1": base64.StdEncoding.EncodeToString(m1),
		"m2": base64.StdEncoding.EncodeToString(m2), "rememberMe": true, "trustTokens": []string{},
	}, "")
	if err != nil {
		return nil, false, err
	}
	switch status {
	case http.StatusOK:
		return l, false, nil
	case http.StatusConflict:
		l.loadPhones(ctx)
		return l, true, nil
	case http.StatusPreconditionFailed:
		return nil, false, errors.New("accept the latest terms at account.apple.com first")
	case http.StatusUnauthorized, http.StatusForbidden:
		return nil, false, errors.New(errBadLogin)
	default:
		return nil, false, fmt.Errorf("Apple sign-in failed: HTTP %d", status)
	}
}

func (l *Login) loadPhones(ctx context.Context) {
	_, body, err := l.do(ctx, http.MethodGet, l.auth, nil, "")
	if err != nil {
		return
	}
	var opts struct {
		TrustedPhoneNumbers []Phone `json:"trustedPhoneNumbers"`
	}
	if json.Unmarshal(body, &opts) == nil {
		l.Phones = opts.TrustedPhoneNumbers
	}
}

// RequestSMS asks Apple to text a code to a trusted phone number.
func (l *Login) RequestSMS(ctx context.Context, phoneID int) error {
	status, _, err := l.do(ctx, http.MethodPut, l.auth+"/verify/phone", map[string]any{"phoneNumber": map[string]int{"id": phoneID}, "mode": "sms"}, "")
	if err != nil {
		return err
	}
	if status >= 300 {
		return fmt.Errorf("failed to send SMS code: HTTP %d", status)
	}
	return nil
}

// SubmitCode verifies a trusted-device code (phoneID 0) or an SMS code.
func (l *Login) SubmitCode(ctx context.Context, code string, phoneID int) error {
	path, payload := "/verify/trusteddevice/securitycode", map[string]any{"securityCode": map[string]string{"code": code}}
	if phoneID > 0 {
		path = "/verify/phone/securitycode"
		payload = map[string]any{"phoneNumber": map[string]int{"id": phoneID}, "securityCode": map[string]string{"code": code}, "mode": "sms"}
	}
	status, body, err := l.do(ctx, http.MethodPost, l.auth+path, payload, "")
	if err != nil {
		return err
	}
	accepted := status == http.StatusOK || status == http.StatusNoContent || status == http.StatusPreconditionFailed
	if status == http.StatusConflict {
		var v struct {
			SecurityCode struct {
				Valid bool `json:"valid"`
			} `json:"securityCode"`
		}
		accepted = json.Unmarshal(body, &v) == nil && v.SecurityCode.Valid
	}
	if !accepted {
		return errors.New("verification code is incorrect")
	}
	if _, _, err := l.do(ctx, http.MethodGet, l.auth+"/2sv/trust", nil, ""); err != nil {
		return err
	}
	return nil
}

// Finish exchanges the Apple ID session for iCloud web cookies.
func (l *Login) Finish(ctx context.Context) (map[string]string, error) {
	if l.sessionToken == "" {
		return nil, errors.New("Apple sign-in did not return a session token")
	}
	status, _, err := l.do(ctx, http.MethodPost, l.setup+"/accountLogin", map[string]any{
		"accountCountryCode": l.country, "dsWebAuthToken": l.sessionToken, "extended_login": true, "trustToken": l.trustToken,
	}, "")
	if err != nil {
		return nil, err
	}
	if status != http.StatusOK {
		return nil, fmt.Errorf("iCloud login failed: HTTP %d", status)
	}
	cookies := l.icloud.snapshot()
	if cookies["X-APPLE-WEBAUTH-TOKEN"] == "" {
		return nil, errors.New("iCloud login did not return session cookies")
	}
	return cookies, nil
}

func (l *Login) do(ctx context.Context, method, rawURL string, body any, accept string) (int, []byte, error) {
	var rd io.Reader
	if body != nil {
		b, _ := json.Marshal(body)
		rd = bytes.NewReader(b)
	}
	req, err := http.NewRequestWithContext(ctx, method, rawURL, rd)
	if err != nil {
		return 0, nil, err
	}
	authOrigin := strings.TrimSuffix(l.auth, "/appleauth/auth")
	h := req.Header
	if strings.HasPrefix(rawURL, l.setup) {
		h.Set("Origin", l.home)
		h.Set("Referer", l.home+"/")
		h.Set("Accept", "*/*")
	} else {
		if accept == "" {
			accept = "application/json, text/javascript"
		}
		h.Set("Accept", accept)
		h.Set("Origin", authOrigin)
		h.Set("Referer", authOrigin+"/")
		h.Set("X-Apple-OAuth-Client-Id", widgetKey)
		h.Set("X-Apple-OAuth-Client-Type", "firstPartyAuth")
		h.Set("X-Apple-OAuth-Redirect-URI", l.home)
		h.Set("X-Apple-OAuth-Require-Grant-Code", "true")
		h.Set("X-Apple-OAuth-Response-Mode", "web_message")
		h.Set("X-Apple-OAuth-Response-Type", "code")
		h.Set("X-Apple-OAuth-State", l.state)
		h.Set("X-Apple-Widget-Key", widgetKey)
		h.Set("X-Apple-Frame-Id", l.state)
		h.Set("X-Requested-With", "XMLHttpRequest")
		for k, v := range map[string]string{"scnt": l.scnt, "X-Apple-ID-Session-Id": l.sessionID, "X-Apple-Auth-Attributes": l.authAttr} {
			if v != "" {
				h.Set(k, v)
			}
		}
	}
	if body != nil {
		h.Set("Content-Type", "application/json")
	}
	h.Set("User-Agent", authAgent)
	resp, err := l.http.Do(req)
	if err != nil {
		return 0, nil, fmt.Errorf("Apple request failed: %w", err)
	}
	defer resp.Body.Close()
	for k, dst := range map[string]*string{
		"scnt": &l.scnt, "X-Apple-ID-Session-Id": &l.sessionID, "X-Apple-Auth-Attributes": &l.authAttr,
		"X-Apple-ID-Account-Country": &l.country, "X-Apple-Session-Token": &l.sessionToken, "X-Apple-TwoSV-Trust-Token": &l.trustToken,
	} {
		if v := resp.Header.Get(k); v != "" {
			*dst = v
		}
	}
	if strings.Contains(resp.Request.URL.Host, "icloud.com") {
		l.icloud.absorb(resp.Header.Values("Set-Cookie"))
	}
	data, _ := io.ReadAll(io.LimitReader(resp.Body, 4<<20))
	return resp.StatusCode, data, nil
}
