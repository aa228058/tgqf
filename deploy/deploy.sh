#!/usr/bin/env bash
set -Eeuo pipefail

# TGQF production installer/upgrader for Ubuntu 24.04+.
# First install: sudo bash deploy/deploy.sh
# Upgrade: pull the intended release, then rerun the same command.
ROOT="$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)"
cd "$ROOT"
ENV_FILE="$ROOT/.env"
COMPOSE_FILE="$ROOT/deploy/compose.yaml"
SITE_NAME="tgqf"

log(){ printf '\n[TGQF] %s\n' "$*"; }
fail(){ printf '\n[ERROR] %s\n' "$*" >&2; exit 1; }
prompt(){ local label="$1" default="$2" answer; read -r -p "$label [$default]: " answer; printf '%s' "${answer:-$default}"; }

[[ $EUID -eq 0 ]] || fail "Run with sudo: sudo bash deploy/deploy.sh"
[[ -f "$COMPOSE_FILE" && -f "$ROOT/.env.example" ]] || fail "Run from a complete TGQF repository checkout."
command -v apt-get >/dev/null 2>&1 || fail "This installer supports Ubuntu/Debian apt-based hosts."
command -v openssl >/dev/null 2>&1 || fail "openssl is required."

log "Installing Docker, Compose, Nginx and Certbot packages"
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
    # Generated secrets are hex/base64; values used here contain no sed delimiter or '&'.
    sed -i -E "s|^#?${key}=.*$|${key}=${value}|" "$ENV_FILE"
  else
    printf '%s=%s\n' "$key" "$value" >> "$ENV_FILE"
  fi
}

log "Collecting application settings"
DOMAIN="$(prompt 'Application domain (DNS A record must point to this server)' "${TGQF_DOMAIN:-tgqf.example.com}")"
[[ "$DOMAIN" =~ ^[A-Za-z0-9.-]+$ && "$DOMAIN" != *..* ]] || fail "Invalid domain name."
read -r -s -p "Telegram API ID (Enter keeps current value): " TG_API_ID_INPUT; printf '\n'
read -r -s -p "Telegram API HASH (Enter keeps current value): " TG_API_HASH_INPUT; printf '\n'
TG_API_ID="${TG_API_ID_INPUT:-${TELEGRAM_API_ID:-}}"
TG_API_HASH="${TG_API_HASH_INPUT:-${TELEGRAM_API_HASH:-}}"
[[ "$TG_API_ID" =~ ^[1-9][0-9]*$ && -n "$TG_API_HASH" ]] || fail "A valid Telegram API ID and API HASH are required."

