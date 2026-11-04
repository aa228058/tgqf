# Phase 3 quality gates

This document records the automated release gates added for Phase 3. CI success is evidence for repository-level checks only; it is not a substitute for the isolated-host restore rehearsal or live Telegram acceptance in later phases.

## Backend and data services

- Go CI checks formatting, module tidiness, `go test ./...`, `go vet ./...`, and builds both API and Worker binaries.
- PostgreSQL 16 is a required CI service. Missing `TEST_DATABASE_URL` fails integration tests rather than silently skipping them.
- The migration integration test applies the complete embedded migration chain to an empty schema.
- PostgreSQL connection recovery is fault-injected by terminating a pooled backend connection and asserting that the pool reconnects and can execute a query.
- Worker lifecycle integration coverage records start, graceful stop, and restart against PostgreSQL, checking that the instance returns to `running`, clears `stopped_at`, and updates its process ID.
- Redis 7 is a required CI service. Stream tests cover consumer groups, pending reclaim, acknowledgement/deletion, outage/recovery, and PostgreSQL outbox publication.

## Secret and log handling

- API and Worker JSON logs use the shared `internal/safelog` handler.
- Sensitive structured field names are redacted. Common credential-bearing URLs, key/value parameters, and bearer tokens are scrubbed from strings and error messages.
- Unit tests assert that database, Redis, proxy, Telegram API hash, password, token, and bearer test values do not appear in log output.
- Frontend CI runs Gitleaks against the PR change range with full Git history available to the scanner.

## Deployment script safety

- Shell syntax and ShellCheck warning-level validation include deployment scripts and their tests.
- Backup fault injection makes Redis SAVE fail and verifies non-zero exit, private backup directory permissions, and an attempted service restart.
- Restore safety tests verify that user cancellation and checksum failure occur before any service stop.
- CI validates Compose configuration and builds the API, Worker, and frontend release images.

## Merge gate

Merge only after both Go backend and Frontend CI pass on the final pull-request head commit. A formatter automation commit does not count as a validated release candidate until those checks have run against its resulting tree.

## Boundaries carried into later phases

These tests do not claim that a production backup has been restored on an isolated server, that host-level disk/resource alerts work, or that real Telegram accounts have passed end-to-end checks. Those remain explicit deployment/operations and live-acceptance items. A CI green result must not be reported as production recovery or Telegram acceptance.
