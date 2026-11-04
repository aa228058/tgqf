package messages

import (
	"context"
	"database/sql"
	"errors"
	"strconv"
	"strings"
)

// ChangeUnifiedTaskStatus delegates controls to the existing queue records;
// the task center never maintains a second, divergent copy of task state.
func (s *Service) ChangeUnifiedTaskStatus(ctx context.Context, tenantID, taskType, id, action string) error {
	if tenantID == "" || strings.TrimSpace(id) == "" {
		return errors.New("invalid task")
	}
	if action != "pause" && action != "resume" && action != "retry" && action != "cancel" {
		return errors.New("unsupported task action")
	}
	switch taskType {
	case "message_reply", "keyword_action", "profile_update":
		if !validUUID(id) {
			return errors.New("invalid task id")
		}
	case "group_join":
		if _, err := strconv.ParseInt(id, 10, 64); err != nil {
			return errors.New("invalid group join task id")
		}
	default:
		return errors.New("invalid task type")
	}
	if taskType == "group_join" {
		return s.changeJoinTaskAccountStatus(ctx, tenantID, id, action)
	}
	return withTenantTx(ctx, s.db, tenantID, func(tx *sql.Tx) error {
		var query string
		switch taskType {
		case "message_reply":
			switch action {
			case "pause":
				query = `UPDATE message_reply_jobs SET status='paused',lease_until=NULL,last_error='paused by user',updated_at=NOW() WHERE tenant_id=$1 AND id=$2::uuid AND status IN ('pending','failed')`
			case "cancel":
				query = `UPDATE message_reply_jobs SET status='cancelled',lease_until=NULL,last_error='cancelled by user',updated_at=NOW() WHERE tenant_id=$1 AND id=$2::uuid AND status IN ('pending','paused','failed')`
			case "resume", "retry":
				status := "paused"
				if action == "retry" {
					status = "failed"
				}
				query = `UPDATE message_reply_jobs j SET status='pending',available_at=NOW(),attempts=CASE WHEN $3='retry' THEN 0 ELSE attempts END,last_error='',updated_at=NOW() FROM tenants t,telegram_accounts a WHERE j.tenant_id=$1 AND j.id=$2::uuid AND j.status=$4 AND t.id=j.tenant_id AND t.status='active' AND t.expires_at>NOW() AND a.tenant_id=j.tenant_id AND a.id=j.telegram_account_id AND a.status='authorized' AND a.session_ciphertext IS NOT NULL`
				result, err := tx.ExecContext(ctx, query, tenantID, id, action, status)
				return checkTaskRows(result, err)
			}
		case "keyword_action":
			switch action {
			case "pause":
				query = `UPDATE keyword_action_jobs SET status='paused',lease_until=NULL,last_error='paused by user',updated_at=NOW() WHERE tenant_id=$1 AND id=$2::uuid AND status IN ('pending','failed')`
			case "resume", "retry":
				status := "paused"
				if action == "retry" {
					status = "failed"
				}
				query = `UPDATE keyword_action_jobs j SET status='pending',attempts=CASE WHEN $4='retry' THEN 0 ELSE attempts END,available_at=NOW(),lease_until=NULL,last_error='',updated_at=NOW() FROM tenants t,telegram_accounts a WHERE j.tenant_id=$1 AND j.id=$2::uuid AND j.status=$3 AND t.id=j.tenant_id AND t.status='active' AND t.expires_at>NOW() AND a.tenant_id=j.tenant_id AND a.id=j.telegram_account_id AND a.status='authorized' AND a.session_ciphertext IS NOT NULL`
				result, err := tx.ExecContext(ctx, query, tenantID, id, status, action)
				return checkTaskRows(result, err)
			case "cancel":
				query = `UPDATE keyword_action_jobs SET status='cancelled',lease_until=NULL,last_error='cancelled by user',updated_at=NOW() WHERE tenant_id=$1 AND id=$2::uuid AND status IN ('pending','paused','failed')`
			}
		case "profile_update":
			switch action {
			case "pause":
				query = `UPDATE account_profile_update_jobs SET status='paused',lease_until=NULL,last_error='paused by user',updated_at=NOW() WHERE tenant_id=$1 AND id=$2::uuid AND status='pending'`
			case "resume", "retry":
				status := "paused"
				if action == "retry" {
					status = "failed"
				}
				query = `UPDATE account_profile_update_jobs j SET status='pending',attempts=CASE WHEN $3='retry' THEN 0 ELSE attempts END,next_run_at=NOW(),lease_until=NULL,last_error='',updated_at=NOW() FROM tenants t,telegram_accounts a WHERE j.tenant_id=$1 AND j.id=$2::uuid AND j.status=$4 AND t.id=j.tenant_id AND t.status='active' AND t.expires_at>NOW() AND a.tenant_id=j.tenant_id AND a.id=j.account_id AND a.status='authorized' AND a.session_ciphertext IS NOT NULL`
				result, err := tx.ExecContext(ctx, query, tenantID, id, action, status)
				return checkTaskRows(result, err)
			case "cancel":
				query = `UPDATE account_profile_update_jobs SET status='cancelled',lease_until=NULL,last_error='cancelled by user',updated_at=NOW() WHERE tenant_id=$1 AND id=$2::uuid AND status IN ('pending','paused')`
			}
		default:
			return errors.New("invalid task type")
		}
		if query == "" {
			return errors.New("unsupported task action")
		}
		result, err := tx.ExecContext(ctx, query, tenantID, id)
		return checkTaskRows(result, err)
	})
}

