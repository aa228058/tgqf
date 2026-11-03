package messages

import (
	"context"
	"database/sql"
	"errors"
	"strings"
	"time"
)

type UnifiedTask struct {
	ID string `json:"id"`
	GroupID string `json:"group_id,omitempty"`
	Type string `json:"type"`
	Title string `json:"title"`
	Status string `json:"status"`
	AccountID string `json:"account_id"`
	AccountName string `json:"account_name"`
	Attempts int `json:"attempts"`
	ScheduledAt time.Time `json:"scheduled_at"`
	LeaseUntil *time.Time `json:"lease_until,omitempty"`
	LastError string `json:"last_error,omitempty"`
	CreatedAt time.Time `json:"created_at"`
	UpdatedAt time.Time `json:"updated_at"`
}

type UnifiedTaskFilter struct {
	Status string
	Type string
	AccountID string
	Limit int
	Offset int
}

func normalizeUnifiedTaskFilter(filter UnifiedTaskFilter) (UnifiedTaskFilter, error) {
	filter.Status = strings.TrimSpace(strings.ToLower(filter.Status))
	filter.Type = strings.TrimSpace(strings.ToLower(filter.Type))
	filter.AccountID = strings.TrimSpace(filter.AccountID)
	switch filter.Type {
	case "", "message_reply", "keyword_action", "profile_update", "group_join":
	default:
		return filter, errors.New("invalid task type")
	}
	switch filter.Status {
	case "", "pending", "processing", "sent", "failed", "paused", "completed", "cancelled",
		"draft", "queued", "request_pending", "joined", "already_member", "skipped":
	default:
		return filter, errors.New("invalid task status")
	}
	if filter.AccountID != "" && !validUUID(filter.AccountID) {
		return filter, errors.New("invalid account id")
	}
	if filter.Limit < 1 || filter.Limit > 100 {
		filter.Limit = 50
	}
	if filter.Offset < 0 {
		filter.Offset = 0
	}
	return filter, nil
}

// ListUnifiedTasks provides a tenant-scoped read model over the existing
// independently leased queues. It deliberately does not duplicate queue state.
func (s *Service) ListUnifiedTasks(ctx context.Context, tenantID string, input UnifiedTaskFilter) ([]UnifiedTask, int, error) {
	filter, err := normalizeUnifiedTaskFilter(input)
	if err != nil {
		return nil, 0, err
	}
	items := []UnifiedTask{}
	total := 0
	err = withTenantTx(ctx, s.db, tenantID, func(tx *sql.Tx) error {
		const source = `
			SELECT j.id::text AS id, ''::text AS group_id, 'message_reply'::text AS task_type,
				CASE WHEN j.message_type='contact' THEN 'Send contact card' ELSE 'Reply to message' END AS title,
				j.status, j.telegram_account_id::text AS account_id,
				COALESCE(NULLIF(a.display_name,''),a.phone,'') AS account_name,
				j.attempts, j.available_at AS scheduled_at, j.lease_until, j.last_error,
				j.created_at, j.updated_at
			FROM message_reply_jobs j
			JOIN telegram_accounts a ON a.tenant_id=j.tenant_id AND a.id=j.telegram_account_id
			WHERE j.tenant_id=$1
			UNION ALL
			SELECT j.id::text, ''::text, 'keyword_action'::text, 'Keyword auto-reply',
				j.status, j.telegram_account_id::text,
				COALESCE(NULLIF(a.display_name,''),a.phone,''),
				j.attempts, j.available_at, j.lease_until, j.last_error,
				j.created_at, j.updated_at
			FROM keyword_action_jobs j
			JOIN telegram_accounts a ON a.tenant_id=j.tenant_id AND a.id=j.telegram_account_id
			WHERE j.tenant_id=$1
			UNION ALL
			SELECT j.id::text, j.batch_id::text, 'profile_update'::text, 'Update account profile',
				j.status, j.account_id::text,
				COALESCE(NULLIF(a.display_name,''),a.phone,''),
				j.attempts, j.next_run_at, j.lease_until, j.last_error,
				j.created_at, j.updated_at
			FROM account_profile_update_jobs j
			JOIN telegram_accounts a ON a.tenant_id=j.tenant_id AND a.id=j.account_id
			WHERE j.tenant_id=$1
			UNION ALL
			SELECT ja.id::text, ja.task_id::text, 'group_join'::text, jt.name,
				ja.status, ja.telegram_account_id::text,
				COALESCE(NULLIF(a.display_name,''),a.phone,''),
				ja.attempts, ja.available_at, ja.lease_until, ja.last_error,
				ja.created_at, ja.updated_at
			FROM tenant_join_task_accounts ja
			JOIN tenant_join_tasks jt ON jt.tenant_id=ja.tenant_id AND jt.id=ja.task_id
			JOIN telegram_accounts a ON a.tenant_id=ja.tenant_id AND a.id=ja.telegram_account_id
			WHERE ja.tenant_id=$1
		`
		const where = ` FROM (` + source + `) tasks
			WHERE ($2='' OR tasks.status=$2)
			  AND ($3='' OR tasks.task_type=$3)
			  AND ($4='' OR tasks.account_id=$4)`
		if err := tx.QueryRowContext(ctx, `SELECT COUNT(*)`+where, tenantID, filter.Status, filter.Type, filter.AccountID).Scan(&total); err != nil {
			return err
		}
		rows, err := tx.QueryContext(ctx, `SELECT id,group_id,task_type,title,status,account_id,account_name,
			attempts,scheduled_at,lease_until,last_error,created_at,updated_at`+where+
			` ORDER BY created_at DESC,id DESC LIMIT $5 OFFSET $6`,
			tenantID, filter.Status, filter.Type, filter.AccountID, filter.Limit, filter.Offset)
		if err != nil {
			return err
		}
		defer rows.Close()
		for rows.Next() {
			var item UnifiedTask
			if err := rows.Scan(&item.ID, &item.GroupID, &item.Type, &item.Title, &item.Status,
				&item.AccountID, &item.AccountName, &item.Attempts, &item.ScheduledAt,
				&item.LeaseUntil, &item.LastError, &item.CreatedAt, &item.UpdatedAt); err != nil {
				return err
			}
			items = append(items, item)
		}
		return rows.Err()
	})
	return items, total, err
}
