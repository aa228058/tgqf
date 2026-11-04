package auth

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"strings"
	"time"
)

type AuditLog struct {
	ID int64 `json:"id"`
	TenantID string `json:"tenant_id,omitempty"`
	TenantName string `json:"tenant_name,omitempty"`
	ActorUserID string `json:"actor_user_id,omitempty"`
	ActorUsername string `json:"actor_username,omitempty"`
	Action string `json:"action"`
	ResourceType string `json:"resource_type"`
	ResourceID string `json:"resource_id"`
	Details json.RawMessage `json:"details"`
	CreatedAt time.Time `json:"created_at"`
}

func (s *Service) ListAuditLogs(ctx context.Context, tenantID, action string, limit, offset int) ([]AuditLog, int, error) {
	if limit < 1 || limit > 100 {
		limit = 50
	}
	if offset < 0 {
		offset = 0
	}
	if offset > 10000000 {
		return nil, 0, errors.New("audit log offset exceeds maximum")
	}
	tenantID = strings.TrimSpace(tenantID)
	action = strings.TrimSpace(action)
	var total int
	const filter = `($1='' OR l.tenant_id::text=$1) AND ($2='' OR l.action ILIKE '%'||$2||'%')`
	if err := s.db.QueryRowContext(ctx, `SELECT COUNT(*) FROM audit_logs l WHERE `+filter, tenantID, action).Scan(&total); err != nil {
		return nil, 0, err
	}
	rows, err := s.db.QueryContext(ctx, `SELECT l.id,COALESCE(l.tenant_id::text,''),COALESCE(t.name,''),
		COALESCE(l.actor_user_id::text,''),COALESCE(u.username,''),l.action,l.resource_type,l.resource_id,l.details,l.created_at
		FROM audit_logs l
		LEFT JOIN tenants t ON t.id=l.tenant_id
		LEFT JOIN users u ON u.id=l.actor_user_id
		WHERE `+filter+`
		ORDER BY l.created_at DESC,l.id DESC LIMIT $3 OFFSET $4`, tenantID, action, limit, offset)
	if err != nil {
		return nil, 0, err
	}
	defer rows.Close()
	items := make([]AuditLog, 0)
	for rows.Next() {
		var item AuditLog
		var details []byte
		if err := rows.Scan(&item.ID, &item.TenantID, &item.TenantName, &item.ActorUserID, &item.ActorUsername, &item.Action, &item.ResourceType, &item.ResourceID, &details, &item.CreatedAt); err != nil {
			return nil, 0, err
		}
		item.Details = append(json.RawMessage(nil), details...)
		items = append(items, item)
	}
	return items, total, rows.Err()
}

var _ = sql.ErrNoRows
