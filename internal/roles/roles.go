package roles

import (
	"context"
	"encoding/json"
	"errors"
	"strings"

	"tempmail/internal/config"
	"tempmail/internal/db"
)

// Role is the effective role definition. Env-defined roles (USER_ROLES) and
// DB-defined roles (role_configs) are merged; DB values override quotas.
type Role struct {
	Role                string   `json:"role"`
	Name                string   `json:"name,omitempty"`
	Domains             []string `json:"domains"`
	Prefix              *string  `json:"prefix,omitempty"`
	MaxAddressCount     int      `json:"max_address_count"`     // -1 unlimited, 0 use global user_settings
	MonthlyAddressQuota int      `json:"monthly_address_quota"` // -1 unlimited
	CanCustomName       bool     `json:"can_custom_name"`
	CanSendMail         bool     `json:"can_send_mail"`
	Source              string   `json:"source"` // env | db
}

type Store struct {
	cfg *config.Config
	db  *db.DB
}

func New(cfg *config.Config, d *db.DB) *Store { return &Store{cfg: cfg, db: d} }

func (s *Store) List(ctx context.Context) ([]Role, error) {
	out := []Role{}
	index := map[string]int{}
	for _, r := range s.cfg.UserRoles {
		index[r.Role] = len(out)
		out = append(out, Role{Role: r.Role, Domains: r.Domains, Prefix: r.Prefix,
			MaxAddressCount: 0, MonthlyAddressQuota: -1, CanCustomName: true, CanSendMail: true, Source: "env"})
	}
	rows, err := s.db.Query(ctx, `SELECT * FROM role_configs ORDER BY created_at`)
	if err != nil {
		return nil, err
	}
	for _, row := range rows {
		r := Role{
			Role:                row.Str("role"),
			Name:                row.Str("name"),
			MaxAddressCount:     int(row.Int("max_address_count")),
			MonthlyAddressQuota: int(row.Int("monthly_address_quota")),
			CanCustomName:       row.Int("can_custom_name") == 1,
			CanSendMail:         row.Int("can_send_mail") == 1,
			Source:              "db",
		}
		if d := row.Str("domains"); d != "" {
			_ = json.Unmarshal([]byte(d), &r.Domains)
		}
		if p, isStr := row["prefix"].(string); isStr {
			r.Prefix = &p
		}
		if r.Domains == nil {
			r.Domains = []string{}
		}
		if i, exists := index[r.Role]; exists {
			if len(r.Domains) == 0 {
				r.Domains = out[i].Domains
			}
			if r.Prefix == nil {
				r.Prefix = out[i].Prefix
			}
			out[i] = r
			continue
		}
		index[r.Role] = len(out)
		out = append(out, r)
	}
	return out, nil
}

func (s *Store) Get(ctx context.Context, name string) (*Role, error) {
	if name == "" {
		return nil, nil
	}
	list, err := s.List(ctx)
	if err != nil {
		return nil, err
	}
	for i := range list {
		if list[i].Role == name {
			return &list[i], nil
		}
	}
	return nil, nil
}

func (s *Store) Exists(ctx context.Context, name string) bool {
	r, _ := s.Get(ctx, name)
	return r != nil
}

func (s *Store) Save(ctx context.Context, r Role) error {
	r.Role = strings.TrimSpace(r.Role)
	if r.Role == "" {
		return errors.New("role is required")
	}
	domains, _ := json.Marshal(normalize(r.Domains))
	var prefix any
	if r.Prefix != nil {
		prefix = strings.ToLower(strings.TrimSpace(*r.Prefix))
	}
	b := func(v bool) int {
		if v {
			return 1
		}
		return 0
	}
	_, err := s.db.Exec(ctx, `INSERT INTO role_configs
		(role, name, domains, prefix, max_address_count, monthly_address_quota, can_custom_name, can_send_mail)
		VALUES (?, ?, ?, ?, ?, ?, ?, ?)
		ON CONFLICT(role) DO UPDATE SET name=excluded.name, domains=excluded.domains, prefix=excluded.prefix,
		max_address_count=excluded.max_address_count, monthly_address_quota=excluded.monthly_address_quota,
		can_custom_name=excluded.can_custom_name, can_send_mail=excluded.can_send_mail, updated_at=datetime('now')`,
		r.Role, r.Name, string(domains), prefix, r.MaxAddressCount, r.MonthlyAddressQuota, b(r.CanCustomName), b(r.CanSendMail))
	return err
}

