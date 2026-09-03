#!/usr/bin/env bash
# Deploy to the production server: sync sources + backups, build, import data
# on first run, start via docker compose.
set -euo pipefail

SERVER="${SERVER:-root@root@example.com}"
PORT="${PORT:-22}"
KEY="${KEY:-$HOME/.ssh/id_general}"
DIR="${DIR:-/opt/tempmail}"
SSH="ssh -i $KEY -p $PORT -o ConnectTimeout=20 $SERVER"

cd "$(dirname "$0")"
[ -f .env ] || { echo ".env missing (copy .env.example)"; exit 1; }

echo "== sync sources"
tar --exclude=.git --exclude=data --exclude=reference --exclude='web/frontend/node_modules' \
    --exclude='web/dist' --exclude='*.exe' --exclude='*.log' --exclude='backups' -czf - . \
  | $SSH "mkdir -p $DIR && tar -xzf - -C $DIR"

echo "== sync D1 dumps (only if not present)"
$SSH "mkdir -p $DIR/backups/d1"
for f in backups/d1/cf-mail.sql backups/d1/cf-teml-mail-2.sql; do
  if ! $SSH "test -f $DIR/$f"; then
    scp -i "$KEY" -P "$PORT" "$f" "$SERVER:$DIR/$f"
  fi
done

$SSH bash -s "$DIR" <<'EOF'
set -euo pipefail
DIR="$1"; cd "$DIR"
command -v docker >/dev/null || { curl -fsSL https://get.docker.com | sh; systemctl enable --now docker; }
docker compose version >/dev/null

echo "== build"
docker compose build --build-arg VERSION="$(date +%Y%m%d-%H%M)"

if [ ! -f data/tempmail.db ]; then
  echo "== first run: import D1 dumps"
  mkdir -p data
  docker compose run --rm --no-deps tempmail -import /backups/d1/cf-mail.sql
  docker compose run --rm --no-deps tempmail -import /backups/d1/cf-teml-mail-2.sql -merge
fi

echo "== start"
docker compose up -d --remove-orphans
sleep 3
docker compose ps
curl -fsS http://127.0.0.1:8080/health; echo
EOF
echo "== deployed"
