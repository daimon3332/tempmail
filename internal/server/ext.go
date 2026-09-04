package server

import (
	"context"
	"encoding/base64"
	"net"
	"net/http"
	"strings"
	"time"
)

// Endpoints beyond the upstream API, designed for automation clients
// (OutlookRegister etc.). All are admin-authenticated unless noted.
func (a *App) extRoutes() {
	m := a.mux
	m.HandleFunc("GET /health", a.extHealth)
	m.HandleFunc("GET /admin/health", a.extHealth)
	m.HandleFunc("POST /admin/ensure_address", a.extEnsureAddress)
	m.HandleFunc("GET /admin/address/lookup", a.extLookupAddress)
	m.HandleFunc("GET /admin/address/mails", a.extAddressMailsByName)
	m.HandleFunc("POST /admin/address/access", a.extAddressAccess)
	m.HandleFunc("GET /admin/address/{id}", a.extAddressDetail)
	m.HandleFunc("GET /admin/address/{id}/mails", a.extAddressMails)
	m.HandleFunc("POST /admin/address/{id}/archive", a.extArchive)
	m.HandleFunc("POST /admin/address/{id}/restore", a.extRestore)
	m.HandleFunc("DELETE /admin/address/{id}", a.adminDeleteAddress)
	m.HandleFunc("POST /admin/address/{id}/recreate", a.extRecreate)
	m.HandleFunc("GET /admin/stats", a.extStats)
	m.HandleFunc("GET /admin/domains", a.extDomains)
	m.HandleFunc("POST /external/ingest", a.extIngest)
}

func (a *App) extHealth(w http.ResponseWriter, r *http.Request) {
	dbStatus, smtpStatus := "ok", "ok"
	if err := a.db.PingContext(r.Context()); err != nil {
		dbStatus = "error: " + err.Error()
	}
	addr := a.cfg.SMTPAddr
	if strings.HasPrefix(addr, ":") {
		addr = "127.0.0.1" + addr
	}
	if c, err := net.DialTimeout("tcp", addr, 2*time.Second); err != nil {
		smtpStatus = "error: " + err.Error()
	} else {
		c.Close()
	}
	status := "ok"
	if dbStatus != "ok" || smtpStatus != "ok" {
		status = "degraded"
	}
	code := 200
	if dbStatus != "ok" {
		code = 503
	}
	jsonResp(w, code, map[string]any{"status": status, "database": dbStatus, "mail_receiver": smtpStatus, "version": "v1.12.0"})
}

func (a *App) addressSummary(ctx context.Context, where string, arg any) map[string]any {
	row, _ := a.db.QueryOne(ctx, `SELECT a.id, a.name, a.created_at, a.updated_at, a.source_meta,
		(SELECT COUNT(*) FROM raw_mails WHERE address = a.name) AS mail_count,
		(SELECT MAX(created_at) FROM raw_mails WHERE address = a.name) AS last_mail_at
		FROM address a WHERE `+where, arg)
	if row == nil {
		return nil
	}
	name := row.Str("name")
	archived := strings.HasPrefix(row.Str("source_meta"), "archived:")
	return map[string]any{
		"id": row.Int("id"), "address": name, "domain": mailDomain(name), "address_id": row.Int("id"),
		"mail_count": row.Int("mail_count"), "created_at": row["created_at"], "updated_at": row["updated_at"],
		"last_mail_at": row["last_mail_at"], "archived": archived,
	}
}

func (a *App) extEnsureAddress(w http.ResponseWriter, r *http.Request) {
	var req struct {
		Name   string `json:"name"`
		Domain string `json:"domain"`
	}
	readJSON(r, &req)
	req.Name = strings.ToLower(strings.TrimSpace(req.Name))
	req.Domain = strings.ToLower(strings.TrimSpace(req.Domain))
	if req.Name == "" {
		text(w, 400, "Required field missing")
		return
	}
	if req.Domain == "" {
		req.Domain = a.cfg.Domains[0]
	}
	ctx := r.Context()
	address := req.Name
	if !strings.Contains(req.Name, "@") {
		address = req.Name + "@" + req.Domain
	}
	if s := a.addressSummary(ctx, "a.name = ?", address); s != nil {
		token, _ := a.jwt.AddressToken(address, s["id"].(int64))
		jsonResp(w, 200, map[string]any{"address": address, "address_id": s["id"], "jwt": token, "reused": true})
		return
	}
	res, err := a.newAddress(ctx, newAddressOpts{name: req.Name, domain: req.Domain, allowDomains: a.cfg.Domains, sourceMeta: "admin"})
	if err != nil {
		if s := a.addressSummary(ctx, "a.name = ?", address); s != nil {
			token, _ := a.jwt.AddressToken(address, s["id"].(int64))
			jsonResp(w, 200, map[string]any{"address": address, "address_id": s["id"], "jwt": token, "reused": true})
			return
		}
		text(w, 400, "Failed to create address: "+err.Error())
		return
	}
	jsonResp(w, 200, map[string]any{"address": res.Address, "address_id": res.AddressID, "jwt": res.JWT, "reused": false})
}

