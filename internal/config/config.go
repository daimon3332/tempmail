package config

import (
	"encoding/json"
	"log"
	"os"
	"strconv"
	"strings"
)

// UserRole mirrors the original project's USER_ROLES entry.
type UserRole struct {
	Role    string   `json:"role"`
	Domains []string `json:"domains"`
	Prefix  *string  `json:"prefix,omitempty"`
}

type SMTPRelay struct {
	Host     string `json:"host"`
	Port     int    `json:"port"`
	Secure   bool   `json:"secure"`
	StartTLS bool   `json:"startTls"`
	Username string `json:"username"`
	Password string `json:"password"`
	From     string `json:"from"`
}

type Config struct {
	HTTPAddr  string
	SMTPAddr  string
	DBPath    string
	JWTSecret string

	Title        string
	Announcement string
	DefaultLang  string
	Copyright    string
	AdminContact string
	FrontendURL  string

	Domains                []string
	DefaultDomains         []string
	RandomSubdomainDomains []string
	RandomSubdomainLength  int
	DomainLabels           []string
	Prefix                 string
	MinAddressLen          int
	MaxAddressLen          int
	AddressRegex           string
	AddressCheckRegex      string

	Passwords       []string
	AdminPasswords  []string
	AdminUserRole   string
	UserDefaultRole string
	UserRoles       []UserRole
	NoLimitSendRole []string

	EnableUserCreateEmail             bool
	DisableAnonymousUserCreateEmail   bool
	DisableCustomAddressName          bool
	EnableUserDeleteEmail             bool
	EnableMailReadStatus              bool
	EnableAutoReply                   bool
	EnableAddressPassword             bool
	EnableWebhook                     bool
	EnableIndexAbout                  bool
	EnableCreateAddressSubdomainMatch bool
	CreateAddressDefaultDomainFirst   bool
	DisableAdminPasswordCheck         bool
	DisableShowGithub                 bool

	SMTPRelay           map[string]SMTPRelay
	DefaultSendBalance  int
	MaxMessageBytes     int64
	SMTPHostname        string
	BlockUnknownAddress bool
	ForwardAddressList  []string
	JunkMailCheckList   []string
	EnableCheckJunkMail bool

	TrustedProxies []string
	IngestToken    string
	OriginToken    string

	TelegramBotToken       string
	TGMaxAddress           int
	TGAllowUserLang        bool
	EnableTGPushAttachment bool
	TGPolling              bool

	S3Endpoint        string
	S3AccessKeyID     string
	S3SecretAccessKey string
	S3Bucket          string
	S3Region          string
	S3URLExpires      int

	TurnstileSiteKey      string
	TurnstileSecretKey    string
	EnableGlobalTurnstile bool

	AIExtractEndpoint  string
	AIExtractAPIKey    string
	AIExtractModel     string
	RateLimitPerMinute int
}

func env(key, def string) string {
	if v, ok := os.LookupEnv(key); ok && v != "" {
		return v
	}
	return def
}

func envBool(key string, def bool) bool {
	v := strings.ToLower(strings.TrimSpace(os.Getenv(key)))
	if v == "" {
		return def
	}
	return v == "true" || v == "1" || v == "yes"
}

func envInt(key string, def int) int {
	if v := os.Getenv(key); v != "" {
		if n, err := strconv.Atoi(v); err == nil {
			return n
		}
	}
	return def
}

// envList accepts either a JSON array or a comma separated list.
func envList(key string) []string {
	v := strings.TrimSpace(os.Getenv(key))
	if v == "" {
		return nil
	}
	var out []string
	if strings.HasPrefix(v, "[") {
		if err := json.Unmarshal([]byte(v), &out); err != nil {
			log.Printf("config: %s is not a valid JSON array: %v", key, err)
		}
	} else {
		out = strings.Split(v, ",")
	}
	res := out[:0]
	for _, s := range out {
		if s = strings.TrimSpace(s); s != "" {
			res = append(res, s)
		}
	}
	return res
}

