# Tempmail

Self-hosted replacement for [cloudflare_temp_email](https://github.com/dreamhunter2333/cloudflare_temp_email):
same HTTP API, same database schema, same Vue frontend — running as a single Go binary on your own server
(SQLite, no D1 read limits). Existing clients only need to keep the same `base_url`.

## Layout

```
cmd/tempmail          entrypoint (+ `-import` for D1 dumps)
internal/config       env → Config (same variable names as upstream wrangler.toml)
internal/db           SQLite (modernc, no CGO), upstream schema + role_configs
internal/auth         HS256 JWT, identical claims to upstream (tokens interchangeable)
internal/mailparse    MIME → sender/subject/text/html/attachments (postal-mime equivalent)
internal/mailer       outbound: SMTP relay (SMTP_CONFIG) or direct-to-MX
internal/inbound      SMTP receiver on :25 + HTTP ingest pipeline (blocklist, junk, forward, auto-reply, webhook)
internal/roles        USER_ROLES (env) merged with dynamic roles (DB) + quota checks
internal/server       HTTP: /api /user_api /admin /open_api /external + embedded SPA
internal/importer     wrangler `d1 export` SQL → SQLite (primary or merge mode)
web/frontend          vendored upstream frontend; web/dist is embedded into the binary
workers/              Cloudflare bridge: proxy.js (custom domain) and ingest.js (Email Routing)
```

## Mail flow

Two inbound paths feed the same pipeline:

1. **Cloudflare Email Routing → `workers/ingest.js` → `POST /external/ingest`** (current production setup;
   keeps the existing MX records, no DNS change needed).
2. **Direct SMTP on port 25** (point MX at the server to drop Cloudflare entirely).

`tempmail.daimon.dpdns.org` stays on Cloudflare via `workers/proxy.js` (Worker custom domain) which forwards
to the origin with `x-origin-token`; the origin refuses requests without it.

## Run

```bash
cp .env.example .env   # set DOMAINS, ADMIN_PASSWORDS, JWT_SECRET, ORIGIN_TOKEN, INGEST_TOKEN
docker compose up -d --build
```

Import an existing D1 database (once, before first start):

```bash
npx wrangler d1 export <db> --remote --output=backups/d1/main.sql
docker compose run --rm --no-deps tempmail -import /backups/d1/main.sql          # ids preserved
docker compose run --rm --no-deps tempmail -import /backups/d1/other.sql -merge  # fold in a second db
```

`deploy.sh` does all of the above on the server; `workers/deploy.sh` publishes the two Cloudflare workers.

## API

Everything the upstream frontend calls is implemented with the same paths, headers
(`x-admin-auth`, `x-user-token`, `x-user-access-token`, `x-custom-auth`, `Authorization: Bearer <address jwt>`)
and response shapes. Not supported: Telegram bot, passkeys, S3 attachments, Turnstile.

Additions (admin auth unless noted):

| Method | Path | Purpose |
|---|---|---|
| GET | `/health`, `/admin/health` | `{status,database,mail_receiver}` (no auth) |
| POST | `/admin/ensure_address` | create-or-reuse `{name,domain}` → `{address,address_id,jwt,reused}` |
| GET | `/admin/address/lookup?address=` | exact lookup → `{found,...}` |
| GET | `/admin/address/{id}` | detail incl. `mail_count`, `last_mail_at` |
| POST | `/admin/address/access` | `{address}` or `{address_id}` → address JWT |
| GET | `/admin/address/{id}/mails`, `/admin/address/mails?address=` | parsed items for one address (`limit,offset,after_id,after`) |
| POST | `/admin/address/{id}/archive`, `/restore`, `/recreate` | maintenance |
| DELETE | `/admin/address/{id}` | same as `/admin/delete_address/{id}` |
| GET | `/admin/stats`, `/admin/domains` | dashboard data |
| GET/POST/DELETE | `/admin/roles`, `/admin/roles/{role}` | dynamic roles: `max_address_count`, `monthly_address_quota`, `can_custom_name`, `can_send_mail`, `domains`, `prefix` |
| POST | `/external/ingest` | `{from,to,raw|raw_base64}` with `x-ingest-token` |

`GET /api/mails` and `GET /admin/mails` accept optional `after_id=` / `after=<RFC3339>` for incremental polling.

## Roles and quotas

Roles come from `USER_ROLES` (env, upstream format) and/or `role_configs` (created via `/admin/roles`,
editable at runtime). Assign with `POST /admin/user_roles {user_id, role_text}`. Enforcement happens on
`/api/new_address` (logged-in users), `/user_api/bind_address` and transfers:
`max_address_count` (-1 unlimited, 0 = global `user_settings.maxAddressCount`), `monthly_address_quota`
(-1 unlimited), `can_custom_name`, `can_send_mail`, and per-role `domains`.

## Development

```bash
go test ./...
cd web/frontend && pnpm install && npx vite build -m prod --outDir ../dist --emptyOutDir
DOMAINS='["example.com"]' JWT_SECRET=dev ADMIN_PASSWORDS='["dev"]' HTTP_ADDR=:8080 SMTP_ADDR=:2525 go run ./cmd/tempmail
```