func (a *App) extLookupAddress(w http.ResponseWriter, r *http.Request) {
	addr := strings.TrimSpace(r.URL.Query().Get("address"))
	if addr == "" {
		text(w, 400, "address is required")
		return
	}
	s := a.addressSummary(r.Context(), "a.name = ?", addr)
	if s == nil {
		jsonResp(w, 200, map[string]any{"found": false})
		return
	}
	s["found"] = true
	jsonResp(w, 200, s)
}

func (a *App) extAddressDetail(w http.ResponseWriter, r *http.Request) {
	s := a.addressSummary(r.Context(), "a.id = ?", atoi(r.PathValue("id")))
	if s == nil {
		text(w, 404, "Address not found")
		return
	}
	jsonResp(w, 200, s)
}

func (a *App) extAddressAccess(w http.ResponseWriter, r *http.Request) {
	var req struct {
		Address   string `json:"address"`
		AddressID int64  `json:"address_id"`
	}
	readJSON(r, &req)
	var s map[string]any
	if req.AddressID > 0 {
		s = a.addressSummary(r.Context(), "a.id = ?", req.AddressID)
	} else {
		s = a.addressSummary(r.Context(), "a.name = ?", strings.TrimSpace(req.Address))
	}
	if s == nil {
		text(w, 404, "Address not found")
		return
	}
	token, _ := a.jwt.AddressToken(s["address"].(string), s["id"].(int64))
	jsonResp(w, 200, map[string]any{"address": s["address"], "address_id": s["id"], "jwt": token, "expires_in": nil})
}

func (a *App) mailItems(ctx context.Context, address string, limit, offset, afterID int64, after string) []map[string]any {
	q := `SELECT id, message_id, source, address, raw, raw_blob, is_unread, created_at FROM raw_mails WHERE address = ?`
	args := []any{address}
	if afterID > 0 {
		q += " AND id > ?"
		args = append(args, afterID)
	}
	if after != "" {
		q += " AND created_at > ?"
		args = append(args, after)
	}
	q += " ORDER BY id DESC LIMIT ? OFFSET ?"
	args = append(args, limit, offset)
	rows, _ := a.db.Query(ctx, q, args...)
	out := make([]map[string]any, 0, len(rows))
	for _, row := range rows {
		resolveRaw(row)
		item := map[string]any{"id": row.Int("id"), "message_id": row["message_id"], "source": row["source"], "address": address,
			"is_unread": row["is_unread"], "received_at": row["created_at"], "created_at": row["created_at"],
			"subject": "", "text": "", "html": "", "sender": ""}
		if m := a.parseRawFromRow(row); m != nil {
			item["subject"], item["text"], item["html"], item["sender"] = m.Subject, m.Text, m.HTML, strings.TrimSpace(m.Sender)
		}
		out = append(out, item)
	}
	return out
}

func parseListParams(r *http.Request) (limit, offset, afterID int64, after string) {
	q := r.URL.Query()
	limit, offset, afterID = atoi(q.Get("limit")), atoi(q.Get("offset")), atoi(q.Get("after_id"))
	if limit <= 0 || limit > 100 {
		limit = 20
	}
	if offset < 0 {
		offset = 0
	}
	if t := q.Get("after"); t != "" {
		if ts, err := time.Parse(time.RFC3339, t); err == nil {
			after = ts.UTC().Format("2006-01-02 15:04:05")
		}
	}
	return
}

func (a *App) extAddressMails(w http.ResponseWriter, r *http.Request) {
	s := a.addressSummary(r.Context(), "a.id = ?", atoi(r.PathValue("id")))
	if s == nil {
		text(w, 404, "Address not found")
		return
	}
	limit, offset, afterID, after := parseListParams(r)
	jsonResp(w, 200, map[string]any{"address": s["address"], "address_id": s["id"], "count": s["mail_count"],
		"items": a.mailItems(r.Context(), s["address"].(string), limit, offset, afterID, after)})
}

func (a *App) extAddressMailsByName(w http.ResponseWriter, r *http.Request) {
	addr := strings.TrimSpace(r.URL.Query().Get("address"))
	if addr == "" {
		text(w, 400, "address is required")
		return
	}
	limit, offset, afterID, after := parseListParams(r)
	count, _ := a.db.Count(r.Context(), `SELECT COUNT(*) FROM raw_mails WHERE address = ?`, addr)
	jsonResp(w, 200, map[string]any{"address": addr, "count": count, "items": a.mailItems(r.Context(), addr, limit, offset, afterID, after)})
}

