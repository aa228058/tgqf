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

if [[ ! -f "$ENV_FILE" ]]; then
  cp "$ROOT/.env.example" "$ENV_FILE"
  chmod 600 "$ENV_FILE"
fi

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
DOMAIN="$(prompt 'Application domain (DNS must already point to this server)' 'tgqf.example.com')"
[[ "$DOMAIN" =~ ^[A-Za-z0-9.-]+$ ]] || fail "Invalid domain name."
ADMIN_USER="$(prompt 'Initial super-admin username' 'admin')"
[[ "$ADMIN_USER" =~ ^[A-Za-z0-9_.-]{3,64}$ ]] || fail "Username must be 3-64 chars: letters, digits, dot, underscore or dash."
read -r -s -p "Telegram API ID (from my.telegram.org): " TG_API_ID; printf '\n'
read -r -s -p "Telegram API HASH (from my.telegram.org): " TG_API_HASH; printf '\n'
[[ "$TG_API_ID" =~ ^[1-9][0-9]*$ && -n "$TG_API_HASH" ]] || fail "A valid Telegram API ID and API HASH are required by the Worker."
read -r -s -p "Initial super-admin password (leave empty to generate): " ADMIN_PASSWORD; printf '\n'
if [[ -z "$ADMIN_PASSWORD" ]]; then ADMIN_PASSWORD="$(openssl rand -base64 24 | tr -d '\n')"; fi
[[ ${#ADMIN_PASSWORD} -ge 14 ]] || fail "Admin password must contain at least 14 characters."

DB_PASSWORD="$(openssl rand -hex 24)"
REDIS_PASSWORD="$(openssl rand -hex 24)"
SESSION_KEY="$(openssl rand -base64 32 | tr -d '\n')"
IMAGE_TAG="$(git -C "$ROOT" rev-parse --short=12 HEAD 2>/dev/null || date -u +%Y%m%d%H%M%S)"

set_env APP_ENV production
set_env TGQF_IMAGE_TAG "$IMAGE_TAG"
set_env POSTGRES_USER tgqf_user
set_env POSTGRES_PASSWORD "$DB_PASSWORD"
set_env POSTGRES_DB tgqf
set_env REDIS_PASSWORD "$REDIS_PASSWORD"
set_env DATABASE_URL "postgres://tgqf_user:${DB_PASSWORD}@postgres:5432/tgqf?sslmode=disable"
set_env REDIS_URL "redis://:${REDIS_PASSWORD}@redis:6379/0"
set_env SESSION_ENCRYPTION_KEY "$SESSION_KEY"
set_env TELEGRAM_API_ID "$TG_API_ID"
set_env TELEGRAM_API_HASH "$TG_API_HASH"
set_env SUPER_ADMIN_USERNAME "$ADMIN_USER"
set_env SUPER_ADMIN_PASSWORD "$ADMIN_PASSWORD"
chmod 600 "$ENV_FILE"

log "Validating Compose configuration"
docker compose --env-file "$ENV_FILE" -f "$COMPOSE_FILE" config --quiet

log "Building application images"
docker compose --env-file "$ENV_FILE" -f "$COMPOSE_FILE" build

log "Starting PostgreSQL, Redis, API, Worker and frontend"
docker compose --env-file "$ENV_FILE" -f "$COMPOSE_FILE" up -d

log "Waiting for API and frontend health checks (up to 5 minutes)"
deadline=$((SECONDS + 300))
while (( SECONDS < deadline )); do
  api_status="$(docker inspect --format '{{if .State.Health}}{{.State.Health.Status}}{{else}}{{.State.Status}}{{end}}' tgqf-api-1 2>/dev/null || true)"
  front_status="$(docker inspect --format '{{if .State.Health}}{{.State.Health.Status}}{{else}}{{.State.Status}}{{end}}' tgqf-frontend-1 2>/dev/null || true)"
  if [[ "$api_status" == healthy && "$front_status" == healthy ]]; then break; fi
  sleep 5
done
if [[ "$api_status" != healthy || "$front_status" != healthy ]]; then
  docker compose --env-file "$ENV_FILE" -f "$COMPOSE_FILE" ps
  docker compose --env-file "$ENV_FILE" -f "$COMPOSE_FILE" logs --tail=100 api frontend worker
  fail "Application did not become healthy. Fix the reported issue and rerun this script."
fi

log "Installing temporary HTTP site and requesting HTTPS certificate"
sed "s/example.com/$DOMAIN/g; s/www.$DOMAIN/$DOMAIN/g" "$ROOT/deploy/nginx/tgqf-http-bootstrap.conf" > "/etc/nginx/sites-available/$SITE_NAME"
ln -sfn "/etc/nginx/sites-available/$SITE_NAME" "/etc/nginx/sites-enabled/$SITE_NAME"
rm -f /etc/nginx/sites-enabled/default
nginx -t
systemctl reload nginx
if [[ ! -s "/etc/letsencrypt/live/$DOMAIN/fullchain.pem" ]]; then
  certbot certonly --webroot -w /var/www/certbot --non-interactive --agree-tos --register-unsafely-without-email -d "$DOMAIN" || {
    echo "Certificate request failed. Check DNS and inbound TCP/80, then rerun this script." >&2
    exit 1
  }
fi
sed "s/example.com/$DOMAIN/g; s/www.$DOMAIN/$DOMAIN/g" "$ROOT/deploy/nginx/tgqf.conf" > "/etc/nginx/sites-available/$SITE_NAME"
nginx -t
systemctl reload nginx
systemctl enable --now certbot.timer 2>/dev/null || true

log "Removing one-time bootstrap credentials from .env after super-admin creation"
set_env SUPER_ADMIN_USERNAME ""
set_env SUPER_ADMIN_PASSWORD ""
docker compose --env-file "$ENV_FILE" -f "$COMPOSE_FILE" up -d api worker

log "Running public HTTPS smoke checks"
for attempt in {1..30}; do
  if curl -fsS "https://$DOMAIN/health/ready" >/dev/null && curl -fsS "https://$DOMAIN/" >/dev/null; then break; fi
  sleep 5
done
curl -fsS "https://$DOMAIN/health/ready" >/dev/null || fail "HTTPS API readiness check failed."
curl -fsS "https://$DOMAIN/" >/dev/null || fail "HTTPS frontend check failed."

cat <<EOF

TGQF deployment completed.
URL: https://$DOMAIN
Initial super-admin: $ADMIN_USER
Initial password: $ADMIN_PASSWORD

IMPORTANT:
- Save the initial password in a password manager now; it is not stored in .env.
- Back up .env securely. It contains the Telegram session encryption key and database secrets.
- Verify Docker health and complete real Telegram test-account checks before inviting tenants.
- This script does not open firewall ports. Allow SSH, TCP 80 and TCP 443 only.
- Upgrade later by pulling the intended Git commit and rerunning: sudo bash deploy/deploy.sh
- Backup: bash deploy/backup.sh
EOF
