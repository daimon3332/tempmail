package server

import (
	"net/http"
	"strings"
)

// s3Enabled reports whether the S3 backend is configured (mirrors isS3Enabled).
func (a *App) s3Enabled() bool { return a.cfg.S3Enabled() }

func (a *App) apiAttachmentList(w http.ResponseWriter, r *http.Request) {
	if !a.s3Enabled() {
		text(w, 400, "S3 is not enabled")
		return
	}
	address, _ := addressOf(r)
	keys, err := a.s3.List(r.Context(), address)
	if err != nil {
		text(w, 500, "Failed to list attachments")
		return
	}
	out := make([]map[string]string, 0, len(keys))
	for _, k := range keys {
		out = append(out, map[string]string{"key": k})
	}
	jsonResp(w, 200, map[string]any{"results": out})
}

func (a *App) apiAttachmentGetURL(w http.ResponseWriter, r *http.Request) {
	if !a.s3Enabled() {
		text(w, 400, "S3 is not enabled")
		return
	}
	var req struct {
		Key string `json:"key"`
	}
	readJSON(r, &req)
	if req.Key == "" {
		text(w, 400, "key is required")
		return
	}
	address, _ := addressOf(r)
	url, err := a.s3.GetURL(r.Context(), address, req.Key)
	if err != nil {
		text(w, 500, "Failed to sign GET url")
		return
	}
	jsonResp(w, 200, map[string]string{"url": url})
}

func (a *App) apiAttachmentPutURL(w http.ResponseWriter, r *http.Request) {
	if !a.s3Enabled() {
		text(w, 400, "S3 is not enabled")
		return
	}
	var req struct {
		Key string `json:"key"`
	}
	readJSON(r, &req)
	if req.Key == "" {
		text(w, 400, "key is required")
		return
	}
	address, _ := addressOf(r)
	url, err := a.s3.PutURL(r.Context(), address, req.Key)
	if err != nil {
		text(w, 500, "Failed to sign PUT url")
		return
	}
	jsonResp(w, 200, map[string]string{"url": url})
}

func (a *App) apiAttachmentDelete(w http.ResponseWriter, r *http.Request) {
	if !a.s3Enabled() {
		text(w, 400, "S3 is not enabled")
		return
	}
	var req struct {
		Key string `json:"key"`
	}
	readJSON(r, &req)
	if req.Key == "" {
		text(w, 400, "key is required")
		return
	}
	address, _ := addressOf(r)
	if err := a.s3.Delete(r.Context(), address, req.Key); err != nil {
		text(w, 500, "Failed to delete attachment")
		return
	}
	ok(w)
}

func (a *App) s3CleanPrefix(prefix string) string {
	return strings.TrimPrefix(prefix, "/")
}
