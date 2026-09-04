package server

import (
	"context"
	"encoding/json"
)

const runtimeKey = "runtime_config"

// runtimeConfig holds the hot-reloadable subset of configuration, persisted in
// the settings table. It is layered over the env-loaded Config; fields the
// admin config page manages take effect immediately on the next read.
// Domains are intentionally excluded (they affect SMTP/MX acceptance and need a
// restart). Auth passwords can be overridden when the *_enabled flags are set.
type runtimeConfig struct {
	SitePassword         string `json:"site_password"`
	SitePasswordEnabled  bool   `json:"site_password_enabled"`
	AdminPassword        string `json:"admin_password"`
	AdminPasswordEnabled bool   `json:"admin_password_enabled"`
	APIKey               string `json:"api_key"`

	Title        string `json:"title"`
	Announcement string `json:"announcement"`
	Copyright    string `json:"copyright"`
	AdminContact string `json:"admin_contact"`
	DefaultLang  string `json:"default_lang"`

	Prefix                 string   `json:"prefix"`
	MinAddressLen          int      `json:"min_address_len"`
	MaxAddressLen          int      `json:"max_address_len"`
	AddressRegex           string   `json:"address_regex"`
	AddressCheckRegex      string   `json:"address_check_regex"`
	RandomSubdomainDomains []string `json:"random_subdomain_domains"`
	RandomSubdomainLength  int      `json:"random_subdomain_length"`

	EnableUserCreateEmail           bool `json:"enable_user_create_email"`
	DisableAnonymousUserCreateEmail bool `json:"disable_anonymous_user_create_email"`
	DisableCustomAddressName        bool `json:"disable_custom_address_name"`
	EnableUserDeleteEmail           bool `json:"enable_user_delete_email"`
	EnableMailReadStatus            bool `json:"enable_mail_read_status"`
	EnableAddressPassword           bool `json:"enable_address_password"`
	EnableWebhook                   bool `json:"enable_webhook"`
	EnableAutoReply                 bool `json:"enable_auto_reply"`
	EnableCheckJunkMail             bool `json:"enable_check_junk_mail"`
	BlockUnknownAddress             bool `json:"block_unknown_address"`

	AIEnabled             bool     `json:"ai_enabled"`
	AIEndpoint            string   `json:"ai_endpoint"`
	AIAPIKey              string   `json:"ai_api_key"`
	AIModel               string   `json:"ai_model"`
	AIEnableAllowList     bool     `json:"ai_enable_allow_list"`
	AIAllowList           []string `json:"ai_allow_list"`
	AIEnableRegexFallback bool     `json:"ai_enable_regex_fallback"`
}

// readRuntime returns the stored runtime overrides (empty struct if none).
func (a *App) readRuntime(ctx context.Context) runtimeConfig {
	var rc runtimeConfig
	if raw, _ := a.db.GetSetting(ctx, runtimeKey); raw != "" {
		_ = json.Unmarshal([]byte(raw), &rc)
	}
	return rc
}

