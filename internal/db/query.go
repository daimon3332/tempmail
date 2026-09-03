package db

import (
	"context"
	"database/sql"
	"time"
)

// Row is a generic result row; values mirror what D1 returns to the frontend.
type Row map[string]any

func normalize(v any) any {
	switch x := v.(type) {
	case []byte:
		return string(x)
	case time.Time:
		return x.UTC().Format("2006-01-02 15:04:05")
	default:
		return v
	}
}

func (d *DB) Query(ctx context.Context, query string, args ...any) ([]Row, error) {
	rows, err := d.QueryContext(ctx, query, args...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	cols, err := rows.Columns()
	if err != nil {
		return nil, err
	}
	out := []Row{}
	for rows.Next() {
		vals := make([]any, len(cols))
		ptrs := make([]any, len(cols))
		for i := range vals {
			ptrs[i] = &vals[i]
		}
		if err := rows.Scan(ptrs...); err != nil {
			return nil, err
		}
		r := make(Row, len(cols))
		for i, c := range cols {
			r[c] = normalize(vals[i])
		}
		out = append(out, r)
	}
	return out, rows.Err()
}

func (d *DB) QueryOne(ctx context.Context, query string, args ...any) (Row, error) {
	rows, err := d.Query(ctx, query, args...)
	if err != nil || len(rows) == 0 {
		return nil, err
	}
	return rows[0], nil
}

func (d *DB) Count(ctx context.Context, query string, args ...any) (int64, error) {
	var n int64
	err := d.QueryRowContext(ctx, query, args...).Scan(&n)
	return n, err
}

func (d *DB) ScanString(ctx context.Context, query string, args ...any) (string, bool, error) {
	var v sql.NullString
	err := d.QueryRowContext(ctx, query, args...).Scan(&v)
	if err == sql.ErrNoRows {
		return "", false, nil
	}
	return v.String, err == nil && v.Valid, err
}

func (d *DB) ScanInt(ctx context.Context, query string, args ...any) (int64, bool, error) {
	var v sql.NullInt64
	err := d.QueryRowContext(ctx, query, args...).Scan(&v)
	if err == sql.ErrNoRows {
		return 0, false, nil
	}
	return v.Int64, err == nil && v.Valid, err
}

func (d *DB) Exec(ctx context.Context, query string, args ...any) (int64, error) {
	res, err := d.ExecContext(ctx, query, args...)
	if err != nil {
		return 0, err
	}
	n, _ := res.RowsAffected()
	return n, nil
}

func (r Row) Str(k string) string {
	if v, ok := r[k].(string); ok {
		return v
	}
	return ""
}

func (r Row) Int(k string) int64 {
	switch v := r[k].(type) {
	case int64:
		return v
	case int:
		return int64(v)
	case float64:
		return int64(v)
	}
	return 0
}
