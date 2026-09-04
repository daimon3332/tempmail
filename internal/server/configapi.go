package server

import (
	"net/http"
)

// adminGetRuntimeConfig returns the effective runtime configuration (env +
// overrides) for the config page to populate.
func (a *App) adminGetRuntimeConfig(w http.ResponseWriter, r *http.Request) {
	jsonResp(w, 200, a.effective(r.Context()))
}

// adminSaveRuntimeConfig persists runtime overrides; takes effect immediately.
func (a *App) adminSaveRuntimeConfig(w http.ResponseWriter, r *http.Request) {
	var rc runtimeConfig
	if err := readJSON(r, &rc); err != nil {
		text(w, 400, "Invalid input")
		return
	}
	if err := a.saveRuntime(r.Context(), rc); err != nil {
		fail(w, err)
		return
	}
	ok(w)
}

// adminSystemStatus reports the health of each channel (SMTP, ingest, API key).
func (a *App) adminSystemStatus(w http.ResponseWriter, r *http.Request) {
	jsonResp(w, 200, map[string]any{
		"smtp":        a.tcpOpen(r.Context(), a.cfg.SMTPAddr),
		"ingestToken": a.cfg.IngestToken != "",
		"apiKey":      a.effective(r.Context()).APIKey != "",
		"aiEnabled":   a.effective(r.Context()).AIEnabled,
		"telegram":    a.cfg.TelegramBotToken != "",
		"domains":     a.cfg.Domains,
	})
}
