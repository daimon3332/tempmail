package server

import (
	"context"
	"encoding/csv"
	"encoding/json"
	"fmt"
	"io"
	"net"
	"net/http"
	"os"
	"strconv"
	"strings"
	"sync"
	"time"

	"tempmail/internal/importer"
)

// ---- database backup / import ----

func (a *App) backupPath() string { return a.cfg.DBPath }

func (a *App) adminDBBackup(w http.ResponseWriter, r *http.Request) {
	// Copy SQLite via the online backup API to a temp file.
	tmp := a.backupPath() + ".bak"
	if err := a.db.BackupTo(tmp); err != nil {
		text(w, 500, "backup failed: "+err.Error())
		return
	}
	defer os.Remove(tmp)
	f, err := os.Open(tmp)
	if err != nil {
		text(w, 500, "backup failed")
		return
	}
	defer f.Close()
	w.Header().Set("Content-Disposition", "attachment; filename=tempmail-"+time.Now().UTC().Format("20060102")+".db")
	w.Header().Set("Content-Type", "application/octet-stream")
	io.Copy(w, f)
	a.audit(r.Context(), "db_backup", "database backup downloaded")
}

func (a *App) adminDBImport(w http.ResponseWriter, r *http.Request) {
	merge := r.URL.Query().Get("merge") == "1"
	r.Body = http.MaxBytesReader(w, r.Body, 512<<20)
	data, err := io.ReadAll(r.Body)
	if err != nil {
		text(w, 400, "read failed: "+err.Error())
		return
	}
	mode := importer.Primary
	if merge {
		mode = importer.Merge
	}
	st, err := importer.Run(r.Context(), a.db, string(data), mode)
	if err != nil {
		text(w, 400, "import failed: "+err.Error())
		return
	}
	a.audit(r.Context(), "db_import", fmt.Sprintf("imported %d statements (merge=%v)", st.Executed, merge))
	jsonResp(w, 200, map[string]any{"success": true, "executed": st.Executed, "skipped": st.Skipped})
}

// ---- send-mail usage ----

func (a *App) adminSendMailUsage(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	cfg := a.sendLimitConfig(ctx)
	resp := map[string]any{
		"daily_enabled": false, "monthly_enabled": false,
		"daily_limit": nil, "monthly_limit": nil,
		"daily_used": 0, "monthly_used": 0,
	}
	_ = cfg
	var rc runtimeConfig
	_ = rc
	if cfg != nil {
		resp["daily_enabled"] = cfg.DailyEnabled
		resp["monthly_enabled"] = cfg.MonthlyEnabled
		if cfg.DailyLimit != nil {
			resp["daily_limit"] = *cfg.DailyLimit
		}
		if cfg.MonthlyLimit != nil {
			resp["monthly_limit"] = *cfg.MonthlyLimit
		}
		resp["daily_used"] = a.settingInt(ctx, dailyKey())
		resp["monthly_used"] = a.settingInt(ctx, monthlyKey())
	}
	jsonResp(w, 200, resp)
}

func (a *App) adminSendMailUsageReset(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	a.db.Exec(ctx, `DELETE FROM settings WHERE key = ?`, dailyKey())
	a.db.Exec(ctx, `DELETE FROM settings WHERE key = ?`, monthlyKey())
	a.audit(ctx, "send_mail_usage", "reset daily/monthly counters")
	ok(w)
}

// ---- CSV import / export of addresses ----

func (a *App) adminAddressExport(w http.ResponseWriter, r *http.Request) {
	rows, err := a.db.Query(r.Context(), `SELECT id, name, created_at FROM address ORDER BY id`)
	if err != nil {
		fail(w, err)
		return
	}
	w.Header().Set("Content-Type", "text/csv")
	w.Header().Set("Content-Disposition", "attachment; filename=addresses.csv")
	w.Header().Set("Content-Type", "text/csv; charset=utf-8")
	cw := csv.NewWriter(w)
	cw.Write([]string{"id", "address", "created_at"})
	for _, row := range rows {
		cw.Write([]string{row.Str("id"), row.Str("name"), row.Str("created_at")})
	}
	cw.Flush()
}

func (a *App) adminAddressImport(w http.ResponseWriter, r *http.Request) {
	r.Body = http.MaxBytesReader(w, r.Body, 64<<20)
	cr := csv.NewReader(r.Body)
	cr.FieldsPerRecord = -1
	records, err := cr.ReadAll()
	if err != nil {
		text(w, 400, "csv parse failed: "+err.Error())
		return
	}
	created := 0
	skipped := 0
	for i, rec := range records {
		if i == 0 && len(rec) > 0 && strings.EqualFold(strings.TrimSpace(rec[0]), "address") {
			continue // header
		}
		addr := strings.TrimSpace(rec[0])
		if addr == "" {
			continue
		}
		if _, found, _ := a.db.ScanInt(r.Context(), `SELECT id FROM address WHERE name = ?`, addr); found {
			skipped++
			continue
		}
		name := addr
		domain := ""
		if j := strings.LastIndex(addr, "@"); j > 0 {
			name, domain = addr[:j], addr[j+1:]
		}
		if _, err := a.newAddress(r.Context(), newAddressOpts{name: name, domain: domain, allowDomains: a.cfg.Domains, sourceMeta: "csv"}); err != nil {
			skipped++
			continue
		}
		created++
	}
	a.audit(r.Context(), "address_import", fmt.Sprintf("imported %d addresses (%d skipped)", created, skipped))
	jsonResp(w, 200, map[string]any{"created": created, "skipped": skipped})
}

// ---- test mail ----

func (a *App) adminSendTestMail(w http.ResponseWriter, r *http.Request) {
	var req struct {
		To string `json:"to"`
	}
	readJSON(r, &req)
	if req.To == "" {
		text(w, 400, "to is required")
		return
	}
	raw := mailerBuild("Tempmail", "no-reply@"+a.cfg.Domains[0], req.To, "Test mail from Tempmail", "This is a test mail to verify the send path works. If you received this, sending is OK.")
	if err := a.mailer.Send("no-reply@"+a.cfg.Domains[0], req.To, raw); err != nil {
		text(w, 500, "send failed: "+err.Error())
		return
	}
	a.audit(r.Context(), "test_mail", req.To)
	jsonResp(w, 200, map[string]string{"status": "ok"})
}

// ---- domain status ----

func (a *App) adminDomainStatus(w http.ResponseWriter, r *http.Request) {
	type res struct {
		Name    string `json:"name"`
		Default bool   `json:"default"`
		OK      bool   `json:"mx_ok"`
	}
	out := []res{}
	var wg sync.WaitGroup
	var mu sync.Mutex
	for _, d := range a.cfg.Domains {
		d := d
		wg.Add(1)
		go func() {
			defer wg.Done()
			ok := true
			if _, err := net.LookupMX(d); err != nil {
				ok = false
			}
			mu.Lock()
			out = append(out, res{Name: d, Default: contains(a.cfg.DefaultDomains, d), OK: ok})
			mu.Unlock()
		}()
	}
	wg.Wait()
	jsonResp(w, 200, map[string]any{"domains": out})
}

var _ = context.Background
var _ = strconv.Itoa
var _ = json.Marshal
