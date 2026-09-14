#!/usr/bin/env bash
# Deploy the email receiver. Requires wrangler login, ORIGIN_URL and INGEST_TOKEN.
set -euo pipefail
cd "$(dirname "$0")"
: "${ORIGIN_URL:?}" "${INGEST_TOKEN:?}"
npx --yes wrangler@latest deploy ingest.js --name tempmail-ingest --compatibility-date 2026-01-01 \
  --var "ORIGIN_URL:$ORIGIN_URL" --no-bundle
printf '%s' "$INGEST_TOKEN" | npx --yes wrangler@latest secret put INGEST_TOKEN --name tempmail-ingest
