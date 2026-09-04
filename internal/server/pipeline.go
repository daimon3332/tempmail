package server

import (
	"bytes"
	"context"
	"encoding/json"
	"log"
	"net/http"
	"regexp"
	"strings"
	"time"

	"tempmail/internal/mailparse"
)

// ExtractResult mirrors upstream's AI extraction output stored in
// raw_mails.metadata as {"ai_extract": {...}}.
type ExtractResult struct {
	Type       string `json:"type"`
	Result     string `json:"result"`
	ResultText string `json:"result_text"`
}

type aiExtractSettings struct {
	Enabled             bool     `json:"enabled"`
	EnableAllowList     bool     `json:"enableAllowList"`
	AllowList           []string `json:"allowList"`
	EnableRegexFallback bool     `json:"enableRegexFallback"`
}

func (a *App) aiSettings(ctx context.Context) aiExtractSettings {
	s := aiExtractSettings{AllowList: []string{}, EnableRegexFallback: true}
	a.jsonSetting(ctx, "ai_extract_settings", &s)
	if s.AllowList == nil {
		s.AllowList = []string{}
	}
	return s
}

const aiPrompt = `You are an expert email analyzer. Extract the single most important item by priority:
1. auth_code: verification/OTP/security code (return only the code, no spaces or hyphens)
2. auth_link: verify/confirm/activate/login/reset URL (complete http(s) URL from the email only)
3. service_link: commit/PR/issue/deployment URL
4. subscription_link: unsubscribe/manage preferences URL
5. other_link: any other useful URL
6. none
Never fabricate or alter URLs. Return ONLY JSON: {"type":"auth_code|auth_link|service_link|subscription_link|other_link|none","result":"...","result_text":"short display text or empty"}`

var (
	codeRe  = regexp.MustCompile(`(?i)(?:code|码|otp|pin|passcode|verification)[^0-9a-z]{0,40}?\b([0-9]{4,8}|[A-Z0-9]{6,8})\b`)
	digitRe = regexp.MustCompile(`\b(\d{6})\b`)
	linkRe  = regexp.MustCompile(`https?://[^\s"'<>)]+`)
)

// regexExtract is the offline fallback (upstream extract_code.ts equivalent).
func regexExtract(m *mailparse.Mail) ExtractResult {
	text := m.Subject + "\n" + m.Text
	if m.Text == "" {
		text += "\n" + stripTags(m.HTML)
	}
	if g := codeRe.FindStringSubmatch(text); g != nil {
		return ExtractResult{Type: "auth_code", Result: g[1]}
	}
	if g := digitRe.FindStringSubmatch(text); g != nil {
		return ExtractResult{Type: "auth_code", Result: g[1]}
	}
	for _, l := range linkRe.FindAllString(text, -1) {
		ll := strings.ToLower(l)
		if strings.Contains(ll, "unsubscribe") {
			continue
		}
		for _, k := range []string{"verify", "confirm", "activate", "login", "signin", "reset", "token="} {
			if strings.Contains(ll, k) {
				return ExtractResult{Type: "auth_link", Result: l}
			}
		}
	}
	return ExtractResult{Type: "none"}
}

func stripTags(h string) string {
	var b strings.Builder
	in := false
	for _, r := range h {
		switch {
		case r == '<':
			in = true
		case r == '>':
			in = false
			b.WriteByte(' ')
		case !in:
			b.WriteRune(r)
		}
	}
	return b.String()
}

func (a *App) aiExtract(ctx context.Context, m *mailparse.Mail) *ExtractResult {
	if m == nil {
		return nil
	}
	content := "Subject: " + m.Subject + "\n\n" + m.Text
	if m.Text == "" {
		content += stripTags(m.HTML)
	}
	if len(content) > 12000 {
		content = content[:12000]
	}
	if a.cfg.AIExtractEndpoint != "" && a.cfg.AIExtractAPIKey != "" {
		body, _ := json.Marshal(map[string]any{
			"model": a.cfg.AIExtractModel, "temperature": 0,
			"response_format": map[string]string{"type": "json_object"},
			"messages":        []map[string]string{{"role": "system", "content": aiPrompt}, {"role": "user", "content": content}},
		})
		req, _ := http.NewRequestWithContext(ctx, "POST", strings.TrimSuffix(a.cfg.AIExtractEndpoint, "/")+"/chat/completions", bytes.NewReader(body))
		req.Header.Set("Authorization", "Bearer "+a.cfg.AIExtractAPIKey)
		req.Header.Set("Content-Type", "application/json")
		resp, err := (&http.Client{Timeout: 30 * time.Second}).Do(req)
		if err == nil {
			defer resp.Body.Close()
			var out struct {
				Choices []struct {
					Message struct{ Content string } `json:"message"`
				} `json:"choices"`
			}
			if json.NewDecoder(resp.Body).Decode(&out) == nil && len(out.Choices) > 0 {
				var r ExtractResult
				c := out.Choices[0].Message.Content
				if i, j := strings.Index(c, "{"), strings.LastIndex(c, "}"); i >= 0 && j > i {
					c = c[i : j+1]
				}
				if json.Unmarshal([]byte(c), &r) == nil && r.Type != "" {
					return &r
				}
			}
		} else {
			log.Printf("ai extract: %v", err)
		}
	}
	r := regexExtract(m)
	return &r
}

// OnMailStored runs post-storage processing: AI extraction, Telegram push,
// webhooks. Invoked asynchronously by the inbound pipeline.
func (a *App) OnMailStored(ctx context.Context, address string, mailID int64, raw string, parsed *mailparse.Mail) {
	a.notifyMailPublished(address, mailID)
	var metadata string
	s := a.aiSettings(ctx)
	if s.Enabled && (!s.EnableAllowList || contains(s.AllowList, address)) {
		if r := a.aiExtract(ctx, parsed); r != nil {
			if b, err := json.Marshal(map[string]any{"ai_extract": r}); err == nil {
				metadata = string(b)
				a.db.Exec(ctx, `UPDATE raw_mails SET metadata = ? WHERE id = ?`, metadata, mailID)
			}
		}
	}
	if a.tg != nil && a.tg.Enabled() {
		a.tg.PushMail(ctx, address, mailID, raw, parsed, metadata)
	}
	a.TriggerWebhook(ctx, address, mailID, raw, parsed, metadata)
}

func aiFromMetadata(metadata string) *ExtractResult {
	if metadata == "" {
		return nil
	}
	var m struct {
		AI *ExtractResult `json:"ai_extract"`
	}
	if json.Unmarshal([]byte(metadata), &m) != nil || m.AI == nil || m.AI.Type == "none" || m.AI.Result == "" {
		return nil
	}
	return m.AI
}
