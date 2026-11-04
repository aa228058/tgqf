#!/usr/bin/env bash
set -Eeuo pipefail
BASE_URL="${BASE_URL:-https://example.com}"
BASE_URL="${BASE_URL%/}"
echo "Checking API readiness..."
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