func (s *Store) Delete(ctx context.Context, name string) error {
	_, err := s.db.Exec(ctx, `DELETE FROM role_configs WHERE role = ?`, name)
	return err
}

// UserRole returns the role assigned to a user (nil when none / unknown).
func (s *Store) UserRole(ctx context.Context, userID int64) (*Role, error) {
	name, found, err := s.db.ScanString(ctx, `SELECT role_text FROM user_roles WHERE user_id = ?`, userID)
	if err != nil || !found {
		return nil, err
	}
	return s.Get(ctx, name)
}

type UserSettings struct {
	Enable                bool     `json:"enable"`
	EnableMailVerify      bool     `json:"enableMailVerify"`
	VerifyMailSender      string   `json:"verifyMailSender"`
	EnableMailAllowList   bool     `json:"enableMailAllowList"`
	MailAllowList         []string `json:"mailAllowList"`
	MaxAddressCount       int      `json:"maxAddressCount"`
	EnableEmailCheckRegex bool     `json:"enableEmailCheckRegex"`
	EmailCheckRegex       string   `json:"emailCheckRegex"`
}

func (s *Store) UserSettings(ctx context.Context) UserSettings {
	us := UserSettings{MaxAddressCount: 5, MailAllowList: []string{}}
	raw, _ := s.db.GetSetting(ctx, "user_settings")
	if raw != "" {
		var tmp struct {
			UserSettings
			MaxAddressCount *int `json:"maxAddressCount"`
		}
		if json.Unmarshal([]byte(raw), &tmp) == nil {
			mac := us.MaxAddressCount
			us = tmp.UserSettings
			us.MaxAddressCount = mac
			if tmp.MaxAddressCount != nil && *tmp.MaxAddressCount >= 0 {
				us.MaxAddressCount = *tmp.MaxAddressCount
			}
			if us.MailAllowList == nil {
				us.MailAllowList = []string{}
			}
		}
	}
	return us
}

// LimitReached implements upstream isAddressCountLimitReached plus the
// monthly creation quota. roleName may be empty.
func (s *Store) LimitReached(ctx context.Context, userID int64, roleName string) (bool, string) {
	settings := s.UserSettings(ctx)
	max := settings.MaxAddressCount
	monthly := -1
	if roleName != "" {
		role, _ := s.Get(ctx, roleName)
		if role != nil {
			if role.MaxAddressCount != 0 {
				max = role.MaxAddressCount
			}
			monthly = role.MonthlyAddressQuota
		} else if cfg := s.roleAddressConfig(ctx)[roleName]; cfg != nil && cfg.MaxAddressCount != nil && *cfg.MaxAddressCount >= 0 {
			max = *cfg.MaxAddressCount
		}
	}
	if max > 0 {
		n, _ := s.db.Count(ctx, `SELECT COUNT(*) FROM users_address WHERE user_id = ?`, userID)
		if n >= int64(max) {
			return true, "Max address count reached"
		}
	}
	if monthly >= 0 {
		n, _ := s.db.Count(ctx,
			`SELECT COUNT(*) FROM users_address WHERE user_id = ? AND created_at >= date('now','start of month')`, userID)
		if n >= int64(monthly) {
			return true, "Monthly address quota reached"
		}
	}
	return false, ""
}

type roleAddressConfig struct {
	MaxAddressCount *int `json:"maxAddressCount"`
}

func (s *Store) roleAddressConfig(ctx context.Context) map[string]*roleAddressConfig {
	out := map[string]*roleAddressConfig{}
	raw, _ := s.db.GetSetting(ctx, "role_address_config")
	if raw != "" {
		_ = json.Unmarshal([]byte(raw), &out)
	}
	return out
}

func normalize(in []string) []string {
	out := []string{}
	for _, d := range in {
		if d = strings.ToLower(strings.TrimSpace(d)); d != "" {
			out = append(out, d)
		}
	}
	return out
}
