# Production server deployment checklist

This checklist is for the operator preparing the production VPS. Never put real credentials, session data, or backup payloads in GitHub Issues or this repository.

## Host and network

- [ ] Ubuntu 24.04 LTS or another supported Ubuntu LTS.
- [ ] Docker Engine and Compose v2 installed and enabled.
- [ ] Nginx and Certbot installed.
- [ ] DNS A/AAAA records point to this server and have propagated.
- [ ] Firewall permits SSH, TCP 80 and TCP 443 only.
- [ ] Ports 3000, 5432, 6379 and 8080 are not exposed publicly.
- [ ] Host has sufficient free disk space for images, PostgreSQL/Redis volumes and backups.
- [ ] Configure host-level log rotation and monitoring for Docker daemon, disk, memory and filesystem inode exhaustion.

## Secrets and environment

- [ ] Create .env from .env.example; set file mode to 0600.
- [ ] Generate unique PostgreSQL and Redis passwords; ensure URLs match the individual passwords.
- [ ] Generate SESSION_ENCRYPTION_KEY as exactly 32 random bytes encoded in Base64.
- [ ] Store .env and SESSION_ENCRYPTION_KEY in an approved secret manager or encrypted off-host vault.
- [ ] Store Telegram API ID/hash securely; never expose them in frontend settings or logs.
- [ ] Use a unique initial super-admin credential; remove bootstrap username/password after first successful creation.
- [ ] Set immutable TGQF_IMAGE_TAG for every release.
- [ ] Set MIN_DEPLOY_FREE_KB; optionally set MIN_BACKUP_FREE_KB based on expected database/Redis size.
- [ ] Verify .env and all backup files are excluded from Git and public artifact storage.

## First install and upgrade

- [ ] Validate Compose configuration before starting.
- [ ] On upgrade, create and verify a pre-deploy backup before replacing images.
- [ ] Record the deployed Git commit/image tag and migration version.
- [ ] Wait for PostgreSQL, Redis, API, Worker and frontend health checks.
- [ ] Verify HTTPS certificate and /health/ready.
- [ ] Review API/Worker/frontend logs for startup errors and migration failures.
- [ ] Confirm the Worker heartbeat is fresh and Redis queues are being consumed.
- [ ] Confirm system monitoring and alert history work for a super-admin session.


## Upgrade rollback procedure

The installer creates and verifies a pre-upgrade backup, but it does not automatically downgrade a database after a forward migration. Do not blindly run an older binary against a newer schema.

1. Take the affected site out of service at the reverse proxy/load balancer and stop application writers: `docker compose --env-file .env -f deploy/compose.yaml stop worker api frontend`.
2. Retrieve the verified pre-upgrade backup and the matching previous release image tag from `BACKUP_METADATA.txt`. Confirm the encrypted off-host `.env`/session key backup is available.
3. Edit only `TGQF_IMAGE_TAG` in the live `.env` to the previous immutable tag. Do not regenerate PostgreSQL, Redis, Telegram API, or session-encryption secrets.
4. Ensure the previous backend/frontend images are present locally (or pull the exact immutable tags while the site remains offline). Create the old-version app containers without starting them: `docker compose --env-file .env -f deploy/compose.yaml create api worker frontend`.
5. Run `bash deploy/restore.sh /absolute/path/to/verified-backup`. It validates payloads before asking for confirmation, restores PostgreSQL and Redis, then starts the services and waits for API, frontend, Worker and Redis health.
6. Verify `/health/ready`, Worker heartbeat, tenant/account counts, queues and audit records. Reopen traffic only after the operator signs off.
7. Record the failed release tag, restored release tag, backup identifier, migration version, recovery time and data-loss window.

If the previous immutable images are unavailable, keep the site offline and retrieve them before proceeding. Never restore production data into an unisolated test stack. The recovery script is intentionally fail-closed: an incomplete restore leaves application services stopped for operator investigation.

## Backup and restore

- [ ] Run bash deploy/backup.sh; verify SHA256SUMS, BACKUP_METADATA.txt, PostgreSQL dump and Redis archive.
- [ ] Copy the backup to off-host encrypted storage; test retrieval.
- [ ] Back up .env and SESSION_ENCRYPTION_KEY separately in encrypted secret storage; they are intentionally not in the DB/Redis archive.
- [ ] In an isolated environment, restore PostgreSQL and Redis using deploy/restore.sh.
- [ ] Verify API, frontend, Worker and Redis all become healthy after restore.
- [ ] Compare tenant/account counts and verify the Telegram update stream/outbox is recoverable.
- [ ] Record restore duration, backup age, data-loss window and any manual steps.

## Telegram acceptance (authorized test accounts only)

- [ ] Verify login and encrypted Session persistence/restart recovery.
- [ ] Verify message receive/send, media/contact cards, keyword actions and delivery result write-back.
- [ ] Verify shared group catalogue contribution/source attribution and tenant isolation.
- [ ] Verify public/private invite joins, approvals, FloodWait cooldown and failure reporting.
- [ ] Verify native voice call initiation produces target-side ringing; do not mark unanswered calls as connected.
- [ ] Verify profile updates, retries and task cancellation.
- [ ] Verify desktop/mobile admin flows and realtime reconnect behavior.

## Release record

Record release tag, Git commit, schema migration version, host OS, Docker/Compose versions, backup identifier, restore rehearsal date, rollback image tag, known limitations and the operator who approved the release.
