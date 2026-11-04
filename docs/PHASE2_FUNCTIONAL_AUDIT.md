# Phase 2 — Full Functional Audit Matrix

Audit branch: `fix/tenant-inactive-task-freeze`  
Repository: `aa228058/tgqf`  
Audit date: 2026-11-05  
Reference baseline: main commit `a5ccea16dc6a946eb33bafc0c997e52764c19e2b`

## Status definitions

- **Implemented**: current code has the end-to-end API/worker/UI path and automated coverage where the behavior can be simulated.
- **Partial**: a real implementation exists, but one or more integration, operational, or UI boundaries remain.
- **Not implemented**: no production implementation was found.
- **Real-environment pending**: automated tests cannot prove Telegram's external behavior or production-server operation.

This matrix deliberately does not equate code presence with Telegram acceptance.

## Functional matrix

| Module | Current evidence | Status | Remaining acceptance |
|---|---|---|---|
| Tenant authentication and lifecycle | `backend/internal/auth`, RLS migrations, auth integration tests | Partial | Multi-tenant adversarial tests and production authorization acceptance |
| QR / phone / 2FA login | `backend/internal/accounts`, `frontend/app/AccountsPage.tsx` | Implemented in code | Real account login, restart and reconnect acceptance; pending OTP challenge is intentionally in-memory and must be restarted after API restart |
| Session import and encryption | account import handlers, session converter, encrypted session storage | Partial | Real Telethon/Pyrogram files, encrypted backup/restore and actual MTGo authorization |
| Account profile and username rules | `profile_updates.go`, `AccountsPage.tsx`, profile job migrations | Implemented in code | Real photo/profile update, conflict retry and per-account result acceptance |
| Proxy management | proxy handlers, checks, history, Worker scheduler, account proxy binding | Implemented in code | Real MTGo account connection through each supported proxy type |
| Shared global group catalogue | `public_groups`, `public_group_sources`, `tenant_group_contributions`, group import/moderation services | Implemented in code | Multi-tenant live collector soak and real Telegram metadata refresh |
| Group/link collector | `incoming_stream.go`, `links.go`, link-check Worker | Partial | Redis failure/reclaim integration, real invite/username checks and link-to-group matching edge cases |
| Auto-join | `join_tasks.go`, `join_tasks_worker.go`, approval leases and control tests, `JoinTasksPage.tsx` | Implemented in code | Real public/private invite, approval, FloodWait and reconnect acceptance |
| Keyword matching | `keyword_rules.go`, match/filter tests, `KeywordMonitor.tsx` | Implemented in code | Real incoming Telegram messages across private/group/media variants |
| Automated actions | keyword action Worker, MTGo text/contact/media/album/voice-call wrappers | Partial | Real recipient acceptance, album partial-failure and privacy/rate-limit behavior |
| Message inbox | PostgreSQL inbox, Redis Streams, SSE endpoint, `MessageEventStream.tsx`, `MessagesPage.tsx` | Partial | Redis service integration tests; V1 uses authenticated SSE with cursor recovery, not WebSocket |
| Unified task center | `tasks.go`, `tasks_control.go`, bulk control, summary, `page.tsx` | Implemented in code | Multi-worker load/failure recovery and UI real-device acceptance |
| Outgoing native voice call | `telegrammtgo/voice_call.go`, migration 0062, keyword action Worker | Partial | Only outgoing request/ringing is in V1; verify target device shows native ringing and state writeback with two authorized Telegram accounts |
| Super-admin tenant/card management | auth admin routes, `SettingsPage.tsx` | Partial | System-level config and full resource monitoring not implemented |
| Public group moderation | group moderation routes and `AdminModerationPage.tsx` | Implemented in code | Real moderation and merge audit acceptance |
| Admin audit log query | `auth/audit.go`, protected route, `AdminAuditPage.tsx` | Partial | Audit coverage still needs a full inventory of all sensitive operations |
| Workbench and runtime | `Workbench.tsx`, `RuntimePage.tsx`, Worker heartbeat | Partial | Resource metrics (CPU/memory/disk), dependency telemetry and alerting not implemented |
| Frontend responsive workflows | AppShell, account/group/message/task/keyword/join/runtime/settings pages | Partial | Browser build and actual desktop/mobile device interaction acceptance |
| gotd migration | code search for gotd imports returned zero matches; Telegram runtime uses MTGo | Implemented in source | Confirm dependency and runtime inventory again at final release |
| Independent bulk private-message sender | explicitly excluded by current V1 product scope | Out of scope | Do not count keyword-triggered actions as an independent bulk-send module |

## Phase 2 fixes in PR #42

- Tenant disable/expiry/renewal freezes pending and expired-lease jobs across replies, keyword actions, profile updates and group-join queues.
- Active Telegram RPC leases are not revoked. In-flight group-join work receives a pause request and must persist the actual RPC result before pausing.
- Re-enabling or renewing a tenant does not silently resume frozen tasks.
- Added PostgreSQL integration coverage for tenant disable, expiry, renewal, re-enable behavior, active lease preservation, audit log filtering/pagination and super-admin global-catalogue scope isolation.
- Messages UI consumes authenticated SSE events with Last-Event-ID reconnect and keeps polling as a fallback.
- Added super-admin group correction review, manual merge and audit log UI.
- Updated voice-call V1 wording to outgoing native ringing only; ringing is never reported as connected audio.

## Automated test evidence

- PostgreSQL integration suites include:
  - `backend/internal/auth/tenant_lifecycle_integration_test.go`
  - `backend/internal/messages/group_permissions_integration_test.go`
  - `backend/internal/messages/join_task_control_requests_integration_test.go`
  - `backend/internal/messages/keyword_action_integration_test.go`
  - `backend/internal/messages/reply_jobs_integration_test.go`
  - `backend/internal/messages/tasks_control_integration_test.go`
- Go backend CI passed on commit `e5b814fa0c9362d1bacdb4bd2a311c8a4ce4a7a0`; the subsequent commit adds the super-admin global-catalogue isolation test and is running in CI.
- Frontend CI passed on commit `3a8fee586459839bf5012600cc49bcc99d2363f5`; the current branch's final frontend run is still running.
- No Redis service integration test has been added yet. PostgreSQL tests do not prove Redis Streams consumer reclaim or failure recovery.

## Explicit unresolved items

1. System-wide configuration management UI/API is not implemented; do not present environment variables or secrets as editable settings.
2. Full system resource monitoring and alerting (CPU, memory, disk, Redis/PostgreSQL dependency telemetry) is not implemented; Worker heartbeat alone is not full monitoring.
3. V1 message real-time transport is SSE + Redis Streams, not WebSocket. This is documented and functional, but remains a protocol deviation from the original preference.
4. Redis integration tests for stream consumer reclaim, event retention and Redis outage recovery remain outstanding.
5. Real Telegram login, message delivery, invite approval, contact card, media album and native ringing acceptance require authorized real accounts.
6. Production server deployment, backup/restore and rollback are Phase 4/5 work.

## Phase 2 decision

The source-level functional audit and the first critical lifecycle/realtime/admin fixes are documented. Phase 2 must not be marked fully complete until the latest PR #42 Go and Frontend CI runs pass and the remaining in-scope system configuration/monitoring decisions are explicitly resolved. Real Telegram and production acceptance remain Phase 5 and are not claimed here.
