#!/usr/bin/env bash
set -Eeuo pipefail
ROOT="$(cd "$(dirname "${BASH_SOURCE[0]}")/../.." && pwd)"
TMP="$(mktemp -d)"
trap 'rm -rf "$TMP"' EXIT
mkdir -p "$TMP/repo/deploy" "$TMP/bin" "$TMP/backup"
cp "$ROOT/deploy/restore.sh" "$TMP/repo/deploy/restore.sh"
: > "$TMP/repo/deploy/compose.yaml"
cat > "$TMP/repo/.env" <<'EOF'
POSTGRES_USER=test
POSTGRES_PASSWORD=test
POSTGRES_DB=test
REDIS_PASSWORD=test
DATABASE_URL=postgres://test:test@localhost/test
REDIS_URL=redis://:test@localhost:6379/0
EOF
printf 'dump' > "$TMP/backup/postgres.dump"
tar -czf "$TMP/backup/redis-data.tar.gz" --files-from /dev/null
cat > "$TMP/bin/docker" <<'EOF'
#!/usr/bin/env bash
printf '%s\n' "$*" >> "$DOCKER_LOG"
exit 0
EOF
chmod +x "$TMP/bin/docker"
export DOCKER_LOG="$TMP/docker.log"
export PATH="$TMP/bin:$PATH"
set +e
printf 'NO\n' | bash "$TMP/repo/deploy/restore.sh" "$TMP/backup"
status=$?
set -e
[[ "$status" -ne 0 ]] || { echo "expected restore cancellation" >&2; exit 1; }
if grep -q 'stop worker api frontend redis' "$DOCKER_LOG"; then
  echo "cancelled restore stopped production services" >&2
  exit 1
fi
echo "restore cancellation safety test passed"
