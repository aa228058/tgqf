# TGQF production deployment

This directory contains the production Docker Compose stack and the host Nginx HTTPS reverse proxy. The API and Worker use the same backend image; the Worker is started only after the API health check succeeds, so database migrations are applied by the API before the Worker opens storage.

## Requirements

- Ubuntu 24.04 LTS (or a supported current Ubuntu LTS)
- Docker Engine and Docker Compose v2
- Nginx and Certbot on the host
- A DNS A/AAAA record pointing the application domain to this VPS
- Telegram API ID/hash from https://my.telegram.org

Only host Nginx is public. PostgreSQL, Redis, API and Next.js ports are not exposed to the public network; API and frontend bind to 127.0.0.1.

## First deployment

From the repository root:

```bash
cp .env.example .env
chmod 600 .env
```

Generate unique secrets:

```bash
openssl rand -hex 24       # POSTGRES_PASSWORD
openssl rand -hex 24       # REDIS_PASSWORD
openssl rand -base64 32    # SESSION_ENCRYPTION_KEY
openssl rand -hex 24       # SUPER_ADMIN_PASSWORD
```

Set the generated values in `.env`. Use hexadecimal passwords for PostgreSQL/Redis so the URL values need no percent-encoding. Ensure the same password appears in the corresponding URL:

- `POSTGRES_PASSWORD` matches the password in `DATABASE_URL`.
- `REDIS_PASSWORD` matches the password in `REDIS_URL`.
- `SESSION_ENCRYPTION_KEY` is exactly 32 random bytes encoded as Base64.
- Set `TELEGRAM_API_ID` and `TELEGRAM_API_HASH`; the Worker will not start without them.
- Set `SUPER_ADMIN_USERNAME` and a unique `SUPER_ADMIN_PASSWORD` for first boot.
- Set `TGQF_IMAGE_TAG` to a unique release label (for example, the Git commit SHA).

Validate the resolved Compose configuration before starting:

```bash
docker compose --env-file .env -f deploy/compose.yaml config
docker compose --env-file .env -f deploy/compose.yaml build
docker compose --env-file .env -f deploy/compose.yaml up -d
docker compose --env-file .env -f deploy/compose.yaml ps
```

The API runs embedded SQL migrations at startup. The Worker depends on `api: service_healthy`, preventing concurrent first-boot migration attempts. Wait for API and frontend health checks; then inspect:

```bash
docker compose --env-file .env -f deploy/compose.yaml logs --tail=200 api worker frontend
curl -fsS http://127.0.0.1:8080/health/ready
curl -fsS http://127.0.0.1:3000/ >/dev/null
```

After the first successful super-admin creation, remove `SUPER_ADMIN_USERNAME` and `SUPER_ADMIN_PASSWORD` from `.env` and restart API/Worker. The existing super-admin remains in PostgreSQL; startup will not overwrite it.

## HTTPS / Nginx

Create the ACME webroot and install the temporary bootstrap site:

```bash
sudo mkdir -p /var/www/certbot
sudo cp deploy/nginx/tgqf-http-bootstrap.conf /etc/nginx/sites-available/tgqf
sudo ln -sfn /etc/nginx/sites-available/tgqf /etc/nginx/sites-enabled/tgqf
sudo nginx -t && sudo systemctl reload nginx
sudo certbot certonly --webroot -w /var/www/certbot -d example.com -d www.example.com
```

Replace `example.com` in `deploy/nginx/tgqf.conf` and certificate paths with the real domain, then switch the site:

```bash
sudo cp deploy/nginx/tgqf.conf /etc/nginx/sites-available/tgqf
sudo nginx -t && sudo systemctl reload nginx
sudo certbot renew --dry-run
```

Nginx routes `/api/` and health endpoints to Go API, all other paths to Next.js. The SSE message event endpoint disables proxy buffering and has a one-hour read timeout. Keep TLS termination at Nginx; do not expose container ports directly.

Firewall: allow SSH, TCP 80 and TCP 443 only. Do not open 3000, 5432, 6379 or 8080 publicly.

## Upgrade and rollback

Use a new immutable `TGQF_IMAGE_TAG` for every release:

```bash
# Update TGQF_IMAGE_TAG in .env, then:
docker compose --env-file .env -f deploy/compose.yaml build
docker compose --env-file .env -f deploy/compose.yaml up -d
docker compose --env-file .env -f deploy/compose.yaml ps
```

To roll back, restore the previous image tag in `.env` and run `docker compose up -d`. Database migrations are forward-only; take a database backup before upgrading and do not roll back application code across a migration that the old code cannot read.

## Backups

Run `deploy/backup.sh` from any directory. It creates a compressed PostgreSQL dump and a Redis RDB snapshot under `BACKUP_DIR` (default `./backups`). Copy backups off-host and encrypt them. The Redis snapshot is important because the incoming Telegram update stream can contain durable, not-yet-ingested events.

Restore instructions are in `deploy/restore.sh`; restore only into a stopped, isolated stack after verifying the backup.

## Smoke tests

```bash
deploy/smoke-test.sh
```

This checks API readiness, frontend HTTP response and authenticated-route protection. It does not prove Telegram login, group joining, keyword sending, voice-call delivery or media transport. Those require test Telegram accounts and explicit test authorization.

## Operational notes

- PostgreSQL is the source of truth; Redis uses AOF and a persistent volume.
- Keep `.env` mode 0600 and outside backups unless backups are encrypted.
- `SESSION_ENCRYPTION_KEY` loss makes encrypted Telegram sessions and proxy secrets unreadable. Back it up separately in a secret manager.
- The API and Worker each apply storage migrations on startup; Compose orders Worker after API readiness.
- Monitor `/health/ready`, Docker health, Worker heartbeat rows and structured logs.
- Configure host-level log rotation and off-site backups. Test restore before production use.
