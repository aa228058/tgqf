#!/usr/bin/env bash
set -Eeuo pipefail

# TGQF first-install / upgrade helper for Ubuntu 24.04+.
# Run from the repository root: sudo bash deploy/deploy.sh
ROOT="$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)"
cd "$ROOT"
ENV_FILE="$ROOT/.env"
COMPOSE_FILE="$ROOT/deploy/compose.yaml"
SITE_NAME="tgqf"

log(){ printf '\n\033[1;36m[TGQF]\033[0m %s\n' "$*"; }
fail(){ printf '\n[ERROR] %s\n' "$*" >&2; exit 1; }
need(){ command -v "$1" >/dev/null 2>&1 || fail "Missing command: $1"; }

[[ $EUID -eq 0 ]] || fail "Please run with sudo: sudo bash deploy/deploy.sh"
[[ -f "$COMPOSE_FILE" && -f "$ROOT/.env.example" ]] || fail "Run this script from a complete TGQF repository checkout."
need apt-get
need openssl
need sed

log "Checking / installing Ubuntu deployment dependencies"
export DEBIAN_FRONTEND=noninteractive
apt-get update
apt-get install -y ca-certificates curl git openssl nginx certbot docker.io docker-compose-v2
systemctl enable --now docker
systemctl enable --now nginx

docker compose version >/dev/null 2>&1 || fail "Docker Compose v2 plugin is unavailable."
install -d -m 0755 /var/www/certbot

FIRST_INSTALL=0
if [[ ! -f "$ENV_FILE" ]]; then
  FIRST_INSTALL=1
  cp "$ROOT/.env.example" "$ENV_FILE"
  chmod 600 "$ENV_FILE"
fi
set -a
# shellcheck disable=SC1090
. "$ENV_FILE"
set +a

set_env(){
  local key="$1" value="$2"
  if grep -qE "^#?${key}=" "$ENV_FILE"; then
    sed -i -E "s|^#?${key}=.*$|${key}=${value//|/\\|}|" "$ENV_FILE"
  else
    printf '%s=%s\n' "$key" "$value" >> "$ENV_FILE"
  fi
}
prompt(){
  local label="$1" default="$2" answer
  read -r -p "$label [$default]: " answer
  printf '%s' "${answer:-$default}"
}

log "Collecting production settings"
DOMAIN="$(prompt 'Application domain (DNS must already point to this server)' "${TGQF_DOMAIN:-tgqf.example.com}")"
[[ "$DOMAIN" =~ ^[A-Za-z0-9.-]+$ ]] || fail "Invalid domain name."
read -r -s -p "Telegram API ID (Enter keeps current value): " TG_API_ID_INPUT; printf '\\n'
read -r -s -p "Telegram API HASH (Enter keeps current value): " TG_API_HASH_INPUT; printf '\\n'
TG_API_ID="${TG_API_ID_INPUT:-${TELEGRAM_API_ID:-}}"
TG_API_HASH="${TG_API_HASH_INPUT:-${TELEGRAM_API_HASH:-}}"
[[ "$TG_API_ID" =~ ^[1-9][0-9]*$ && -n "$TG_API_HASH" ]] || fail "A valid Telegram API ID and API HASH are required by the Worker."

IMAGE_TAG="$(git -C "$ROOT" rev-parse --short=12 HEAD 2>/dev/null || date -u +%Y%m%d%H%M%S)"
if [[ "$FIRST_INSTALL" == "1" ]]; then
  ADMIN_USER="$(prompt 'Initial super-admin username' 'admin')"
  [[ "$ADMIN_USER" =~ ^[A-Za-z0-9_.-]{3,64}$ ]] || fail "Username must be 3-64 chars: letters, digits, dot, underscore or dash."
  read -r -s -p "Initial super-admin password (leave empty to generate): " ADMIN_PASSWORD; printf '\\n'
  if [[ -z "$ADMIN_PASSWORD" ]]; then ADMIN_PASSWORD="$(openssl rand -base64 24 | tr -d '\\n')"; fi
  [[ ${#ADMIN_PASSWORD} -ge 14 ]] || fail "Admin password must contain at least 14 characters."
  DB_PASSWORD="$(openssl rand -hex 24)"
  REDIS_PASSWORD="$(openssl rand -hex 24)"
  SESSION_KEY="$(openssl rand -base64 32 | tr -d '\\n')"
  set_env POSTGRES_USER tgqf_user
  set_env POSTGRES_PASSWORD "$DB_PASSWORD"
  set_env POSTGRES_DB tgqf
  set_env REDIS_PASSWORD "$REDIS_PASSWORD"
  set_env DATABASE_URL "postgres://tgqf_user:${DB_PASSWORD}@postgres:5432/tgqf?sslmode=disable"
  set_env REDIS_URL "redis://:${REDIS_PASSWORD}@redis:6379/0"
  set_env SESSION_ENCRYPTION_KEY "$SESSION_KEY"
  set_env SUPER_ADMIN_USERNAME "$ADMIN_USER"
  set_env SUPER_ADMIN_PASSWORD "$ADMIN_PASSWORD"
else
  [[ -n "${POSTGRES_PASSWORD:-}" && -n "${REDIS_PASSWORD:-}" && -n "${SESSION_ENCRYPTION_KEY:-}" ]] || fail "Existing .env is missing database/Redis/session secrets. Restore the original .env; do not regenerate them."
  ADMIN_USER="${SUPER_ADMIN_USERNAME:-}"
  ADMIN_PASSWORD=""
fi

set_env APP_ENV production
set_env TGQF_DOMAIN "$DOMAIN"
set_env TGQF_IMAGE_TAG "$IMAGE_TAG"
set_env TELEGRAM_API_ID "$TG_API_ID"
set_env TELEGRAM_API_HASH "$TG_API_HASH"
chmod 600 "$ENV_FILE"

