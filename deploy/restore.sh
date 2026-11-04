#!/usr/bin/env bash
set -Eeuo pipefail

ROOT="$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)"
ENV_FILE="$ROOT/.env"
BACKUP_DIR="${1:-}"

fail() { echo "[restore] ERROR: $*" >&2; exit 1; }
log() { echo "[restore] $*"; }

[[ -n "$BACKUP_DIR" && -d "$BACKUP_DIR" ]] || fail "Usage: bash deploy/restore.sh /path/to/backup"
[[ -f "$ENV_FILE" ]] || fail "Missing $ENV_FILE"
[[ -f "$BACKUP_DIR/postgres.dump" && -f "$BACKUP_DIR/redis-data.tar.gz" ]] || fail "Backup files are incomplete"

set -a
# shellcheck disable=SC1090
. "$ENV_FILE"
set +a
: "${POSTGRES_USER:?POSTGRES_USER is required in .env}"
: "${POSTGRES_DB:?POSTGRES_DB is required in .env}"
: "${REDIS_PASSWORD:?REDIS_PASSWORD is required in .env}"

COMPOSE=(docker compose --env-file "$ENV_FILE" -f "$ROOT/deploy/compose.yaml")
"${COMPOSE[@]}" config --quiet || fail "Compose configuration is invalid"

# Validate both backup payloads before stopping any services.
if [[ -f "$BACKUP_DIR/SHA256SUMS" ]]; then
  (cd "$BACKUP_DIR" && sha256sum -c SHA256SUMS) || fail "Backup checksum validation failed"
fi
pg_restore --list "$BACKUP_DIR/postgres.dump" >/dev/null || fail "PostgreSQL dump is invalid"
tar -tzf "$BACKUP_DIR/redis-data.tar.gz" >/dev/null || fail "Redis archive is invalid"

# Pull the helper image before downtime; restore should not depend on a registry
# being reachable after the production services have been stopped.
if ! docker image inspect alpine:3.22 >/dev/null 2>&1; then
  log "Pulling the Redis-volume restore helper image"
  docker pull alpine:3.22
fi

docker volume inspect tgqf_redis_data >/dev/null 2>&1 || fail "Expected Docker volume tgqf_redis_data does not exist"

echo "WARNING: this replaces the current PostgreSQL database and Redis queue volume."
echo "Restore only into the intended TGQF stack. The .env file and session encryption key are not part of this backup."
read -r -p "Type RESTORE to continue: " answer
[[ "$answer" == "RESTORE" ]] || { log "Restore cancelled; no services were stopped."; exit 1; }

restore_started=0
restore_succeeded=0
on_exit() {
  status=$?
  if [[ "$restore_started" == "1" && "$restore_succeeded" != "1" ]]; then
    "${COMPOSE[@]}" stop worker api frontend redis >/dev/null 2>&1 || true
    echo "[restore] Restore did not complete. Application services and Redis have been stopped to prevent serving a partially restored database. Inspect the error, then rerun the restore or recover from a verified backup." >&2
  fi
  exit "$status"
}
trap on_exit EXIT

log "Stopping application writers and Redis"
"${COMPOSE[@]}" stop worker api frontend redis
restore_started=1

log "Restoring PostgreSQL"
"${COMPOSE[@]}" exec -T postgres pg_restore --clean --if-exists --no-owner --no-privileges -U "$POSTGRES_USER" -d "$POSTGRES_DB" < "$BACKUP_DIR/postgres.dump"

log "Restoring Redis queue and stream data"
# Compose project name is fixed to tgqf in compose.yaml.
docker run --rm -v tgqf_redis_data:/data -v "$BACKUP_DIR:/backup:ro" alpine:3.22 sh -c 'find /data -mindepth 1 -maxdepth 1 -exec rm -rf -- {} + && tar -xzf /backup/redis-data.tar.gz -C /data'

log "Starting services"
"${COMPOSE[@]}" start redis api worker frontend

log "Waiting for API and frontend health checks"
deadline=$((SECONDS + 300))
while (( SECONDS < deadline )); do
  api_id="$("${COMPOSE[@]}" ps -q api)"
  front_id="$("${COMPOSE[@]}" ps -q frontend)"
  api_status="$(docker inspect --format '{{if .State.Health}}{{.State.Health.Status}}{{else}}{{.State.Status}}{{end}}' "$api_id" 2>/dev/null || true)"
  front_status="$(docker inspect --format '{{if .State.Health}}{{.State.Health.Status}}{{else}}{{.State.Status}}{{end}}' "$front_id" 2>/dev/null || true)"
  if [[ "$api_status" == "healthy" && "$front_status" == "healthy" ]]; then
    restore_succeeded=1
    log "Restore completed and API/frontend are healthy."
    exit 0
  fi
  sleep 5
done

"${COMPOSE[@]}" ps
"${COMPOSE[@]}" logs --tail=100 api worker frontend
fail "Services did not become healthy after restore; application services remain stopped only if the failure occurred before they were started. Inspect logs before reopening traffic."
