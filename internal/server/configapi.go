package server

import (
	"net/http"
	"net/url"
	"strings"
)

// adminGetRuntimeConfig returns the effective runtime configuration (env +
// overrides) for the config page to populate.
func (a *App) adminGetRuntimeConfig(w http.ResponseWriter, r *http.Request) {
	rc := a.effective(r.Context())
	rc.Environment = readEnvValues(a.cfg.EnvSyncPath)
	// The admin-only settings surface may display site/API/AI secrets. The
	// administrator password remains write-only by project policy.
	rc.AdminPassword = ""
	jsonResp(w, 200, rc)
}

// adminSaveRuntimeConfig persists runtime overrides; takes effect immediately.
func (a *App) adminSaveRuntimeConfig(w http.ResponseWriter, r *http.Request) {
	var rc runtimeConfig
	if err := readJSON(r, &rc); err != nil {
		text(w, 400, "Invalid input")
		return
	}
	if rc.MinAddressLen < 1 || rc.MaxAddressLen < rc.MinAddressLen || rc.MaxAddressLen > 256 {
		text(w, 400, "Invalid address length range")
		return
	}
	if rc.RateLimitPerMinute < 0 || rc.RateLimitPerMinute > 1000000 {
		text(w, 400, "Invalid rate limit")
		return
	}
	if len(rc.Domains) == 0 {
		text(w, 400, "At least one domain is required")
		return
	}
	seen := map[string]bool{}
	for i, d := range rc.Domains {
		d = strings.ToLower(strings.TrimSpace(d))
		if findAllowedDomain(d, []string{d}, false) == "" || seen[d] {
			text(w, 400, "Invalid or duplicate domain")
			return
		}
		rc.Domains[i] = d
		seen[d] = true
	}
	for i, d := range rc.DefaultDomains {
		d = strings.ToLower(strings.TrimSpace(d))
		if !seen[d] {
			text(w, 400, "Default domain must be in domains")
			return
		}
		rc.DefaultDomains[i] = d
	}
	if len(rc.DefaultDomains) == 0 {
		rc.DefaultDomains = append([]string{}, rc.Domains...)
	}
	rc.RateLimitPerMinuteSet = true
	if rc.AIEndpoint != "" {
		u, err := url.Parse(rc.AIEndpoint)
		if err != nil || (u.Scheme != "http" && u.Scheme != "https") || u.Host == "" {
			text(w, 400, "Invalid AI endpoint")
			return
		}
	}
	if rc.AIModel == "" {
		rc.AIModel = "gpt-4o-mini"
	}
	if rc.AIAllowList == nil {
		rc.AIAllowList = []string{}
	}
	if rc.Environment == nil {
		rc.Environment = readEnvValues(a.cfg.EnvSyncPath)
	}
	if rc.Prefix != "" {
		rc.Prefix = strings.TrimSpace(rc.Prefix)
	}
	rc.PrefixSet = true
	rc.RandomSubdomainDomainsSet = true
	rc.RandomSubdomainLengthSet = true
	// Secret fields are intentionally omitted by GET; preserve them unless a
	// new value is explicitly supplied.
	current := a.effective(r.Context())
	if rc.SitePassword == "" {
		rc.SitePassword = current.SitePassword
	}
	if rc.AdminPassword == "" {
		rc.AdminPassword = current.AdminPassword
	}
	if rc.APIKey == "" {
		rc.APIKey = current.APIKey
	}
	if rc.AIAPIKey == "" {
		rc.AIAPIKey = current.AIAPIKey
	}
	if err := a.saveRuntime(r.Context(), rc); err != nil {
		fail(w, err)
		return
	}
	envStatus := "synced"
	envError := ""
	if err := syncRuntimeEnv(a.cfg.EnvSyncPath, rc); err != nil {
		envStatus, envError = "pending", err.Error()
	}
	restartRequired := []string{"DOMAINS", "DEFAULT_DOMAINS", "RANDOM_SUBDOMAIN_DOMAINS", "SMTP_ADDR", "SMTP_CONFIG", "S3_ENDPOINT", "TELEGRAM_BOT_TOKEN", "JWT_SECRET"}
	jsonResp(w, 200, map[string]any{"success": true, "env_sync": envStatus, "env_error": envError, "restart_required": restartRequired})
}

// adminSystemStatus reports the health of each channel (SMTP, ingest, API key).
func (a *App) adminSystemStatus(w http.ResponseWriter, r *http.Request) {
	jsonResp(w, 200, map[string]any{
		"smtp":        a.tcpOpen(r.Context(), a.cfg.SMTPAddr),
		"ingestToken": a.cfg.IngestToken != "",
		"apiKey":      a.effective(r.Context()).APIKey != "",
		"aiEnabled":   a.effective(r.Context()).AIEnabled,
		"telegram":    a.cfg.TelegramBotToken != "",
		"domains":     a.effective(r.Context()).Domains,
	})
}
