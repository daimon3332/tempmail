#!/usr/bin/env bash
# Build and start the local Docker Compose deployment.
set -euo pipefail

cd "$(dirname "$0")"

if [ ! -f .env ]; then
  cp .env.example .env
  chmod 600 .env 2>/dev/null || true
  echo "Created .env from .env.example; review it before exposing the service."
fi

command -v docker >/dev/null || { echo "docker is required" >&2; exit 1; }
docker compose version >/dev/null
mkdir -p data backups

echo "== validate compose"
docker compose config >/dev/null

echo "== build"
docker compose build --build-arg VERSION="$(date +%Y%m%d-%H%M)"

if [ ! -f data/tempmail.db ] && [ -f backups/d1/cf-mail.sql ]; then
  echo "== first run: import D1 dump"
  docker compose run --rm --no-deps tempmail -import /backups/d1/cf-mail.sql
  if [ -f backups/d1/cf-teml-mail-2.sql ]; then
    docker compose run --rm --no-deps tempmail -import /backups/d1/cf-teml-mail-2.sql -merge
  fi
fi

echo "== start"
docker compose up -d --remove-orphans
sleep 3
docker compose ps
curl -fsS http://127.0.0.1:8080/health
echo
