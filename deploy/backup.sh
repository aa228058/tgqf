#!/usr/bin/env bash
set -Eeuo pipefail
umask 077
ROOT="$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)"
ENV_FILE="$ROOT/.env"
[[ -f "$ENV_FILE" ]] || { echo "Missing $ENV_FILE" >&2; exit 1; }
set -a
# shellcheck disable=SC1090
. "$ENV_FILE"
set +a
COMPOSE=(docker compose --env-file "$ENV_FILE" -f "$ROOT/deploy/compose.yaml")
BACKUP_ROOT="${BACKUP_DIR:-$ROOT/backups}"
STAMP="$(date -u +%Y%m%dT%H%M%SZ)"
DEST="$BACKUP_ROOT/$STAMP"
mkdir -p "$BACKUP_ROOT"
chmod 0700 "$BACKUP_ROOT"
mkdir -m 0700 "$DEST"
chmod 0700 "$DEST"
stopped=0
restart_stack() {
  if [[ "$stopped" == "1" ]]; then
    "${COMPOSE[@]}" start redis api worker frontend || true
  fi
}
trap restart_stack EXIT

echo "Stopping writers for a consistent Redis stream snapshot..."
stopped=1
"${COMPOSE[@]}" stop worker api frontend
"${COMPOSE[@]}" exec -T -e REDISCLI_AUTH="$REDIS_PASSWORD" redis redis-cli --no-auth-warning SAVE >/dev/null
"${COMPOSE[@]}" exec -T redis sh -c 'tar -czf /tmp/tgqf-redis-data.tar.gz -C /data .'
"${COMPOSE[@]}" stop redis

echo "Dumping PostgreSQL..."
"${COMPOSE[@]}" exec -T postgres pg_dump -U "$POSTGRES_USER" -d "$POSTGRES_DB" --format=custom > "$DEST/postgres.dump"
"${COMPOSE[@]}" cp redis:/tmp/tgqf-redis-data.tar.gz "$DEST/redis-data.tar.gz"
(
  chmod 0600 postgres.dump redis-data.tar.gz
  sha256sum postgres.dump redis-data.tar.gz > SHA256SUMS
  chmod 0600 SHA256SUMS
)
"${COMPOSE[@]}" start redis api worker frontend
stopped=0
echo "Backup complete: $DEST"
