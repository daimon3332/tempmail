#!/usr/bin/env bash
# Deploy the Cloudflare bridge workers. Requires: wrangler login, and
# ORIGIN_URL / ORIGIN_TOKEN / INGEST_TOKEN in the environment.
set -euo pipefail
cd "$(dirname "$0")"
: "${ORIGIN_URL:?}" "${ORIGIN_TOKEN:?}" "${INGEST_TOKEN:?}"
for w in proxy ingest; do
  npx --yes wrangler@latest deploy "$w.js" --name "tempmail-$w" --compatibility-date 2026-01-01 \
    --var "ORIGIN_URL:$ORIGIN_URL" --no-bundle
  printf '%s' "$ORIGIN_TOKEN" | npx --yes wrangler@latest secret put ORIGIN_TOKEN --name "tempmail-$w"
done
printf '%s' "$INGEST_TOKEN" | npx --yes wrangler@latest secret put INGEST_TOKEN --name tempmail-ingest
