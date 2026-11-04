#!/usr/bin/env bash
set -Eeuo pipefail

ROOT="$(cd "$(dirname "${BASH_SOURCE[0]}")/../.." && pwd)"
TMP="$(mktemp -d)"
trap 'rm -rf "$TMP"' EXIT

mkdir -p "$TMP/repo/deploy" "$TMP/bin"
cp "$ROOT/deploy/backup.sh" "$TMP/repo/deploy/backup.sh"
printf 'POSTGRES_USER=test\nPOSTGRES_DB=test\nREDIS_PASSWORD=test-secret\n' > "$TMP/repo/.env"
chmod 600 "$TMP/repo/.env"
: > "$TMP/docker.log"

cat > "$TMP/bin/docker" <<'MOCK'
#!/usr/bin/env bash
printf '%s\n' "$*" >> "$DOCKER_LOG"
case " $* " in
  *" redis-cli --no-auth-warning SAVE "*) exit 42 ;;
  *" cp redis:/tmp/tgqf-redis-data.tar.gz "*)
    destination="${*: -1}"
    printf 'redis snapshot' > "$destination"
    ;;
  *" pg_dump "*)
    printf 'postgres dump'
    ;;
esac
exit 0
MOCK
chmod +x "$TMP/bin/docker"

export DOCKER_LOG="$TMP/docker.log"
export PATH="$TMP/bin:$PATH"
export BACKUP_DIR="$TMP/backups"

set +e
bash "$TMP/repo/deploy/backup.sh" >"$TMP/stdout" 2>"$TMP/stderr"
status=$?
set -e

if [[ "$status" -eq 0 ]]; then
  echo "Expected simulated Redis SAVE failure to fail the backup." >&2
  exit 1
fi
if ! grep -q 'start redis api worker frontend' "$DOCKER_LOG"; then
  echo "Backup failure did not attempt to restart the stopped stack." >&2
  cat "$DOCKER_LOG" >&2
  exit 1
fi
if [[ "$(stat -c '%a' "$BACKUP_DIR")" != "700" ]]; then
  echo "Backup root permissions are not 0700." >&2
  exit 1
fi

echo "Backup failure-injection test passed: failure propagated and services restart was attempted."
