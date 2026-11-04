#!/usr/bin/env bash
set -Eeuo pipefail
ROOT="$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)"
if [[ -f "$ROOT/.env" ]]; then
  set -a
  # shellcheck disable=SC1090
  . "$ROOT/.env"
  set +a
fi
BASE_URL="${BASE_URL:-}"
if [[ -z "$BASE_URL" && -n "${TGQF_DOMAIN:-}" ]]; then
  BASE_URL="https://$TGQF_DOMAIN"
fi
[[ -n "$BASE_URL" ]] || { echo "Set BASE_URL or TGQF_DOMAIN in .env before running smoke tests." >&2; exit 1; }
BASE_URL="${BASE_URL%/}"
echo "Checking API readiness at $BASE_URL..."
curl --fail --silent --show-error "$BASE_URL/health/ready" >/dev/null
echo "Checking frontend..."
curl --fail --silent --show-error "$BASE_URL/" >/dev/null
echo "Checking tenant API authentication..."
code="$(curl --silent --output /dev/null --write-out '%{http_code}' "$BASE_URL/api/v1/accounts")"
if [[ "$code" != "401" && "$code" != "403" ]]; then
  echo "Expected unauthenticated /api/v1/accounts to return 401/403, got $code" >&2
  exit 1
fi
echo "Smoke tests passed (API ready, frontend served, protected API rejects anonymous request)."