// effective returns cfg values with runtime overrides applied.
func (a *App) effective(ctx context.Context) runtimeConfig {
	base := runtimeConfig{
		Title:        a.cfg.Title,
		Announcement: a.cfg.Announcement,
		Copyright:    a.cfg.Copyright,
		AdminContact: a.cfg.AdminContact,
		DefaultLang:  a.cfg.DefaultLang,
		Prefix:       a.cfg.Prefix,

		MinAddressLen:                   a.cfg.MinAddressLen,
		MaxAddressLen:                   a.cfg.MaxAddressLen,
		AddressRegex:                    a.cfg.AddressRegex,
		AddressCheckRegex:               a.cfg.AddressCheckRegex,
		RandomSubdomainDomains:          a.cfg.RandomSubdomainDomains,
		RandomSubdomainLength:           a.cfg.RandomSubdomainLength,
		EnableUserCreateEmail:           a.cfg.EnableUserCreateEmail,
		DisableAnonymousUserCreateEmail: a.cfg.DisableAnonymousUserCreateEmail,
		DisableCustomAddressName:        a.cfg.DisableCustomAddressName,
		EnableUserDeleteEmail:           a.cfg.EnableUserDeleteEmail,
		EnableMailReadStatus:            a.cfg.EnableMailReadStatus,
		EnableAddressPassword:           a.cfg.EnableAddressPassword,
		EnableWebhook:                   a.cfg.EnableWebhook,
		EnableAutoReply:                 a.cfg.EnableAutoReply,
		EnableCheckJunkMail:             a.cfg.EnableCheckJunkMail,
		BlockUnknownAddress:             a.cfg.BlockUnknownAddress,
		AIEndpoint:                      a.cfg.AIExtractEndpoint,
		AIAPIKey:                        a.cfg.AIExtractAPIKey,
		AIModel:                         a.cfg.AIExtractModel,
		AIEnabled:                       a.cfg.AIExtractEndpoint != "" && a.cfg.AIExtractAPIKey != "",
	}
	rc := a.readRuntime(ctx)
	b, _ := json.Marshal(rc)
	if string(b) == "{}" {
		return base
	}
	// Apply only on each field to keep domains untouched.
	if rc.Title != "" {
		base.Title = rc.Title
	}
	if rc.Announcement != "" {
		base.Announcement = rc.Announcement
	}
	if rc.Copyright != "" {
		base.Copyright = rc.Copyright
	}
	if rc.AdminContact != "" {
		base.AdminContact = rc.AdminContact
	}
	if rc.DefaultLang != "" {
		base.DefaultLang = rc.DefaultLang
	}
	if rc.Prefix != "" {
		base.Prefix = rc.Prefix
	}
	if rc.MinAddressLen != 0 {
		base.MinAddressLen = rc.MinAddressLen
	}
	if rc.MaxAddressLen != 0 {
		base.MaxAddressLen = rc.MaxAddressLen
	}
	if rc.AddressRegex != "" {
		base.AddressRegex = rc.AddressRegex
	}
	if rc.AddressCheckRegex != "" {
		base.AddressCheckRegex = rc.AddressCheckRegex
	}
	if len(rc.RandomSubdomainDomains) > 0 {
		base.RandomSubdomainDomains = rc.RandomSubdomainDomains
	}
	if rc.RandomSubdomainLength != 0 {
		base.RandomSubdomainLength = rc.RandomSubdomainLength
	}

	base.EnableUserCreateEmail = rc.EnableUserCreateEmail
	base.DisableAnonymousUserCreateEmail = rc.DisableAnonymousUserCreateEmail
	base.DisableCustomAddressName = rc.DisableCustomAddressName
	base.EnableUserDeleteEmail = rc.EnableUserDeleteEmail
	base.EnableMailReadStatus = rc.EnableMailReadStatus
	base.EnableAddressPassword = rc.EnableAddressPassword
	base.EnableWebhook = rc.EnableWebhook
	base.EnableAutoReply = rc.EnableAutoReply
	base.EnableCheckJunkMail = rc.EnableCheckJunkMail
	base.BlockUnknownAddress = rc.BlockUnknownAddress

	base.APIKey = rc.APIKey
	base.AIEnabled = rc.AIEnabled
	if rc.AIEndpoint != "" {
		base.AIEndpoint = rc.AIEndpoint
	}
	if rc.AIAPIKey != "" {
		base.AIAPIKey = rc.AIAPIKey
	}
	if rc.AIModel != "" {
		base.AIModel = rc.AIModel
	}
	base.AIEnableAllowList = rc.AIEnableAllowList
	base.AIAllowList = rc.AIAllowList
	base.AIEnableRegexFallback = rc.AIEnableRegexFallback
	return base
}

func (a *App) saveRuntime(ctx context.Context, rc runtimeConfig) error {
	raw, _ := json.Marshal(rc)
	if err := a.db.SaveSetting(ctx, runtimeKey, string(raw)); err != nil {
		return err
	}
	as := aiExtractSettings{Enabled: rc.AIEnabled, EnableAllowList: rc.AIEnableAllowList,
		AllowList: rc.AIAllowList, EnableRegexFallback: boolTrue(rc.AIEnableRegexFallback)}
	if as.AllowList == nil {
		as.AllowList = []string{}
	}
	_ = a.saveJSONSetting(ctx, "ai_extract_settings", as)
	a.audit(ctx, "runtime_config", "runtime configuration updated")
	return nil
}

func boolTrue(v bool) bool { return v }
