package server

import (
	"context"
	"encoding/json"
)

const runtimeKey = "runtime_config"

// runtimeConfig holds the hot-reloadable subset of configuration, persisted in
// the settings table. It is layered over the env-loaded Config; fields the
// admin config page manages take effect immediately on the next read.
// Domain changes are applied to HTTP creation/listing immediately; SMTP/MX
// acceptance still requires a service restart. Auth passwords can be
// overridden when the *_enabled flags are set.
type runtimeConfig struct {
	SitePassword          string   `json:"site_password"`
	SitePasswordEnabled   bool     `json:"site_password_enabled"`
	AdminPassword         string   `json:"admin_password"`
	AdminPasswordEnabled  bool     `json:"admin_password_enabled"`
	APIKey                string   `json:"api_key"`
	RateLimitPerMinute    int      `json:"rate_limit_per_minute"`
	RateLimitPerMinuteSet bool     `json:"rate_limit_per_minute_set,omitempty"`
	Domains               []string `json:"domains"`
	DefaultDomains        []string `json:"default_domains"`

	Title        string `json:"title"`
	Announcement string `json:"announcement"`
	Copyright    string `json:"copyright"`
	AdminContact string `json:"admin_contact"`
	DefaultLang  string `json:"default_lang"`

	Prefix                    string   `json:"prefix"`
	PrefixSet                 bool     `json:"prefix_set,omitempty"`
	MinAddressLen             int      `json:"min_address_len"`
	MaxAddressLen             int      `json:"max_address_len"`
	AddressRegex              string   `json:"address_regex"`
	AddressCheckRegex         string   `json:"address_check_regex"`
	RandomSubdomainDomains    []string `json:"random_subdomain_domains"`
	RandomSubdomainDomainsSet bool     `json:"random_subdomain_domains_set,omitempty"`
	RandomSubdomainLength     int      `json:"random_subdomain_length"`
	RandomSubdomainLengthSet  bool     `json:"random_subdomain_length_set,omitempty"`

	EnableUserCreateEmail           bool     `json:"enable_user_create_email"`
	DisableAnonymousUserCreateEmail bool     `json:"disable_anonymous_user_create_email"`
	DisableCustomAddressName        bool     `json:"disable_custom_address_name"`
	EnableUserDeleteEmail           bool     `json:"enable_user_delete_email"`
	EnableMailReadStatus            bool     `json:"enable_mail_read_status"`
	EnableAddressPassword           bool     `json:"enable_address_password"`
	EnableWebhook                   bool     `json:"enable_webhook"`
	EnableAutoReply                 bool     `json:"enable_auto_reply"`
	EnableCheckJunkMail             bool     `json:"enable_check_junk_mail"`
	BlockUnknownAddress             bool     `json:"block_unknown_address"`
	ForwardAddressList              []string `json:"forward_address_list"`
	JunkMailCheckList               []string `json:"junk_mail_check_list"`

	AIEnabled             bool              `json:"ai_enabled"`
	AIEndpoint            string            `json:"ai_endpoint"`
	AIAPIKey              string            `json:"ai_api_key"`
	AIModel               string            `json:"ai_model"`
	AIEnableAllowList     bool              `json:"ai_enable_allow_list"`
	AIAllowList           []string          `json:"ai_allow_list"`
	AIEnableRegexFallback bool              `json:"ai_enable_regex_fallback"`
	Environment           map[string]string `json:"environment,omitempty"`
}

// readRuntime returns the stored runtime overrides (empty struct if none).
func (a *App) readRuntime(ctx context.Context) (runtimeConfig, bool) {
	var rc runtimeConfig
	raw, err := a.db.GetSetting(ctx, runtimeKey)
	if err != nil || raw == "" {
		return rc, false
	}
	if err := json.Unmarshal([]byte(raw), &rc); err != nil {
		return runtimeConfig{}, false
	}
	return rc, true
}