// A group_join task-center row represents one account subtask, not its parent.
// Mutating the parent here used to pause/cancel every account when the user
// selected just one row, and repeated IDs from one parent produced misleading
// partial failures. Keep each requested operation scoped to its child row.
func (s *Service) changeJoinTaskAccountStatus(ctx context.Context, tenantID, rawID, action string) error {
	id, err := strconv.ParseInt(rawID, 10, 64)
	if err != nil || id < 1 {
		return errors.New("join task not found")
	}
	return withTenantTx(ctx, s.db, tenantID, func(tx *sql.Tx) error {
		var parentID string
		if err := tx.QueryRowContext(ctx, `SELECT task_id::text FROM tenant_join_task_accounts WHERE tenant_id=$1 AND id=$2`, tenantID, id).Scan(&parentID); err != nil {
			return errors.New("join task not found")
		}
		if err := lockJoinTaskParent(ctx, tx, tenantID, parentID); err != nil {
			return err
		}
		var query string
		switch action {
		case "pause":
			query = `UPDATE tenant_join_task_accounts SET
				status=CASE WHEN status='queued' THEN 'paused' ELSE status END,
				pause_requested=(status='processing'),last_error='paused by user',updated_at=NOW()
				WHERE tenant_id=$1 AND id=$2 AND status IN ('queued','processing')`
		case "resume":
			query = `UPDATE tenant_join_task_accounts ja SET
				status=CASE WHEN ja.status='paused' THEN 'queued' ELSE ja.status END,
				pause_requested=FALSE,available_at=CASE WHEN ja.status='paused' THEN NOW() ELSE ja.available_at END,
				last_error='',updated_at=NOW()
				FROM tenant_join_tasks jt,tenants t,telegram_accounts a
				WHERE ja.tenant_id=$1 AND ja.id=$2 AND ja.status IN ('paused','processing')
				AND jt.tenant_id=ja.tenant_id AND jt.id=ja.task_id AND jt.authorization_confirmed=TRUE AND jt.authorized_by IS NOT NULL AND jt.authorized_at IS NOT NULL
				AND t.id=ja.tenant_id AND t.status='active' AND t.expires_at>NOW()
				AND a.tenant_id=ja.tenant_id AND a.id=ja.telegram_account_id AND a.status='authorized' AND a.session_ciphertext IS NOT NULL`
		case "retry":
			query = `UPDATE tenant_join_task_accounts ja SET status='queued',attempts=0,available_at=NOW(),lease_until=NULL,last_error='',updated_at=NOW() FROM tenant_join_tasks jt,tenants t,telegram_accounts a WHERE ja.tenant_id=$1 AND ja.id=$2 AND ja.status='failed' AND jt.tenant_id=ja.tenant_id AND jt.id=ja.task_id AND jt.authorization_confirmed=TRUE AND jt.authorized_by IS NOT NULL AND jt.authorized_at IS NOT NULL AND t.id=ja.tenant_id AND t.status='active' AND t.expires_at>NOW() AND a.tenant_id=ja.tenant_id AND a.id=ja.telegram_account_id AND a.status='authorized' AND a.session_ciphertext IS NOT NULL`
		case "cancel":
			query = `UPDATE tenant_join_task_accounts SET
				status=CASE WHEN status='processing' THEN status ELSE 'cancelled' END,
				cancel_requested=(status='processing'),last_error='cancelled by user',updated_at=NOW()
				WHERE tenant_id=$1 AND id=$2 AND status IN ('queued','paused','processing','request_pending')`
		default:
			return errors.New("unsupported task action")
		}
		result, err := tx.ExecContext(ctx, query, tenantID, id)
		if err := checkTaskRows(result, err); err != nil {
			return err
		}
		return refreshJoinTaskControlStatus(ctx, tx, tenantID, parentID)
	})
}

