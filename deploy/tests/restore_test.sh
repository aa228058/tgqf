#!/usr/bin/env bash
set -Eeuo pipefail

ROOT="$(cd "$(dirname "${BASH_SOURCE[0]}")/../.." && pwd)"
TMP="$(mktemp -d)"
trap 'rm -rf "$TMP"' EXIT

mkdir -p "$TMP/repo/deploy" "$TMP/repo/backups/test" "$TMP/bin"
cp "$ROOT/deploy/restore.sh" "$TMP/repo/deploy/restore.sh"
printf 'POSTGRES_USER=test\nPOSTGRES_DB=test\nREDIS_PASSWORD=test-secret\n' > "$TMP/repo/.env"
printf 'postgres dump fixture\n' > "$TMP/repo/backups/test/postgres.dump"
mkdir -p "$TMP/redis"
printf 'redis fixture\n' > "$TMP/redis/dump.rdb"
tar -czf "$TMP/repo/backups/test/redis-data.tar.gz" -C "$TMP/redis" .
(
  cd "$TMP/repo/backups/test"
  sha256sum postgres.dump redis-data.tar.gz > SHA256SUMS
)

cat > "$TMP/bin/docker" <<'MOCK'
#!/usr/bin/env bash
printf '%s\n' "$*" >> "$DOCKER_LOG"
case " $* " in
  *" compose "*" config --quiet "*) exit 0 ;;
  *" image inspect "*) exit 0 ;;
  *" run --rm "*" pg_restore --list "*) exit 0 ;;
  *" volume inspect "*) exit 0 ;;
esac
exit 0
MOCK
chmod +x "$TMP/bin/docker"
export DOCKER_LOG="$TMP/docker.log"
export PATH="$TMP/bin:$PATH"
: > "$DOCKER_LOG"

# A user cancellation must not stop any service.
set +e
printf 'NO\n' | bash "$TMP/repo/deploy/restore.sh" "$TMP/repo/backups/test" >"$TMP/cancel.out" 2>"$TMP/cancel.err"
cancel_status=$?
set -e
if [[ "$cancel_status" -ne 0 ]]; then
  echo "Expected restore cancellation to exit cleanly." >&2
  cat "$TMP/cancel.err" >&2
  exit 1
fi
if grep -q ' stop ' "$DOCKER_LOG"; then
  echo "Restore cancellation stopped production services." >&2
  cat "$DOCKER_LOG" >&2
  exit 1
fi

# Corrupt checksums must fail before the restore confirmation or downtime.
printf 'corrupt\n' >> "$TMP/repo/backups/test/SHA256SUMS"
set +e
bash "$TMP/repo/deploy/restore.sh" "$TMP/repo/backups/test" >"$TMP/checksum.out" 2>"$TMP/checksum.err"
checksum_status=$?
set -e
if [[ "$checksum_status" -eq 0 ]]; then
  echo "Expected corrupted backup checksums to be rejected." >&2
  exit 1
fi
if grep -q ' stop ' "$DOCKER_LOG"; then
  echo "Checksum failure stopped production services." >&2
  cat "$DOCKER_LOG" >&2
  exit 1
fi

echo "Restore safety tests passed: cancellation and checksum failure caused no downtime."
