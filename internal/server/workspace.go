package server

import (
	"net/http"
	"strconv"
	"strings"
)

// workspaceMails exposes the same parsed mail shape to the authenticated
// user workspace. It deliberately keeps the legacy /api and /admin routes
// unchanged while giving the UI one consistent renderer data source.
func (a *App) workspaceMails(w http.ResponseWriter, r *http.Request) {
	q := r.URL.Query()
	where := []string{"1 = 1"}
	args := []any{}
	if u := userOf(r); u != nil {
		if a.isUserAdmin(r) {
			// Admins see every mailbox and message.
		} else {
			where = append(where, "EXISTS (SELECT 1 FROM users_address ua JOIN address own ON own.id = ua.address_id WHERE ua.user_id = ? AND own.name = rm.address)")
			args = append(args, claimInt(u, "user_id"))
		}
	}
	if address := strings.TrimSpace(q.Get("address")); address != "" {
		where = append(where, "rm.address = ?")
		args = append(args, address)
	}
	if domain := strings.TrimSpace(strings.TrimPrefix(q.Get("domain"), "@")); domain != "" {
		where = append(where, "lower(substr(rm.address, instr(rm.address, '@') + 1)) = lower(?)")
		args = append(args, domain)
	}
	if sender := strings.TrimSpace(q.Get("sender")); sender != "" {
		where = append(where, "lower(rm.source) LIKE lower(?)")
		args = append(args, "%"+sender+"%")
	}
	if query := strings.TrimSpace(q.Get("query")); query != "" {
		where = append(where, "lower(rm.raw) LIKE lower(?)")
		args = append(args, "%"+query+"%")
	}
	if q.Get("unread") == "1" || q.Get("unread") == "true" {
		where = append(where, "COALESCE(rm.is_unread, 0) = 1")
	}
	if q.Get("has_attachment") == "1" || q.Get("has_attachment") == "true" {
		where = append(where, "(lower(rm.raw) LIKE '%content-disposition: attachment%' OR lower(rm.raw) LIKE '%content-type: application/%')")
	}
	order := "rm.id desc"
	sortCols := map[string]string{"id": "rm.id", "created_at": "rm.created_at", "address": "rm.address", "source": "rm.source"}
	if col, ok := sortCols[q.Get("sort_by")]; ok {
		dir := "desc"
		if q.Get("sort_order") == "asc" || q.Get("sort_order") == "ascend" {
			dir = "asc"
		}
		order = col + " " + dir
	}
	limit, offset := atoi(q.Get("limit")), atoi(q.Get("offset"))
	if limit <= 0 || limit > 100 || offset < 0 || q.Get("offset") == "" {
		text(w, 400, "Invalid pagination")
		return
	}
	base := " FROM raw_mails rm WHERE " + strings.Join(where, " AND ")
	rows, err := a.db.Query(r.Context(), "SELECT rm.*"+base+" ORDER BY "+order+" LIMIT ? OFFSET ?", append(args, limit, offset)...)
	if err != nil {
		fail(w, err)
		return
	}
	out := make([]map[string]any, 0, len(rows))
	for _, row := range rows {
		out = append(out, a.parsedRow(row))
	}
	count, _ := a.db.Count(r.Context(), "SELECT count(*)"+base, args...)
	jsonResp(w, 200, map[string]any{"results": out, "count": count, "limit": limit, "offset": offset, "next_offset": strconv.FormatInt(offset+int64(len(out)), 10)})
}

func (a *App) workspaceParsedMail(w http.ResponseWriter, r *http.Request) {
	id := atoi(r.PathValue("id"))
	row, err := a.db.QueryOne(r.Context(), `SELECT * FROM raw_mails WHERE id = ?`, id)
	if err != nil {
		fail(w, err)
		return
	}
	if row == nil {
		jsonResp(w, 404, map[string]string{"error": "Mail not found"})
		return
	}
	if u := userOf(r); u != nil && !a.isUserAdmin(r) {
		ok, _ := a.db.Count(r.Context(), `SELECT count(*) FROM users_address ua JOIN address own ON own.id = ua.address_id WHERE ua.user_id = ? AND own.name = ?`, claimInt(u, "user_id"), row.Str("address"))
		if ok == 0 {
			text(w, 404, "Mail not found")
			return
		}
	}
	resolveRaw(row)
	raw := row.Str("raw")
	out := a.parsedRow(row)
	out["raw"] = raw
	jsonResp(w, 200, out)
}

func (a *App) workspaceOwnsMail(r *http.Request, id int64) bool {
	if a.isUserAdmin(r) || userOf(r) == nil {
		return true
	}
	n, _ := a.db.Count(r.Context(), `SELECT count(*) FROM raw_mails rm JOIN address a ON a.name=rm.address JOIN users_address ua ON ua.address_id=a.id WHERE rm.id=? AND ua.user_id=?`, id, claimInt(userOf(r), "user_id"))
	return n > 0
}

func (a *App) workspaceMarkRead(w http.ResponseWriter, r *http.Request) {
	id := atoi(r.PathValue("id"))
	if !a.workspaceOwnsMail(r, id) {
		text(w, 404, "Mail not found")
		return
	}
	var req struct {
		IsUnread bool `json:"isUnread"`
	}
	readJSON(r, &req)
	v := 0
	if req.IsUnread {
		v = 1
	}
	_, err := a.db.Exec(r.Context(), `UPDATE raw_mails SET is_unread=? WHERE id=?`, v, id)
	jsonResp(w, 200, map[string]bool{"success": err == nil})
}

func (a *App) workspaceDeleteMail(w http.ResponseWriter, r *http.Request) {
	id := atoi(r.PathValue("id"))
	if !a.workspaceOwnsMail(r, id) {
		text(w, 404, "Mail not found")
		return
	}
	_, err := a.db.Exec(r.Context(), `DELETE FROM raw_mails WHERE id=?`, id)
	jsonResp(w, 200, map[string]bool{"success": err == nil})
}

func (a *App) isUserAdmin(r *http.Request) bool {
	u := userOf(r)
	if u == nil {
		return false
	}
	role, _ := a.roles.UserRole(r.Context(), claimInt(u, "user_id"))
	return role != nil && role.Role == a.cfg.AdminUserRole
}
