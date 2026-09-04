# Tempmail

Self-hosted replacement for [cloudflare_temp_email](https://github.com/dreamhunter2333/cloudflare_temp_email):
same HTTP API, same database schema — running as a single Go binary on your own server (SQLite, no D1 read
limits). New React frontend (DuckMail-style mail client + Mailbucket-style admin dashboard).
Existing clients keep the same `base_url` and only need to swap the domain.

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
