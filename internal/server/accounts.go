package server

import (
	"context"

	"tempmail/internal/config"
	"tempmail/internal/db"
)

// ensureSystemAccounts is idempotent and only provisions the built-in roles
// and admin account when they do not already exist.
func ensureSystemAccounts(ctx context.Context, cfg *config.Config, d *db.DB) error {
	roles := []struct {
		name, display string
		canSend       int
	}{
		{"admin", "Administrator", 1},
		{"user", "User", 0},
	}
	for _, r := range roles {
		if _, err := d.ExecContext(ctx, `INSERT INTO role_configs
			(role, name, domains, max_address_count, monthly_address_quota, can_custom_name, can_send_mail)
			VALUES (?, ?, '[]', -1, -1, 1, ?)
			ON CONFLICT(role) DO NOTHING`, r.name, r.display, r.canSend); err != nil {
			return err
		}
	}
	var count int64
	if err := d.QueryRowContext(ctx, `SELECT COUNT(*) FROM users WHERE username = 'admin' OR user_email = 'admin'`).Scan(&count); err != nil {
		return err
	}
	if count > 0 {
		var id int64
		if err := d.QueryRowContext(ctx, `SELECT id FROM users WHERE username = 'admin' OR user_email = 'admin' LIMIT 1`).Scan(&id); err == nil {
			if _, err := d.ExecContext(ctx, `INSERT INTO user_roles (user_id, role_text) VALUES (?, ?) ON CONFLICT(user_id) DO UPDATE SET role_text = excluded.role_text`, id, cfg.AdminUserRole); err != nil {
				return err
			}
		}
	}
	if count == 0 {
		password := "admin"
		if len(cfg.AdminPasswords) > 0 && cfg.AdminPasswords[0] != "" {
			password = cfg.AdminPasswords[0]
		}
		res, err := d.ExecContext(ctx, `INSERT INTO users (user_email, username, password, user_info) VALUES (?, ?, ?, ?)`,
			"admin", "admin", sha256Hex(password), `{"userEmail":"admin"}`)
		if err != nil {
			return err
		}
		id, err := res.LastInsertId()
		if err != nil {
			return err
		}
		if _, err := d.ExecContext(ctx, `INSERT INTO user_roles (user_id, role_text) VALUES (?, ?) ON CONFLICT(user_id) DO UPDATE SET role_text = excluded.role_text`, id, cfg.AdminUserRole); err != nil {
			return err
		}
	}
	return nil
}
