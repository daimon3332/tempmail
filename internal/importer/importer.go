package importer

import (
	"context"
	"fmt"
	"regexp"
	"strings"

	"tempmail/internal/db"
)

// Mode controls how a wrangler `d1 export` dump is applied.
//
//	Primary: ids are preserved (first import into an empty database).
//	Merge:   ids are dropped and reassigned; users/address foreign keys are
//	         remapped so a second database can be folded in.
type Mode int

const (
	Primary Mode = iota
	Merge
)

type Stats struct {
	Executed int
	Skipped  int
	Tables   map[string]int
}

var (
	createTableRe = regexp.MustCompile(`(?i)^CREATE TABLE\s+(?:IF NOT EXISTS\s+)?`)
	createIndexRe = regexp.MustCompile(`(?i)^CREATE (UNIQUE )?INDEX\s+(?:IF NOT EXISTS\s+)?`)
	insertRe      = regexp.MustCompile(`(?is)^INSERT INTO\s+"?(\w+)"?\s*\((.*?)\)\s*VALUES\s*\((.*)\)\s*$`)
)

// SplitStatements splits SQL text on ';' outside single-quoted strings.
func SplitStatements(text string) []string {
	var out []string
	var b strings.Builder
	inStr := false
	for i := 0; i < len(text); i++ {
		c := text[i]
		b.WriteByte(c)
		if c == '\'' {
			inStr = !inStr
			continue
		}
		if c == ';' && !inStr {
			s := strings.TrimSpace(b.String())
			if s != ";" && s != "" {
				out = append(out, strings.TrimSuffix(s, ";"))
			}
			b.Reset()
		}
	}
	if s := strings.TrimSpace(b.String()); s != "" {
		out = append(out, s)
	}
	return out
}

// splitTop splits on commas outside quotes and outside parentheses; D1 dumps
// wrap text containing newlines in replace(replace('..','\r',char(13)),..).
func splitTop(s string) []string {
	var parts []string
	var b strings.Builder
	inStr := false
	depth := 0
	for i := 0; i < len(s); i++ {
		c := s[i]
		if c == '\'' {
			inStr = !inStr
		}
		if !inStr {
			switch c {
			case '(':
				depth++
			case ')':
				depth--
			case ',':
				if depth == 0 {
					parts = append(parts, strings.TrimSpace(b.String()))
					b.Reset()
					continue
				}
			}
		}
		b.WriteByte(c)
	}
	parts = append(parts, strings.TrimSpace(b.String()))
	return parts
}

func unquoteIdent(s string) string {
	return strings.Trim(strings.TrimSpace(s), `"`)
}

func unquoteLiteral(s string) string {
	s = strings.TrimSpace(s)
	if len(s) >= 2 && s[0] == '\'' && s[len(s)-1] == '\'' {
		return strings.ReplaceAll(s[1:len(s)-1], "''", "'")
	}
	return s
}

type insertStmt struct {
	table string
	cols  []string
	vals  []string
}

func parseInsert(stmt string) *insertStmt {
	m := insertRe.FindStringSubmatch(stmt)
	if m == nil {
		return nil
	}
	cols := splitTop(m[2])
	for i := range cols {
		cols[i] = unquoteIdent(cols[i])
	}
	vals := splitTop(m[3])
	if len(cols) != len(vals) {
		return nil
	}
	return &insertStmt{table: strings.ToLower(m[1]), cols: cols, vals: vals}
}

func (s *insertStmt) get(col string) (string, int) {
	for i, c := range s.cols {
		if c == col {
			return s.vals[i], i
		}
	}
	return "", -1
}

func (s *insertStmt) set(idx int, val string) { s.vals[idx] = val }

func (s *insertStmt) without(col string) *insertStmt {
	n := &insertStmt{table: s.table}
	for i, c := range s.cols {
		if c != col {
			n.cols = append(n.cols, c)
			n.vals = append(n.vals, s.vals[i])
		}
	}
	return n
}

func (s *insertStmt) sql(orIgnore bool) string {
	verb := "INSERT"
	if orIgnore {
		verb = "INSERT OR IGNORE"
	}
	return fmt.Sprintf(`%s INTO "%s" ("%s") VALUES (%s)`, verb, s.table, strings.Join(s.cols, `","`), strings.Join(s.vals, ","))
}

