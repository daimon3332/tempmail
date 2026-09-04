# Tempmail

Self-hosted replacement for [cloudflare_temp_email](https://github.com/dreamhunter2333/cloudflare_temp_email):
same HTTP API, same database schema — running as a single Go binary on your own server (SQLite, no D1 read
limits). New React frontend (DuckMail-style mail client + Mailbucket-style admin dashboard).
Existing clients keep the same `base_url` and only need to swap the domain.

## 项目定位与功能

Tempmail 是基于原版 `cloudflare_temp_email` 的自托管重构：保留原版 HTTP API、JWT 字段和数据库导入能力，同时增加普通用户登录、用户配额、管理员全量视图、批量维护和可视化配置。邮箱凭据沿用原版语义，仅使用 Address JWT；用户账户才使用用户名和密码登录。

主要功能包括：

- 用户登录、管理员登录、邮箱 JWT 登录和按用户配额创建邮箱
- 按域名/邮箱/邮件搜索、筛选、排序、分页，支持 HTML、纯文本、原始邮件和附件
- 管理员查看全部邮箱、邮件、未知邮件、发件箱、统计、用户和操作日志
- 邮箱 JWT 创建、复制、重新查看，用户可删除自己的邮箱并返还月度额度
- 自动清理（邮件、未知邮件、发件箱、邮箱）与批量立即清理
- SQLite 备份、迁移，以及 Cloudflare Temp Mail / cf-teml-mail SQL 导入和合并
- 自动回复、垃圾邮件检查、邮件转发、Webhook、AI 提取、Telegram、Passkey 和 OAuth2
- 域名、随机子域、邮箱名前缀、限流和运行时环境变量管理

未登录用户只能访问登录页和公开 API 文档（`/docs/api`、`/api/openapi.json`）。Cloudflare 强绑定能力（Email Routing、Turnstile、Worker、S3 等）仅在配置对应环境变量后启用，不会伪造为已连接。

### 邮箱凭据

创建邮箱接口返回 `address`、`address_id` 和 `jwt`。`jwt` 是访问邮箱 API 的 Address JWT，可放在 `Authorization: Bearer` 或兼容请求头中。系统不再为新邮箱生成独立密码；`/api/address_login` 等旧路径保留用于兼容旧部署，不作为新邮箱登录方式。

### 域名配置

设置页提供“域名配置教程”链接，完整步骤见 [`web/app/public/docs/domain-setup.html`](web/app/public/docs/domain-setup.html)。添加域名后，HTTP 创建配置可立即读取；SMTP/MX 接收域名需要重启服务。

### Webhook

邮件 Webhook 支持启停、HTTPS 地址、请求头、Body 模板和 HMAC 签名（`X-Tempmail-Timestamp`、`X-Tempmail-Signature`）。接收方应校验签名和时间戳，并按邮件 ID 实现幂等处理。

## Layout

```
cmd/tempmail          entrypoint (+ `-import` for D1 dumps)
internal/config       env -> Config (same variable names as upstream wrangler.toml)
internal/db           SQLite (modernc, no CGO), upstream schema + role_configs
internal/auth         HS256 JWT, identical claims to upstream (tokens interchangeable)
internal/mailparse    MIME -> sender/subject/text/html/attachments
internal/mailer       outbound: SMTP relay (SMTP_CONFIG) or direct-to-MX
internal/inbound      SMTP receiver :25 + HTTP ingest pipeline
internal/roles        USER_ROLES (env) merged with dynamic roles (DB) + quota checks
internal/passkey      WebAuthn (go-webauthn) on /user_api/passkey/*
internal/s3store      presigned S3 URLs on /api/attachment/*
internal/telegram     Telegram bot + miniapp (initData auth) + mail push
internal/server       HTTP router, guards (Turnstile/IP-blacklist/rate-limit), AI extract
internal/importer     wrangler `d1 export` SQL -> SQLite (primary or merge mode)
web/app               React+Vite+Tailwind frontend -> web/dist -> embedded in binary
workers/              Cloudflare bridge: proxy.js (custom domain alias), ingest.js (Email Routing)
```

## Mail flow

1. Cloudflare Email Routing (all zones) -> worker `tempmail-ingest` -> `POST https://tempmail.333186.xyz/external/ingest`
2. Direct SMTP on port 25 (point MX at the server to drop Cloudflare entirely)

`https://tempmail.333186.xyz` is served by **nginx** (`/etc/nginx/sites-available/tempmail`) which reverse-proxies
to `127.0.0.1:8080` (the Go binary). Port 8080 is loopback-only. `tempmail.daimon.dpdns.org` is kept as an alias
via a Cloudflare worker that forwards to the same origin.

## Run

```bash
cp .env.example .env   # set DOMAINS, ADMIN_PASSWORDS, JWT_SECRET, INGEST_TOKEN
docker compose up -d --build
```

Import an existing D1 database (once):

```bash
npx wrangler d1 export <db> --remote --output=backups/d1/main.sql
docker compose run --rm --no-deps tempmail -import /backups/d1/main.sql          # ids preserved
docker compose run --rm --no-deps tempmail -import /backups/d1/other.sql -merge  # fold in a second db
```

`deploy.sh` syncs sources, builds, and restarts; `workers/deploy.sh` publishes the Cloudflare workers.

## Frontend

`web/app` is a Vite + React + Tailwind SPA (TypeScript-lite .jsx). Build:
`cd web/app && pnpm install && pnpm build` (outputs to `web/dist`, embedded into the Go binary).
The Docker image builds it with `VITE_OUT_DIR=/out`.

## API

Everything the upstream frontend calls is implemented with the same paths and headers
(`x-admin-auth`, `x-user-token`, `x-user-access-token`, `x-custom-auth`, `Authorization: Bearer <address jwt>`).
Not supported: S3 disabled unless `S3_*` env is set; Telegram requires `TELEGRAM_BOT_TOKEN`; Turnstile requires
`CF_TURNSTILE_*`; AI extract requires `AI_EXTRACT_*`. Passkey requires an HTTPS origin (WebAuthn).

Additions (admin auth unless noted):

| Method | Path | Purpose |
|---|---|---|
| GET | `/health`, `/health_check` | liveness (no auth) |
| POST | `/admin/ensure_address` | create-or-reuse -> `{address,address_id,jwt,reused}` |
| GET | `/admin/address/lookup?address=` | exact lookup -> `{found,...}` |
| GET | `/admin/address/{id}`, `/admin/address/{id}/mails` | detail / parsed mails |
| POST | `/admin/address/access` | get an address JWT |
| POST | `/admin/address/{id}/archive\|restore\|recreate` | maintenance |
| GET/POST/DELETE | `/admin/roles` | dynamic roles: `max_address_count`, `monthly_address_quota`, `can_custom_name`, `can_send_mail`, `domains` |
| POST | `/external/ingest` | `{from,to,raw\|raw_base64}` with `x-ingest-token` |

`GET /api/mails` and `GET /admin/mails` accept `after_id=` / `after=<RFC3339>` for incremental polling.

## Roles and quotas

Roles come from `USER_ROLES` (env, upstream format) and/or `role_configs` (created via `/admin/roles`,
editable at runtime). Assign with `POST /admin/user_roles {user_id, role_text}`. Enforced on
`/api/new_address` (logged-in), `/user_api/bind_address`, transfers. `max_address_count` (-1 unlimited,
0 = global `user_settings.maxAddressCount`), `monthly_address_quota`, `can_custom_name`, `can_send_mail`, `domains`.

## Development

```bash
go test ./...
cd web/app && pnpm install && pnpm dev   # vite on :5173, proxies /api etc to :8080
DOMAINS='["example.com"]' JWT_SECRET=dev ADMIN_PASSWORDS='["dev"]' HTTP_ADDR=:8080 SMTP_ADDR=:2525 go run ./cmd/tempmail
```
