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
- iCloud 隐藏邮箱（Hide My Email）：管理员用 iCloud+ 账号批量生成 `@icloud.com` 别名，邮件实时进入对应邮箱

未登录用户只能访问登录页和公开 API 文档（`/docs/api`、`/api/openapi.json`）。Cloudflare 强绑定能力（Email Routing、Turnstile、Worker、S3 等）仅在配置对应环境变量后启用，不会伪造为已连接。

### 邮箱凭据

创建邮箱接口返回 `address`、`address_id` 和 `jwt`。`jwt` 是访问邮箱 API 的 Address JWT，可放在 `Authorization: Bearer` 或兼容请求头中。系统不再为新邮箱生成独立密码；`/api/address_login` 等旧路径保留用于兼容旧部署，不作为新邮箱登录方式。

### 域名配置

已托管在 Cloudflare 的域名：部署 `tempmail-ingest`（配置网站地址 `ORIGIN_URL` 和收信令牌 `INGEST_TOKEN`），启用 Email Routing 并按提示配置 DNS，将“全收”指向 Worker，最后在“域名与新建邮箱”添加后缀并保存。无需网站代理、D1 或 KV。

公开教程 `/docs/domain-setup.html` 提供代码复制和 `worker.js` 下载，下载内容由构建流程直接读取 `workers/ingest.js`。Worker 收信添加后缀无需重启；仅直连 SMTP 接收新域名需重新创建容器。

### 高级配置

域名、站点外观、API、AI、邮件规则使用各自的专用页面，不在高级配置中重复编辑。高级配置只列出支持的启动项，提供布尔下拉、整数范围和 SMTP JSON 校验；服务端也会拒绝非法值。路径、监听端口、JWT 和其他部署参数由服务器配置文件管理。

保存会同步 `.env`，页面显示实际变更中需要重新加载的启动项。执行 `docker compose up -d --force-recreate` 加载新环境变量；`docker compose restart` 不会重新读取 `env_file`。`ORIGIN_TOKEN` 已移除，Worker 收信仍需 `INGEST_TOKEN`。

### iCloud 隐藏邮箱

管理员在顶部导航「iCloud」页添加开通了 iCloud+ 的 Apple 账号（导入 icloud.com 的 Cookie，或用账号密码 + 双重认证登录），即可生成 `@icloud.com` 隐藏邮箱别名。每个别名都是普通邮箱：建好即可收信，邮件列表、SSE、Webhook、AI 提取和 Telegram 推送照常可用；别名不能发信。

**收信方式**：Apple 会把所有别名的邮件转发到账号的「转发到」地址。在「转发设置」里按引导创建一个本系统的中转邮箱（如 `relay@your-domain.com`），把它添加到 Apple 账户并验证，再设为转发地址。之后邮件经 Cloudflare 收信 Worker 进入系统，按 Apple 附加的 `X-Icloud-Hme` 头实时分到对应别名，发件人还原为原始地址。只有经 Cloudflare 通道、最上方验签结果为 `dkim=pass header.d=icloud.com`、且转发目标与账号一致的邮件才会分发，直连 25 端口的邮件不会分发；不需要 App 专用密码，也不轮询 IMAP。

**Apple 的限制**（官方未公布，以下为实测与社区数据）：

| 项目 | 情况 | 系统处理 |
|---|---|---|
| 创建频率 | 每账号一批约 5 个，恢复时间不固定（实测超过 30 分钟），被拒返回 `-41015` | 冷却并指数退避 30 → 60 → 120 → 240 分钟；实测长期平均约 4 个/小时 |
| 数量上限 | 同时存在约 750 个（社区报告 720–1047），停用的也占名额，删除后释放 | 首次被拒时记录为该账号的实测上限 |
| 地址 | 由 Apple 随机生成，不能指定 | 可自定义标签和备注 |