// Run applies the dump inside one transaction.
func Run(ctx context.Context, d *db.DB, dump string, mode Mode) (*Stats, error) {
	st := &Stats{Tables: map[string]int{}}
	tx, err := d.BeginTx(ctx, nil)
	if err != nil {
		return nil, err
	}
	defer tx.Rollback()

	userMap := map[string]int64{}
	addrMap := map[string]int64{}
	remap := func(s *insertStmt, col string, m map[string]int64) bool {
		v, idx := s.get(col)
		if idx < 0 {
			return true
		}
		nv, ok := m[strings.TrimSpace(v)]
		if !ok {
			return false
		}
		s.set(idx, fmt.Sprint(nv))
		return true
	}

	for _, stmt := range SplitStatements(dump) {
		upper := strings.ToUpper(stmt)
		switch {
		case strings.HasPrefix(upper, "PRAGMA"), strings.Contains(upper, "SQLITE_SEQUENCE"), strings.HasPrefix(upper, "BEGIN"), strings.HasPrefix(upper, "COMMIT"):
			st.Skipped++
			continue
		case createTableRe.MatchString(stmt):
			stmt = createTableRe.ReplaceAllString(stmt, "CREATE TABLE IF NOT EXISTS ")
		case createIndexRe.MatchString(stmt):
			stmt = createIndexRe.ReplaceAllStringFunc(stmt, func(s string) string {
				if strings.Contains(strings.ToUpper(s), "UNIQUE") {
					return "CREATE UNIQUE INDEX IF NOT EXISTS "
				}
				return "CREATE INDEX IF NOT EXISTS "
			})
		case strings.HasPrefix(upper, "INSERT"):
			ins := parseInsert(stmt)
			if ins == nil {
				return nil, fmt.Errorf("cannot parse insert: %.120s", stmt)
			}
			if mode == Merge {
				oldID, _ := ins.get("id")
				switch ins.table {
				case "users", "address", "raw_mails", "sendbox", "address_sender", "auto_reply_mails", "user_passkeys", "users_address", "user_roles":
					ins = ins.without("id")
				}
				switch ins.table {
				case "users_address":
					if !remap(ins, "user_id", userMap) || !remap(ins, "address_id", addrMap) {
						st.Skipped++
						continue
					}
				case "user_roles", "user_passkeys":
					if !remap(ins, "user_id", userMap) {
						st.Skipped++
						continue
					}
				}
				res, err := tx.ExecContext(ctx, ins.sql(true))
				if err != nil {
					return nil, fmt.Errorf("%s: %w", ins.table, err)
				}
				if n, _ := res.RowsAffected(); n == 0 {
					st.Skipped++
					if ins.table == "address" {
						// duplicate name: map to the existing row so bindings still resolve
						name, _ := ins.get("name")
						var id int64
						if tx.QueryRowContext(ctx, `SELECT id FROM address WHERE name = ?`, unquoteLiteral(name)).Scan(&id) == nil {
							addrMap[strings.TrimSpace(oldID)] = id
						}
					}
					continue
				}
				newID, _ := res.LastInsertId()
				if ins.table == "users" {
					userMap[strings.TrimSpace(oldID)] = newID
				}
				if ins.table == "address" {
					addrMap[strings.TrimSpace(oldID)] = newID
				}
				st.Executed++
				st.Tables[ins.table]++
				continue
			}
			stmt = ins.sql(true)
			st.Tables[ins.table]++
		}
		if _, err := tx.ExecContext(ctx, stmt); err != nil {
			return nil, fmt.Errorf("%.100s: %w", stmt, err)
		}
		st.Executed++
	}
	if _, err := tx.ExecContext(ctx, `DELETE FROM user_roles WHERE user_id NOT IN (SELECT id FROM users)`); err != nil {
		return nil, err
	}
	if _, err := tx.ExecContext(ctx, `DELETE FROM users_address WHERE user_id NOT IN (SELECT id FROM users) OR address_id NOT IN (SELECT id FROM address)`); err != nil {
		return nil, err
	}
	return st, tx.Commit()
}
