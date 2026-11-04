# Release Audit Baseline — Phase 1

Audit date: 2026-11-05  
Repository: `aa228058/tgqf`  
Baseline branch: `main`  
Baseline commit: `272c16f2e12e712e4270ecb15be1017f34a5927c` (merge of PR #38)

## Executive result

Phase 1 established the release baseline. The formal `main` branch is the PR #38 merge commit, both required push workflows completed successfully, the previously open PR queue was reconciled, and no open PRs remain.

This is a source/CI baseline only. It is **not** a claim that every product workflow has passed real Telegram or production-server acceptance. Those checks belong to Phases 2–5.

## Main CI evidence

| Workflow | Run | Result |
|---|---:|---|
| Go backend checks | [41875421890](https://github.com/aa228058/tgqf/actions/runs/41875421890) | success |
| Frontend CI | [41875422024](https://github.com/aa228058/tgqf/actions/runs/41875422024) | success |

Both runs target `272c16f2e12e712e4270ecb15be1017f34a5927c`; each has a completed `verify` job with conclusion `success`.

## PR reconciliation

- PR #38 is merged into `main` at `272c16f2e12e712e4270ecb15be1017f34a5927c`.
- PR #39 was **closed without merge** (`merged_at=null`). Its branch contains 12 commits not in main's commit ancestry and is based on the old `frontend/phase-30-global-ux` candidate. It was not merged blindly.
- The functional changes called out by PR #39 were checked against the current main tree and are present in the current implementation:
  - `backend/internal/accounts/handler.go`: persisted device profile load for QR login, profile update pause route, profile endpoints.
  - `backend/internal/accounts/profile_updates.go`: `HasPhoto` status and profile job behavior.
  - `frontend/app/AccountsPage.tsx`: per-account profile overrides, username generation settings, avatar jobs and pause/resume/retry/cancel controls, proxy health history.
  - `frontend/app/GroupsPage.tsx`: newer request sequencing/debounced catalogue UI.
  - `frontend/app/KeywordMonitor.tsx`: expanded rule filters and ringing-only voice-call action.
- The remaining open PRs were reviewed against the current main tree. PRs #1, #3–#21, and #23–#37 were closed as superseded/stale; their required implementation or later equivalent is already present in main. PR #2 and #22 were not open in the final open-PR query.
- A final GitHub query returned **zero open PRs**.

Closing a stale PR does not mean its exact commits were merged. Where changes were superseded, the current main implementation is the release source of truth.

## Source-level implementation map for Phase 2

This is a navigation baseline, not a pass/fail functional certification.

| Area | Current main evidence | Phase 2 focus |
|---|---|---|
| Telegram runtime/device identity | `backend/cmd/worker/main.go`, `backend/internal/accounts/service.go`, `backend/internal/telegrammtgo/manager.go` | session/proxy change reconciliation, device profile persistence, reconnect and restart behavior |
| Account profile/proxy | `backend/internal/accounts/handler.go`, `backend/internal/accounts/profile_updates.go`, `frontend/app/AccountsPage.tsx` | per-account values, avatar, async job outcomes, proxy checks |
| Shared group catalogue/collector | `backend/internal/messages/service.go`, `backend/internal/messages/group_import.go`, `backend/internal/messages/group_permissions.go`, `frontend/app/GroupsPage.tsx` | tenant contributions, global deduplication, source attribution, invite validation |
| Auto-join | `backend/internal/messages/join_tasks_worker.go`, `backend/internal/messages/tasks_control.go`, join task handlers/UI | public/private invite, approval sync, in-flight lease safety, FloodWait |
| Keyword monitor/reply | `backend/internal/messages/keyword_rules.go`, `backend/internal/messages/send_outcome.go`, `frontend/app/KeywordMonitor.tsx` | filters, idempotency, uncertain sends, authorization/opt-out/rate limits |
| Message ingestion/inbox | `backend/internal/messages/incoming_stream.go`, `backend/internal/messages/outbox.go`, `backend/internal/messages/media_assets.go`, `frontend/app/MessagesPage.tsx` | durable ingestion, dedup, albums/media, send writeback, realtime reconnect |
| Voice call | `backend/internal/telegrammtgo/voice_call.go`, `docs/VOICE_CALL_IMPLEMENTATION.md` | verify outgoing ringing-only behavior with real authorized accounts; no two-way audio requirement |
| Deployment/backup | `deploy/compose.yaml`, `deploy/deploy.sh`, `deploy/backup.sh`, `deploy/restore.sh`, `deploy/README.md` | actual PostgreSQL/Redis integration, backup restore drill, rollback and Ubuntu validation |

## Product scope baseline

The current formal `README.md` states that V1 does **not** include an independent bulk-send module, page, or standalone bulk-send task. Keyword-triggered rule-based actions remain in scope. Collection and monitoring are for new messages after enablement; no historical backfill. Automated contact must respect authorization, privacy settings, opt-out, deduplication, rate limits, daily limits, cooldowns, audit logs, and Telegram FloodWait/platform restrictions.

Phase 2 must not silently treat keyword-triggered actions as a standalone bulk-send module. Any change to the formal V1 scope must be recorded explicitly before implementation.

## Explicitly not certified by Phase 1

- Real Telegram account login, message send/receive, invite checks, join approvals, contact cards, and ringing-only voice-call delivery.
- Production Ubuntu deployment, actual PostgreSQL/Redis behavior under load, backup restoration, and rollback.
- Full tenant-isolation and end-to-end business workflow acceptance.
- Mobile and desktop real-device/browser acceptance.

These are carried forward to Phases 2–5. Do not mark them complete based on source presence or CI alone.