func normalizeDomains(in []string) []string {
	out := make([]string, 0, len(in))
	for _, d := range in {
		if d = strings.ToLower(strings.TrimSpace(d)); d != "" {
			out = append(out, d)
		}
	}
	return out
}

func Load() *Config {
	c := &Config{
		HTTPAddr:  env("HTTP_ADDR", ":8080"),
		SMTPAddr:  env("SMTP_ADDR", ":25"),
		DBPath:    env("DB_PATH", "./data/tempmail.db"),
		JWTSecret: os.Getenv("JWT_SECRET"),

		Title:        env("TITLE", "Temp Email"),
		Announcement: os.Getenv("ANNOUNCEMENT"),
		DefaultLang:  env("DEFAULT_LANG", "zh"),
		Copyright:    os.Getenv("COPYRIGHT"),
		AdminContact: os.Getenv("ADMIN_CONTACT"),
		FrontendURL:  os.Getenv("FRONTEND_URL"),

		Domains:                normalizeDomains(envList("DOMAINS")),
		DefaultDomains:         normalizeDomains(envList("DEFAULT_DOMAINS")),
		RandomSubdomainDomains: normalizeDomains(envList("RANDOM_SUBDOMAIN_DOMAINS")),
		RandomSubdomainLength:  envInt("RANDOM_SUBDOMAIN_LENGTH", 8),
		DomainLabels:           envList("DOMAIN_LABELS"),
		Prefix:                 strings.ToLower(strings.TrimSpace(os.Getenv("PREFIX"))),
		MinAddressLen:          envInt("MIN_ADDRESS_LEN", 1),
		MaxAddressLen:          envInt("MAX_ADDRESS_LEN", 30),
		AddressRegex:           os.Getenv("ADDRESS_REGEX"),
		AddressCheckRegex:      os.Getenv("ADDRESS_CHECK_REGEX"),

		Passwords:       envList("PASSWORDS"),
		AdminPasswords:  envList("ADMIN_PASSWORDS"),
		AdminUserRole:   os.Getenv("ADMIN_USER_ROLE"),
		UserDefaultRole: os.Getenv("USER_DEFAULT_ROLE"),
		NoLimitSendRole: envList("NO_LIMIT_SEND_ROLE"),

		EnableUserCreateEmail:             envBool("ENABLE_USER_CREATE_EMAIL", true),
		DisableAnonymousUserCreateEmail:   envBool("DISABLE_ANONYMOUS_USER_CREATE_EMAIL", false),
		DisableCustomAddressName:          envBool("DISABLE_CUSTOM_ADDRESS_NAME", false),
		EnableUserDeleteEmail:             envBool("ENABLE_USER_DELETE_EMAIL", true),
		EnableMailReadStatus:              envBool("ENABLE_MAIL_READ_STATUS", true),
		EnableAutoReply:                   envBool("ENABLE_AUTO_REPLY", false),
		EnableAddressPassword:             envBool("ENABLE_ADDRESS_PASSWORD", false),
		EnableWebhook:                     envBool("ENABLE_WEBHOOK", false),
		EnableIndexAbout:                  envBool("ENABLE_INDEX_ABOUT", false),
		EnableCreateAddressSubdomainMatch: envBool("ENABLE_CREATE_ADDRESS_SUBDOMAIN_MATCH", false),
		CreateAddressDefaultDomainFirst:   envBool("CREATE_ADDRESS_DEFAULT_DOMAIN_FIRST", false),
		DisableAdminPasswordCheck:         envBool("DISABLE_ADMIN_PASSWORD_CHECK", false),
		DisableShowGithub:                 envBool("DISABLE_SHOW_GITHUB", false),

		DefaultSendBalance:  envInt("DEFAULT_SEND_BALANCE", 0),
		MaxMessageBytes:     int64(envInt("MAX_MESSAGE_BYTES", 25*1024*1024)),
		SMTPHostname:        os.Getenv("SMTP_HOSTNAME"),
		BlockUnknownAddress: envBool("BLOCK_UNKNOWN_ADDRESS", false),
		ForwardAddressList:  envList("FORWARD_ADDRESS_LIST"),
		JunkMailCheckList:   envList("JUNK_MAIL_CHECK_LIST"),
		EnableCheckJunkMail: envBool("ENABLE_CHECK_JUNK_MAIL", false),
		TrustedProxies:      envList("TRUSTED_PROXIES"),
		IngestToken:         os.Getenv("INGEST_TOKEN"),
		OriginToken:         os.Getenv("ORIGIN_TOKEN"),

		TelegramBotToken:       os.Getenv("TELEGRAM_BOT_TOKEN"),
		TGMaxAddress:           envInt("TG_MAX_ADDRESS", 5),
		TGAllowUserLang:        envBool("TG_ALLOW_USER_LANG", false),
		EnableTGPushAttachment: envBool("ENABLE_TG_PUSH_ATTACHMENT", false),
		TGPolling:              envBool("TG_POLLING", false),

		S3Endpoint:        os.Getenv("S3_ENDPOINT"),
		S3AccessKeyID:     os.Getenv("S3_ACCESS_KEY_ID"),
		S3SecretAccessKey: os.Getenv("S3_SECRET_ACCESS_KEY"),
		S3Bucket:          os.Getenv("S3_BUCKET"),
		S3Region:          env("S3_REGION", "auto"),
		S3URLExpires:      envInt("S3_URL_EXPIRES", 360),

		TurnstileSiteKey:      os.Getenv("CF_TURNSTILE_SITE_KEY"),
		TurnstileSecretKey:    os.Getenv("CF_TURNSTILE_SECRET_KEY"),
		EnableGlobalTurnstile: envBool("ENABLE_GLOBAL_TURNSTILE_CHECK", false),

		AIExtractEndpoint:  os.Getenv("AI_EXTRACT_ENDPOINT"),
		AIExtractAPIKey:    os.Getenv("AI_EXTRACT_API_KEY"),
		AIExtractModel:     env("AI_EXTRACT_MODEL", "gpt-4o-mini"),
		RateLimitPerMinute: envInt("RATE_LIMIT_PER_MINUTE", 30),
	}

	if raw := os.Getenv("USER_ROLES"); raw != "" {
		if err := json.Unmarshal([]byte(raw), &c.UserRoles); err != nil {
			log.Fatalf("config: USER_ROLES is not valid JSON: %v", err)
		}
		for i := range c.UserRoles {
			c.UserRoles[i].Domains = normalizeDomains(c.UserRoles[i].Domains)
		}
	}
	if raw := os.Getenv("SMTP_CONFIG"); raw != "" {
		if err := json.Unmarshal([]byte(raw), &c.SMTPRelay); err != nil {
			log.Fatalf("config: SMTP_CONFIG is not valid JSON: %v", err)
		}
	}
	if len(c.Domains) == 0 {
		log.Fatal("config: DOMAINS is required")
	}
	if c.JWTSecret == "" {
		log.Fatal("config: JWT_SECRET is required")
	}
	if len(c.DefaultDomains) == 0 {
		c.DefaultDomains = c.Domains
	}
	if c.SMTPHostname == "" {
		c.SMTPHostname = "mail." + c.Domains[0]
	}
	return c
}

func (c *Config) RelayFor(domain string) *SMTPRelay {
	domain = strings.ToLower(domain)
	for k, v := range c.SMTPRelay {
		if strings.ToLower(k) == domain {
			return &v
		}
	}
	if v, ok := c.SMTPRelay["*"]; ok {
		return &v
	}
	return nil
}

func (c *Config) S3Enabled() bool {
	return c.S3Endpoint != "" && c.S3AccessKeyID != "" && c.S3SecretAccessKey != "" && c.S3Bucket != ""
}

func (c *Config) TurnstileEnabled() bool {
	return c.TurnstileSiteKey != "" && c.TurnstileSecretKey != ""
}

func (c *Config) GlobalTurnstile() bool { return c.EnableGlobalTurnstile && c.TurnstileEnabled() }
