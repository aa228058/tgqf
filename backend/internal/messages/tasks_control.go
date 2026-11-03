package messages

import (
	"context"
	"database/sql"
	"errors"
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
	if taskType == "group_join" {
		var parentID string
		if err := withTenantTx(ctx, s.db, tenantID, func(tx *sql.Tx) error {
			return tx.QueryRowContext(ctx, `SELECT task_id::text FROM tenant_join_task_accounts WHERE tenant_id=$1 AND id=$2`, tenantID, id).Scan(&parentID)
		}); err != nil {
			return errors.New("join task not found")
		}
		switch action {
		case "pause":
			return s.PauseJoinTask(ctx, tenantID, parentID)
		case "resume":
			return s.ResumeJoinTask(ctx, tenantID, parentID)
		case "cancel":
			return s.CancelJoinTask(ctx, tenantID, parentID)
		case "retry":
			return errors.New("retry is managed at the join-task level")
		}
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
				if action == "retry" { status = "failed" }
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
				if action == "retry" { status = "failed" }
				query = `UPDATE keyword_action_jobs j SET status='pending',attempts=CASE WHEN $4='retry' THEN 0 ELSE attempts END,available_at=NOW(),lease_until=NULL,last_error='',updated_at=NOW() FROM tenants t,telegram_accounts a WHERE j.tenant_id=$1 AND j.id=$2::uuid AND j.status=$3 AND t.id=j.tenant_id AND t.status='active' AND t.expires_at>NOW() AND a.tenant_id=j.tenant_id AND a.id=j.telegram_account_id AND a.status='authorized' AND a.session_ciphertext IS NOT NULL`
				result, err := tx.ExecContext(ctx, query, tenantID, id, status, action)
				return checkTaskRows(result, err)
			case "cancel":
				query = `UPDATE keyword_action_jobs SET status='cancelled',lease_until=NULL,last_error='cancelled by user',updated_at=NOW() WHERE tenant_id=$1 AND id=$2::uuid AND status IN ('pending','paused','failed')`
			}
		case "profile_update":
			switch action {
			case "pause":
				query = `UPDATE account_profile_update_jobs SET status='paused',last_error='paused by user',updated_at=NOW() WHERE tenant_id=$1 AND id=$2::uuid AND status='pending'`
			case "resume", "retry":
				status := "paused"
				if action == "retry" { status = "failed" }
				query = `UPDATE account_profile_update_jobs j SET status='pending',attempts=CASE WHEN $3='retry' THEN 0 ELSE attempts END,next_run_at=NOW(),last_error='',updated_at=NOW() FROM tenants t,telegram_accounts a WHERE j.tenant_id=$1 AND j.id=$2::uuid AND j.status=$4 AND t.id=j.tenant_id AND t.status='active' AND t.expires_at>NOW() AND a.tenant_id=j.tenant_id AND a.id=j.account_id AND a.status='authorized' AND a.session_ciphertext IS NOT NULL`
				result, err := tx.ExecContext(ctx, query, tenantID, id, action, status)
				return checkTaskRows(result, err)
			case "cancel":
				query = `UPDATE account_profile_update_jobs SET status='cancelled',last_error='cancelled by user',updated_at=NOW() WHERE tenant_id=$1 AND id=$2::uuid AND status IN ('pending','paused')`
			}

		default:
			return errors.New("invalid task type")
		}
		if query == "" { return errors.New("unsupported task action") }
		result, err := tx.ExecContext(ctx, query, tenantID, id)
		return checkTaskRows(result, err)
	})
}

func checkTaskRows(result sql.Result, err error) error {
	if err != nil { return err }
	n, err := result.RowsAffected()
	if err != nil { return err }
	if n == 0 { return errors.New("task not found or current state does not allow action") }
	return nil
}

