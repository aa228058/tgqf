#!/usr/bin/env bash
set -Eeuo pipefail
ROOT="$(cd "$(dirname "${BASH_SOURCE[0]}")/../.." && pwd)"
TMP="$(mktemp -d)"
trap 'rm -rf "$TMP"' EXIT
mkdir -p "$TMP/repo/deploy" "$TMP/bin"
cp "$ROOT/deploy/backup.sh" "$TMP/repo/deploy/backup.sh"
: > "$TMP/repo/deploy/compose.yaml"
cat > "$TMP/repo/.env" <<'EOF'
POSTGRES_USER=test
POSTGRES_PASSWORD=test
POSTGRES_DB=test
REDIS_PASSWORD=test
DATABASE_URL=postgres://test:test@localhost/test
REDIS_URL=redis://:test@localhost:6379/0
EOF
cat > "$TMP/bin/docker" <<'EOF'
#!/usr/bin/env bash
printf '%s\n' "$*" >> "$DOCKER_LOG"
if [[ "$*" == *"SAVE"* ]]; then
  exit 42
fi
exit 0
EOF
chmod +x "$TMP/bin/docker"
export DOCKER_LOG="$TMP/docker.log"
export PATH="$TMP/bin:$PATH"
set +e
BACKUP_DIR="$TMP/backups" bash "$TMP/repo/deploy/backup.sh"
status=$?
set -e
[[ "$status" -ne 0 ]] || { echo "expected injected Redis SAVE failure" >&2; exit 1; }
grep -q 'start redis api worker frontend' "$DOCKER_LOG" || { echo "backup failure did not attempt to restart services" >&2; exit 1; }
[[ "$(stat -c '%a' "$TMP/backups")" == "700" ]] || { echo "backup root permissions are not 0700" >&2; exit 1; }
echo "backup failure recovery test passed"
