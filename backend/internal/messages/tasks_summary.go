package messages

import (
	"context"
	"database/sql"
	"time"
)

type UnifiedTaskCount struct {
	Type string `json:"type"`
	Status string `json:"status"`
	Count int `json:"count"`
}

type UnifiedTaskSummary struct {
	Total int `json:"total"`
	Processing int `json:"processing"`
	ExpiredLeases int `json:"expired_leases"`
	Counts []UnifiedTaskCount `json:"counts"`
	GeneratedAt time.Time `json:"generated_at"`
}

// GetUnifiedTaskSummary aggregates the existing queue tables. It does not
// create a second source of truth and always executes inside tenant RLS scope.
func (s *Service) GetUnifiedTaskSummary(ctx context.Context, tenantID string) (UnifiedTaskSummary, error) {
	summary := UnifiedTaskSummary{Counts: []UnifiedTaskCount{}}
	err := withTenantTx(ctx, s.db, tenantID, func(tx *sql.Tx) error {
		query := "SELECT task_type, status, COUNT(*)::int, " +
			"COUNT(*) FILTER (WHERE status='processing')::int, " +
			"COUNT(*) FILTER (WHERE status='processing' AND lease_until < NOW())::int " +
			"FROM ( " +
			"SELECT 'message_reply'::text AS task_type, status, lease_until FROM message_reply_jobs WHERE tenant_id=$1 " +
			"UNION ALL SELECT 'keyword_action'::text, status, lease_until FROM keyword_action_jobs WHERE tenant_id=$1 " +
			"UNION ALL SELECT 'profile_update'::text, status, lease_until FROM account_profile_update_jobs WHERE tenant_id=$1 " +
			"UNION ALL SELECT 'group_join'::text, status, lease_until FROM tenant_join_task_accounts WHERE tenant_id=$1 " +
			") task_rows GROUP BY task_type, status ORDER BY task_type, status"
		rows, err := tx.QueryContext(ctx, query, tenantID)
		if err != nil { return err }
		defer rows.Close()
		for rows.Next() {
			var item UnifiedTaskCount
			var processing, expired int
			if err := rows.Scan(&item.Type, &item.Status, &item.Count, &processing, &expired); err != nil { return err }
			summary.Counts = append(summary.Counts, item)
			summary.Total += item.Count
			summary.Processing += processing
			summary.ExpiredLeases += expired
		}
		if err := rows.Err(); err != nil { return err }
		if err := rows.Close(); err != nil { return err }
		return tx.QueryRowContext(ctx, "SELECT NOW()").Scan(&summary.GeneratedAt)
	})
	return summary, err
}