- **自动补满**（默认开启，可按账号关闭）：冷却结束后持续创建，直到 Apple 提示已满。没收过信的别名标记为「未使用」，「新建 iCloud 邮箱」时优先分配，不受限速影响。
- **名额回收**（默认开启）：仅在数量已满且没有未使用别名时触发，每次在 iCloud 永久删除 1 个代价最小的别名：先删已停用的，再删创建超过 30 天且从未收信的，最后删超过 90 天没收信的。标记「保留」、分配给用户、未使用、以及非本系统创建（如 iPhone 上建的）的别名永不回收。
- **会话保活**：每 30 分钟刷新一次 iCloud 会话，每 6 小时同步一次别名列表；会话失效时账号卡片显示「登录失效」，并通过 Telegram 通知全局推送名单中的管理员。已有别名的收信不依赖会话。
- 多个 Apple 账号（例如 iCloud+ 家庭共享的成员）可分别添加，创建时自动在账号间分配；每个账号需要单独登录，建议各用一个独立的中转邮箱。
- 若 iCloud+ 到期，Apple 未说明已有别名是否继续转发（用户报告不一），且无法再创建或管理别名。

接口见 `/docs/api` 的「iCloud 隐藏邮箱」一节（`/admin/hme/*`）。

### Webhook

邮件 Webhook 支持启停、HTTPS 地址、请求头、Body 模板和 HMAC 签名（`X-Tempmail-Timestamp`、`X-Tempmail-Signature`）。接收方应校验签名和时间戳，并按邮件 ID 实现幂等处理。

## Layout

```
cmd/tempmail          entrypoint (+ `-import` for D1 dumps)
internal/config       env -> Config (same variable names as upstream wrangler.toml)
internal/db           SQLite (modernc, no CGO), upstream schema + role_configs, quotas, hme_*
internal/auth         HS256 JWT, identical claims to upstream (tokens interchangeable)
internal/mailparse    MIME -> sender/subject/text/html/attachments
internal/mailer       outbound: SMTP relay (SMTP_CONFIG) or direct-to-MX
internal/inbound      SMTP receiver :25 + HTTP ingest pipeline
internal/roles        USER_ROLES (env) merged with dynamic roles (DB) + quota checks
internal/passkey      WebAuthn (go-webauthn) on /user_api/passkey/*
internal/s3store      presigned S3 URLs on /api/attachment/*
internal/telegram     Telegram bot + miniapp (initData auth) + mail push
internal/icloud       iCloud web client for Hide My Email + Apple ID SRP sign-in
internal/server       HTTP router, guards (Turnstile/IP-blacklist/rate-limit), AI extract
internal/importer     wrangler `d1 export` SQL -> SQLite (primary or merge mode)
web/app               React+Vite+Tailwind frontend -> web/dist -> embedded in binary
workers/              Cloudflare Email Routing receiver (ingest.js)
```

## Mail flow

1. Cloudflare Email Routing -> worker `tempmail-ingest` -> `POST <ORIGIN_URL>/external/ingest`
2. Direct SMTP on port 25 (point MX at the server to drop Cloudflare entirely)
3. iCloud Hide My Email aliases: Apple forwards to a relay mailbox -> path 1 -> routed to the alias by `X-Icloud-Hme`

Serve the website through an HTTPS reverse proxy to `127.0.0.1:8080` (the Go binary).
No Cloudflare website proxy Worker is required.

## Run

```bash
cp .env.example .env  # edit DOMAINS, ADMIN_PASSWORDS, JWT_SECRET and other settings
docker compose pull   # pulls ghcr.io/daimon3332/tempmail:latest
docker compose up -d
```

The root [`docker-compose.yml`](docker-compose.yml) is a complete example. Edit its image tag, ports, volumes,
and environment file as needed, then run `docker compose pull && docker compose up -d`. The published image is
available at `ghcr.io/daimon3332/tempmail`; the package must be marked **Public** in the GitHub package settings
before anonymous users can pull it. If `.env` is missing, use the optional `bash deploy.sh` helper: it creates
`.env` from `.env.example` without overwriting an existing file, pulls the image, starts Compose, and checks
`/health`.

Import an existing D1 database (once):

```bash
npx wrangler d1 export <db> --remote --output=backups/d1/main.sql
docker compose run --rm --no-deps tempmail -import /backups/d1/main.sql          # ids preserved
docker compose run --rm --no-deps tempmail -import /backups/d1/other.sql -merge  # fold in a second db
```

`workers/deploy.sh` publishes the optional Cloudflare workers separately. The container image is published by
[`.github/workflows/publish-image.yml`](.github/workflows/publish-image.yml) on every `main` push and `v*` tag.

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
| GET/POST | `/admin/hme/accounts` | iCloud accounts (cookies import, sync, forward-to, sign-in) |
| GET/POST | `/admin/hme/aliases` | list / create iCloud aliases (returns `{address,address_id,jwt}`; 429 when Apple throttles) |

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