// refreshJoinTaskControlStatus derives the parent state from its children.
// Pending approval remains active; paused children do not block other queued
// children. In-flight pause/cancel is recorded as a request and keeps its lease.
func deriveJoinTaskParentStatus(childStatuses []string) string {
	if len(childStatuses) == 0 {
		return "completed"
	}
	hasQueued, hasPaused, hasFailed, allCancelled := false, false, false, true
	for _, status := range childStatuses {
		switch status {
		case "queued", "processing", "request_pending":
			hasQueued = true
			allCancelled = false
		case "paused":
			hasPaused = true
			allCancelled = false
		case "failed":
			hasFailed = true
			allCancelled = false
		case "cancelled":
		case "joined", "already_member", "skipped":
			allCancelled = false
		default:
			// Unknown future states must not make the parent appear complete.
			hasQueued = true
			allCancelled = false
		}
	}
	switch {
	case hasQueued:
		return "queued"
	case hasPaused:
		return "paused"
	case hasFailed:
		return "failed"
	case allCancelled:
		return "cancelled"
	default:
		return "completed"
	}
}

func refreshJoinTaskControlStatus(ctx context.Context, tx *sql.Tx, tenantID, taskID string) error {
	rows, err := tx.QueryContext(ctx, `SELECT status FROM tenant_join_task_accounts WHERE tenant_id=$1 AND task_id=$2 ORDER BY id`, tenantID, taskID)
	if err != nil {
		return err
	}
	statuses := make([]string, 0)
	for rows.Next() {
		var status string
		if err := rows.Scan(&status); err != nil {
			rows.Close()
			return err
		}
		statuses = append(statuses, status)
	}
	if err := rows.Err(); err != nil {
		rows.Close()
		return err
	}
	if err := rows.Close(); err != nil {
		return err
	}
	parentStatus := deriveJoinTaskParentStatus(statuses)
	_, err = tx.ExecContext(ctx, `UPDATE tenant_join_tasks SET status=$3,updated_at=NOW()
		WHERE tenant_id=$1 AND id=$2 AND status NOT IN ('draft','cancelled')`, tenantID, taskID, parentStatus)
	return err
}

func checkTaskRows(result sql.Result, err error) error {
	if err != nil {
		return err
	}
	n, err := result.RowsAffected()
	if err != nil {
		return err
	}
	if n == 0 {
		return errors.New("task not found or current state does not allow action")
	}
	return nil
}
