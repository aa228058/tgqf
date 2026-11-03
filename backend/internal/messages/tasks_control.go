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
	return withTenantTx(ctx, s.db, tenantID, func(tx *sql.Tx) error {
		var query string
		switch taskType {
		case "message_reply":
			switch action {
			case "pause":
				query = `UPDATE message_reply_jobs SET status='paused',lease_until=NULL,last_error='paused by user',updated_at=NOW() WHERE tenant_id=$1 AND id=$2::uuid AND status IN ('pending','failed')`
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
				query = `UPDATE keyword_action_jobs j SET status='pending',attempts=0,available_at=NOW(),lease_until=NULL,last_error='',updated_at=NOW() FROM tenants t WHERE j.tenant_id=$1 AND j.id=$2::uuid AND j.status=$3 AND t.id=j.tenant_id AND t.status='active' AND t.expires_at>NOW()`
				result, err := tx.ExecContext(ctx, query, tenantID, id, status)
				return checkTaskRows(result, err)
			case "cancel":
				return errors.New("cancellation is not supported by the keyword action queue")
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
		case "group_join":
			var parentID string
			if err := tx.QueryRowContext(ctx, `SELECT task_id::text FROM tenant_join_task_accounts WHERE tenant_id=$1 AND id=$2`, tenantID, id).Scan(&parentID); err != nil {
				return errors.New("join task not found")
			}
			switch action {
			case "pause":
				query = `UPDATE tenant_join_tasks SET status='paused',updated_at=NOW() WHERE tenant_id=$1 AND id=$2::uuid AND status='queued'`
			case "resume":
				query = `UPDATE tenant_join_tasks j SET status='queued',updated_at=NOW() FROM tenants t WHERE j.tenant_id=$1 AND j.id=$2::uuid AND j.status='paused' AND j.authorization_confirmed=TRUE AND t.id=j.tenant_id AND t.status='active' AND t.expires_at>NOW()`
				result, err := tx.ExecContext(ctx, query, tenantID, parentID)
				if err != nil { return err }
				if err = checkTaskRows(result, nil); err != nil { return err }
				_, err = tx.ExecContext(ctx, `UPDATE tenant_join_task_accounts SET status='queued',available_at=NOW(),last_error='',updated_at=NOW() WHERE tenant_id=$1 AND task_id=$2::uuid AND status='paused'`, tenantID, parentID)
				return err
			case "cancel":
				query = `UPDATE tenant_join_tasks SET status='cancelled',updated_at=NOW() WHERE tenant_id=$1 AND id=$2::uuid AND status IN ('draft','queued','paused','failed')`
				result, err := tx.ExecContext(ctx, query, tenantID, parentID)
				if err != nil { return err }
				if err = checkTaskRows(result, nil); err != nil { return err }
				_, err = tx.ExecContext(ctx, `UPDATE tenant_join_task_accounts SET status='cancelled',lease_until=NULL,last_error='cancelled by user',updated_at=NOW() WHERE tenant_id=$1 AND task_id=$2::uuid AND status IN ('queued','paused','request_pending','failed')`, tenantID, parentID)
				return err
			case "retry":
				return errors.New("retry is managed at the join-task level")
			}
			id = parentID
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

