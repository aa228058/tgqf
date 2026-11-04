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

The production installer automatically creates a consistent PostgreSQL + Redis backup before replacing an existing stack. The backup is written under `backups/` with owner-only directory/file permissions. The script verifies the SHA-256 manifest, Redis archive and PostgreSQL custom dump before reporting success, and writes `BACKUP_METADATA.txt` with the release tag and recovery notes. Set `MIN_BACKUP_FREE_KB` in `.env` to enforce a minimum free-space threshold (default 0). Copy backups off-host and encrypt them; they contain tenant data and queued Telegram updates. To skip this safety step only after independently verifying a recent external backup, use `sudo TGQF_SKIP_PREDEPLOY_BACKUP=1 bash deploy/deploy.sh` for that installer invocation; a value in `.env` does not override the one-shot command-line choice.

Use a new immutable `TGQF_IMAGE_TAG` for every release:

```bash
# Update TGQF_IMAGE_TAG in .env, then:
docker compose --env-file .env -f deploy/compose.yaml build
docker compose --env-file .env -f deploy/compose.yaml up -d
docker compose --env-file .env -f deploy/compose.yaml ps
```

To roll back, restore the previous image tag in `.env` and run `docker compose up -d`. Database migrations are forward-only; take a database backup before upgrading and do not roll back application code across a migration that the old code cannot read.

## Backups

Run `deploy/backup.sh` from any directory. It creates a compressed PostgreSQL dump and a Redis data snapshot under `BACKUP_DIR` (default `./backups`). Backup directories are mode 0700 and backup files are mode 0600; keep them private. Copy backups off-host and encrypt them. The Redis snapshot is important because the incoming Telegram update stream can contain durable, not-yet-ingested events.

Restore instructions are in `deploy/restore.sh`; restore only into a stopped, isolated stack after verifying the backup. The restore command waits for PostgreSQL/API readiness through the API health check and also requires Redis, Worker and frontend containers to report healthy before reopening the stack. Run both backup and restore rehearsals against an isolated Compose project before production use.

## Smoke tests

```bash
deploy/smoke-test.sh
```

This checks API readiness, frontend HTTP response and authenticated-route protection. It does not prove Telegram login, group joining, keyword sending, voice-call delivery or media transport. Those require test Telegram accounts and explicit test authorization.

## Operational notes

- PostgreSQL is the source of truth; Redis uses AOF and a persistent volume.
- Keep `.env` mode 0600 and outside backups unless backups are encrypted.
- `SESSION_ENCRYPTION_KEY` loss makes encrypted Telegram sessions and proxy secrets unreadable. It is deliberately excluded from database/Redis backups; back it up separately in a secret manager or an encrypted off-host configuration backup, and rehearse retrieval. Never commit it to Git.
- `deploy/backup.sh` checks the resulting dump/archive and checksum manifest before restarting services. A failed backup exits non-zero and its exit trap attempts to restart stopped services.
- `deploy/tests/backup_test.sh` injects a Redis SAVE failure and checks service restart behavior; `deploy/tests/restore_test.sh` verifies cancellation occurs before any service is stopped. CI runs both.
- The API and Worker each apply storage migrations on startup; Compose orders Worker after API readiness.
- Monitor `/health/ready`, Docker health, Worker heartbeat rows and structured logs.
- Configure host-level log rotation and off-site backups. Test restore before production use.


### Restore safety checks

Before stopping the running stack, `deploy/restore.sh` verifies the optional SHA-256 manifest, validates the PostgreSQL dump and Redis archive, checks the Compose configuration and expected Redis volume, and makes sure the Alpine restore helper image is available. It then asks for the explicit `RESTORE` confirmation.

If a restore fails after the services have been stopped, the script keeps the application services and Redis stopped rather than serving a potentially partial restore. Inspect the reported error and container logs, then rerun the restore from a verified backup before reopening traffic. The `.env` file (especially `SESSION_ENCRYPTION_KEY`) is not included in database/Redis backups and must be backed up separately and securely.
