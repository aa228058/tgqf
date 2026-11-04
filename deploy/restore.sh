#!/usr/bin/env bash
set -Eeuo pipefail
ROOT="$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)"
ENV_FILE="$ROOT/.env"
BACKUP_DIR="${1:-}"
[[ -n "$BACKUP_DIR" && -d "$BACKUP_DIR" ]] || { echo "Usage: bash deploy/restore.sh /path/to/backup" >&2; exit 2; }
[[ -f "$ENV_FILE" ]] || { echo "Missing $ENV_FILE" >&2; exit 1; }
set -a
# shellcheck disable=SC1090
. "$ENV_FILE"
set +a
COMPOSE=(docker compose --env-file "$ENV_FILE" -f "$ROOT/deploy/compose.yaml")
[[ -f "$BACKUP_DIR/postgres.dump" && -f "$BACKUP_DIR/redis-data.tar.gz" ]] || { echo "Backup files are incomplete" >&2; exit 1; }
if [[ -f "$BACKUP_DIR/SHA256SUMS" ]]; then (cd "$BACKUP_DIR" && sha256sum -c SHA256SUMS); fi
echo "WARNING: this replaces the current PostgreSQL database and Redis queue volume."
read -r -p "Type RESTORE to continue: " answer
[[ "$answer" == "RESTORE" ]] || { echo "Restore cancelled"; exit 1; }

"${COMPOSE[@]}" stop worker api frontend redis
"${COMPOSE[@]}" exec -T postgres pg_restore --clean --if-exists --no-owner --no-privileges -U "$POSTGRES_USER" -d "$POSTGRES_DB" < "$BACKUP_DIR/postgres.dump"
# Compose project name is fixed to tgqf in compose.yaml.
docker run --rm -v tgqf_redis_data:/data -v "$BACKUP_DIR:/backup:ro" alpine:3.22 sh -c 'rm -rf /data/* && tar -xzf /backup/redis-data.tar.gz -C /data'
"${COMPOSE[@]}" start redis api worker frontend
echo "Restore complete. Verify Docker health and application smoke tests before reopening traffic."