func (a *App) setArchived(w http.ResponseWriter, r *http.Request, archive bool) {
	id := atoi(r.PathValue("id"))
	ctx := r.Context()
	meta, found, _ := a.db.ScanString(ctx, `SELECT source_meta FROM address WHERE id = ?`, id)
	if !found {
		text(w, 404, "Address not found")
		return
	}
	if archive && !strings.HasPrefix(meta, "archived:") {
		meta = "archived:" + meta
	} else if !archive {
		meta = strings.TrimPrefix(meta, "archived:")
	}
	var v any
	if meta != "" {
		v = meta
	}
	a.db.Exec(ctx, `UPDATE address SET source_meta = ?, updated_at = datetime('now') WHERE id = ?`, v, id)
	jsonResp(w, 200, map[string]any{"success": true, "archived": archive})
}

func (a *App) extArchive(w http.ResponseWriter, r *http.Request) { a.setArchived(w, r, true) }
func (a *App) extRestore(w http.ResponseWriter, r *http.Request) { a.setArchived(w, r, false) }

// extRecreate deletes the address and all its data, then creates it again
// with a new id and JWT. Intended for manual use only.
func (a *App) extRecreate(w http.ResponseWriter, r *http.Request) {
	id := atoi(r.PathValue("id"))
	ctx := r.Context()
	name, found, _ := a.db.ScanString(ctx, `SELECT name FROM address WHERE id = ?`, id)
	if !found {
		text(w, 404, "Address not found")
		return
	}
	if err := a.deleteAddressesWhere(ctx, `id = ?`, id); err != nil {
		fail(w, err)
		return
	}
	if _, err := a.db.Exec(ctx, `INSERT INTO address(name, source_meta) VALUES(?, 'admin')`, name); err != nil {
		fail(w, err)
		return
	}
	newID, _, _ := a.db.ScanInt(ctx, `SELECT id FROM address WHERE name = ?`, name)
	token, _ := a.jwt.AddressToken(name, newID)
	jsonResp(w, 200, map[string]any{"address": name, "address_id": newID, "jwt": token, "old_address_id": id})
}

func (a *App) extStats(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	count := func(q string) int64 { n, _ := a.db.Count(ctx, q); return n }
	jsonResp(w, 200, map[string]any{
		"address_count":          count(`SELECT count(*) FROM address`),
		"active_address_count":   count(`SELECT count(*) FROM address WHERE updated_at > datetime('now', '-30 day')`),
		"archived_address_count": count(`SELECT count(*) FROM address WHERE source_meta LIKE 'archived:%'`),
		"mail_count":             count(`SELECT count(*) FROM raw_mails`),
		"today_mail_count":       count(`SELECT count(*) FROM raw_mails WHERE created_at >= date('now')`),
		"unread_mail_count":      count(`SELECT count(*) FROM raw_mails WHERE is_unread = 1`),
		"user_count":             count(`SELECT count(*) FROM users`),
		"sendbox_count":          count(`SELECT count(*) FROM sendbox`),
	})
}

func (a *App) extDomains(w http.ResponseWriter, r *http.Request) {
	out := make([]map[string]any, 0, len(a.cfg.Domains))
	for _, d := range a.cfg.Domains {
		out = append(out, map[string]any{"name": d, "enabled": true, "default": contains(a.cfg.DefaultDomains, d),
			"random_subdomain": a.allowRandomSubdomain(r.Context(), d)})
	}
	jsonResp(w, 200, map[string]any{"domains": out})
}

// extIngest accepts a raw message from a trusted relay (Cloudflare Email
// Routing worker). Auth: x-ingest-token, or the admin password.
func (a *App) extIngest(w http.ResponseWriter, r *http.Request) {
	tok := r.Header.Get("x-ingest-token")
	if !((a.cfg.IngestToken != "" && tok == a.cfg.IngestToken) || contains(a.cfg.AdminPasswords, r.Header.Get("x-admin-auth"))) {
		text(w, 401, "Invalid ingest token")
		return
	}
	if a.ingest == nil {
		text(w, 503, "Ingest not available")
		return
	}
	var req struct {
		From      string `json:"from"`
		To        string `json:"to"`
		Raw       string `json:"raw"`
		RawBase64 string `json:"raw_base64"`
	}
	if err := readJSON(r, &req); err != nil {
		text(w, 400, "Invalid JSON")
		return
	}
	raw := []byte(req.Raw)
	if req.RawBase64 != "" {
		b, err := base64.StdEncoding.DecodeString(req.RawBase64)
		if err != nil {
			text(w, 400, "Invalid raw_base64")
			return
		}
		raw = b
	}
	if req.To == "" || len(raw) == 0 {
		text(w, 400, "to and raw are required")
		return
	}
	if int64(len(raw)) > a.cfg.MaxMessageBytes {
		text(w, 413, "Message too large")
		return
	}
	id, err := a.ingest(r.Context(), req.From, req.To, raw)
	if err != nil {
		text(w, 400, "Rejected: "+err.Error())
		return
	}
	jsonResp(w, 200, map[string]any{"success": true, "id": id})
}
