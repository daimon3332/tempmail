package server

import (
	"encoding/json"
	"fmt"
	"net/url"
	"regexp"
	"strconv"
	"strings"

	"tempmail/internal/config"
)

type environmentField struct {
	Key   string `json:"key"`
	Label string `json:"label"`
	Type  string `json:"type"`
	Help  string `json:"help"`
	Min   int64  `json:"min"`
	Max   int64  `json:"max"`
}

// These are startup settings without a dedicated runtime editor.
var environmentFields = []environmentField{
	{"INGEST_TOKEN", "Worker 收信令牌", "text", "与 Cloudflare Worker 的同名 Secret 一致；留空禁用令牌收信。", 0, 0},
	{"MAX_MESSAGE_BYTES", "单封邮件大小上限", "integer", "单位为字节；留空使用 26214400（25 MiB）。", 1, 1073741824},
	{"SMTP_HOSTNAME", "SMTP 服务器名称", "text", "直连 SMTP 握手使用的主机名；留空自动生成。", 0, 0},
	{"SMTP_CONFIG", "发信服务器配置", "smtp", "JSON 对象，以邮箱域名或 * 为键；含 host、port、secure、startTls、username、password、from。port 省略或为 0 时使用 587；留空使用直连 MX 发信。", 0, 0},
	{"DEFAULT_SEND_BALANCE", "默认发信余额", "integer", "兼容接口的初始发信余额；用户权限仍在用户管理中分配。留空为 0。", 0, 1000000000},
	{"FRONTEND_URL", "网站公开地址", "url", "用于 Passkey 等服务的站点来源。修改可能影响已有 Passkey。", 0, 0},
	{"ENABLE_CREATE_ADDRESS_SUBDOMAIN_MATCH", "允许匹配固定子域", "boolean", "允许 API 使用已配置后缀的子域；不会自动配置邮件路由。留空为 false。", 0, 0},
	{"CREATE_ADDRESS_DEFAULT_DOMAIN_FIRST", "优先默认邮箱域名", "boolean", "未指定后缀时优先使用默认域名。留空为 false。", 0, 0},
	{"ENABLE_INDEX_ABOUT", "显示关于信息", "boolean", "控制兼容接口的关于信息开关。留空为 false。", 0, 0},
	{"DISABLE_SHOW_GITHUB", "隐藏 GitHub 链接", "boolean", "控制兼容接口的项目链接开关。留空为 false。", 0, 0},
	{"TELEGRAM_BOT_TOKEN", "Telegram Bot Token", "text", "机器人凭据；留空停用 Telegram 集成。", 0, 0},
	{"TG_MAX_ADDRESS", "Telegram 邮箱数量上限", "integer", "每个 Telegram 用户的邮箱上限。留空为 5。", 1, 1000000},
	{"TG_ALLOW_USER_LANG", "Telegram 使用用户语言", "boolean", "留空为 false。", 0, 0},
	{"ENABLE_TG_PUSH_ATTACHMENT", "Telegram 推送附件", "boolean", "留空为 false。", 0, 0},
	{"TG_POLLING", "Telegram 轮询模式", "boolean", "启用轮询获取机器人消息；留空为 false。", 0, 0},
	{"S3_ENDPOINT", "S3 接口地址", "url", "附件存储服务的 HTTP(S) 地址。", 0, 0},
	{"S3_ACCESS_KEY_ID", "S3 Access Key", "text", "对象存储访问标识。", 0, 0},
	{"S3_SECRET_ACCESS_KEY", "S3 Secret Key", "text", "对象存储访问密钥。", 0, 0},
	{"S3_BUCKET", "S3 存储桶", "text", "保存附件的存储桶名称。", 0, 0},
	{"S3_REGION", "S3 区域", "text", "由存储服务提供；留空为 auto。", 0, 0},
	{"S3_URL_EXPIRES", "附件链接有效期", "integer", "单位为秒；留空为 360。", 1, 604800},
	{"CF_TURNSTILE_SITE_KEY", "Turnstile Site Key", "text", "Cloudflare 人机验证的站点标识。", 0, 0},
	{"CF_TURNSTILE_SECRET_KEY", "Turnstile Secret Key", "text", "Cloudflare 人机验证的服务端密钥。", 0, 0},
	{"ENABLE_GLOBAL_TURNSTILE_CHECK", "启用全局人机验证", "boolean", "需同时配置两项 Turnstile 密钥。留空为 false。", 0, 0},
}

func editableEnvironment(values map[string]string) map[string]string {
	out := map[string]string{}
	for _, f := range environmentFields {
		out[f.Key] = values[f.Key]
		if f.Type == "boolean" {
			switch strings.ToLower(strings.TrimSpace(out[f.Key])) {
			case "1", "yes", "true":
				out[f.Key] = "true"
			case "0", "no", "false":
				out[f.Key] = "false"
			}
		}
	}
	return out
}

var integerValue = regexp.MustCompile(`^[0-9]+$`)

func validateEnvironment(values map[string]string) error {
	fields := map[string]environmentField{}
	for _, f := range environmentFields {
		fields[f.Key] = f
	}
	managed := runtimeEnvValues(runtimeConfig{})
	for key, value := range values {
		f, ok := fields[key]
		if !ok {
			// Old clients may still submit dedicated settings. Their canonical
			// runtime fields remain authoritative; never write these raw values.
			if _, managedKey := managed[key]; managedKey {
				continue
			}
			return fmt.Errorf("%s 不允许在高级配置中修改", key)
		}
		if strings.ContainsAny(value, "\r\n\x00") && f.Type != "smtp" {
			return fmt.Errorf("%s 不能包含换行或空字符", key)
		}
		if value == "" {
			continue
		}
		switch f.Type {
		case "boolean":
			if value != "true" && value != "false" {
				return fmt.Errorf("%s 只能选择 true 或 false", key)
			}
		case "integer":
			n, err := strconv.ParseInt(value, 10, 64)
			if !integerValue.MatchString(value) || err != nil || n < f.Min || n > f.Max {
				return fmt.Errorf("%s 必须为 %d–%d 之间的整数", key, f.Min, f.Max)
			}
		case "url":
			u, err := url.Parse(value)
			if err != nil || u.Host == "" || (u.Scheme != "https" && u.Scheme != "http") || u.User != nil {
				return fmt.Errorf("%s 必须为有效 HTTP(S) 地址", key)
			}
		case "smtp":
			var relays map[string]config.SMTPRelay
			decoder := json.NewDecoder(strings.NewReader(value))
			decoder.DisallowUnknownFields()
			if !json.Valid([]byte(value)) || decoder.Decode(&relays) != nil || relays == nil {
				return fmt.Errorf("%s 必须为有效的 SMTP 配置 JSON 对象", key)
			}
			for domain, relay := range relays {
				if strings.TrimSpace(domain) == "" || strings.TrimSpace(relay.Host) == "" || relay.Port < 0 || relay.Port > 65535 {
					return fmt.Errorf("%s 的每项配置必须包含域名、host；port 必须为 0–65535 的整数", key)
				}
			}
		}
	}
	return nil
}