// effective returns cfg values with runtime overrides applied.
func (a *App) effective(ctx context.Context) runtimeConfig {
	base := runtimeConfig{
		SitePassword:         first(a.cfg.Passwords),
		AdminPassword:        first(a.cfg.AdminPasswords),
		SitePasswordEnabled:  len(a.cfg.Passwords) > 0,
		AdminPasswordEnabled: len(a.cfg.AdminPasswords) > 0,
		APIKey:               a.cfg.APIKey,
		Title:                a.cfg.Title,
		Announcement:         a.cfg.Announcement,
		Copyright:            a.cfg.Copyright,
		AdminContact:         a.cfg.AdminContact,
		DefaultLang:          a.cfg.DefaultLang,
		Prefix:               a.cfg.Prefix,

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
		ForwardAddressList:              append([]string{}, a.cfg.ForwardAddressList...),
		JunkMailCheckList:               append([]string{}, a.cfg.JunkMailCheckList...),
		AIEndpoint:                      a.cfg.AIExtractEndpoint,
		AIAPIKey:                        a.cfg.AIExtractAPIKey,
		AIModel:                         a.cfg.AIExtractModel,
		AIEnabled:                       a.cfg.AIExtractEndpoint != "" && a.cfg.AIExtractAPIKey != "",
		RateLimitPerMinute:              a.cfg.RateLimitPerMinute,
		Domains:                         append([]string{}, a.cfg.Domains...),
		DefaultDomains:                  append([]string{}, a.cfg.DefaultDomains...),
	}
	rc, ok := a.readRuntime(ctx)
	if !ok {
		return base
	}
	// Saved booleans are authoritative. Empty display fields fall back to env;
	// optional content fields may intentionally be cleared.
	if rc.Title != "" {
		base.Title = rc.Title
	}
	base.Announcement = rc.Announcement
	base.Copyright = rc.Copyright
	base.AdminContact = rc.AdminContact
	if rc.DefaultLang != "" {
		base.DefaultLang = rc.DefaultLang
	}
	if rc.PrefixSet || rc.Prefix != "" {
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
	if rc.RandomSubdomainDomainsSet || len(rc.RandomSubdomainDomains) > 0 {
		base.RandomSubdomainDomains = rc.RandomSubdomainDomains
	}
	if rc.RandomSubdomainLengthSet || rc.RandomSubdomainLength != 0 {
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
	if rc.ForwardAddressList != nil {
		base.ForwardAddressList = append([]string{}, rc.ForwardAddressList...)
	}
	if rc.JunkMailCheckList != nil {
		base.JunkMailCheckList = append([]string{}, rc.JunkMailCheckList...)
	}

	base.SitePassword = rc.SitePassword
	base.SitePasswordEnabled = rc.SitePasswordEnabled
	base.AdminPassword = rc.AdminPassword
	base.AdminPasswordEnabled = rc.AdminPasswordEnabled
	base.APIKey = rc.APIKey
	if rc.RateLimitPerMinuteSet {
		base.RateLimitPerMinute = rc.RateLimitPerMinute
	}
	if len(rc.Domains) > 0 {
		base.Domains = append([]string{}, rc.Domains...)
	}
	if len(rc.DefaultDomains) > 0 {
		base.DefaultDomains = append([]string{}, rc.DefaultDomains...)
	}
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
	if rc.Environment != nil {
		base.Environment = rc.Environment
	}
	return base
}

func first(values []string) string {
	if len(values) == 0 {
		return ""
	}
	return values[0]
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
	return nil
}

func (a *App) MailFeatureEnabled(ctx context.Context, name string) bool {
	rc := a.effective(ctx)
	switch name {
	case "block_unknown_address":
		return rc.BlockUnknownAddress
	case "check_junk_mail":
		return rc.EnableCheckJunkMail
	case "mail_read_status":
		return rc.EnableMailReadStatus
	case "auto_reply":
		return rc.EnableAutoReply
	default:
		return false
	}
}

func (a *App) MailJunkCheckList(ctx context.Context) []string {
	return append([]string(nil), a.effective(ctx).JunkMailCheckList...)
}

func boolTrue(v bool) bool { return v }