IMAGE_TAG="$(git -C "$ROOT" rev-parse --short=12 HEAD 2>/dev/null || date -u +%Y%m%d%H%M%S)"
ADMIN_USER=""
ADMIN_PASSWORD=""
if [[ "$FIRST_INSTALL" == "1" ]]; then
  ADMIN_USER="$(prompt 'Initial super-admin username' 'admin')"
  [[ "$ADMIN_USER" =~ ^[A-Za-z0-9_.-]{3,64}$ ]] || fail "Username must be 3-64 characters (letters, digits, dot, underscore or dash)."
  read -r -s -p "Initial super-admin password (leave empty to generate): " ADMIN_PASSWORD; printf '\n'
  if [[ -z "$ADMIN_PASSWORD" ]]; then ADMIN_PASSWORD="$(openssl rand -base64 24 | tr -d '\n')"; fi
  [[ ${#ADMIN_PASSWORD} -ge 14 ]] || fail "Admin password must contain at least 14 characters."

  DB_PASSWORD="$(openssl rand -hex 24)"
  REDIS_PASSWORD="$(openssl rand -hex 24)"
  SESSION_KEY="$(openssl rand -base64 32 | tr -d '\n')"
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
fi

set_env APP_ENV production
set_env TGQF_DOMAIN "$DOMAIN"
set_env TGQF_IMAGE_TAG "$IMAGE_TAG"
set_env TELEGRAM_API_ID "$TG_API_ID"
set_env TELEGRAM_API_HASH "$TG_API_HASH"
chmod 600 "$ENV_FILE"

COMPOSE=(docker compose --env-file "$ENV_FILE" -f "$COMPOSE_FILE")
log "Validating Compose configuration"
"${COMPOSE[@]}" config --quiet

log "Building release images"
"${COMPOSE[@]}" build
log "Starting database, cache, API, Worker and frontend"
"${COMPOSE[@]}" up -d

log "Waiting up to five minutes for API and frontend health"
deadline=$((SECONDS + 300))
api_status=""
front_status=""
while (( SECONDS < deadline )); do
  api_id="$("${COMPOSE[@]}" ps -q api)"
  front_id="$("${COMPOSE[@]}" ps -q frontend)"
  api_status="$(docker inspect --format '{{if .State.Health}}{{.State.Health.Status}}{{else}}{{.State.Status}}{{end}}' "$api_id" 2>/dev/null || true)"
  front_status="$(docker inspect --format '{{if .State.Health}}{{.State.Health.Status}}{{else}}{{.State.Status}}{{end}}' "$front_id" 2>/dev/null || true)"
  [[ "$api_status" == healthy && "$front_status" == healthy ]] && break
  sleep 5
done
if [[ "$api_status" != healthy || "$front_status" != healthy ]]; then
  "${COMPOSE[@]}" ps
  "${COMPOSE[@]}" logs --tail=120 api worker frontend
  fail "Services did not become healthy. Resolve the logs above, then rerun the installer."
fi

read -r -p "Let's Encrypt notification email (optional, press Enter to skip): " CERT_EMAIL
log "Configuring HTTPS for $DOMAIN"
if [[ ! -s "/etc/letsencrypt/live/$DOMAIN/fullchain.pem" ]]; then
  sed "s/example.com/$DOMAIN/g; s/www.$DOMAIN/$DOMAIN/g" "$ROOT/deploy/nginx/tgqf-http-bootstrap.conf" > "/etc/nginx/sites-available/$SITE_NAME"
  ln -sfn "/etc/nginx/sites-available/$SITE_NAME" "/etc/nginx/sites-enabled/$SITE_NAME"
  rm -f /etc/nginx/sites-enabled/default
  nginx -t
  systemctl reload nginx
  cert_args=(certbot certonly --webroot -w /var/www/certbot --non-interactive --agree-tos -d "$DOMAIN")
  if [[ -n "$CERT_EMAIL" ]]; then cert_args+=(--email "$CERT_EMAIL"); else cert_args+=(--register-unsafely-without-email); fi
  "${cert_args[@]}" || fail "Certificate request failed. Verify DNS and inbound TCP/80, then rerun."
fi
sed "s/example.com/$DOMAIN/g; s/www.$DOMAIN/$DOMAIN/g" "$ROOT/deploy/nginx/tgqf.conf" > "/etc/nginx/sites-available/$SITE_NAME"
ln -sfn "/etc/nginx/sites-available/$SITE_NAME" "/etc/nginx/sites-enabled/$SITE_NAME"
rm -f /etc/nginx/sites-enabled/default
nginx -t
systemctl reload nginx
systemctl enable --now certbot.timer 2>/dev/null || true

if [[ "$FIRST_INSTALL" == "1" ]]; then
  log "Removing one-time super-admin bootstrap credentials from .env"
  set_env SUPER_ADMIN_USERNAME ""
  set_env SUPER_ADMIN_PASSWORD ""
  "${COMPOSE[@]}" up -d --force-recreate api worker
fi

log "Running HTTPS smoke checks"
for attempt in {1..30}; do
  if curl -fsS "https://$DOMAIN/health/ready" >/dev/null && curl -fsS "https://$DOMAIN/" >/dev/null; then break; fi
  sleep 5
done
curl -fsS "https://$DOMAIN/health/ready" >/dev/null || fail "HTTPS API readiness check failed."
curl -fsS "https://$DOMAIN/" >/dev/null || fail "HTTPS frontend check failed."

printf '\nTGQF deployment completed: https://%s\n' "$DOMAIN"
if [[ "$FIRST_INSTALL" == "1" ]]; then
  printf 'Initial super-admin: %s\nInitial password: %s\n' "$ADMIN_USER" "$ADMIN_PASSWORD"
  printf 'Save this password in a password manager now; it is not retained in .env.\n'
else
  printf 'Existing super-admin and production secrets were preserved.\n'
fi
cat <<'EOF'

IMPORTANT:
- Back up .env securely; it contains the session encryption key and infrastructure secrets.
- This script does not open your cloud firewall. Allow SSH, TCP 80 and TCP 443 only.
- Do not delete or regenerate .env on an existing installation.
- Complete real Telegram test-account checks before inviting tenants.
- Back up: bash deploy/backup.sh
EOF
