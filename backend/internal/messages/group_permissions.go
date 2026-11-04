package messages

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"time"
)

var ErrGroupPermissionLeaseLost = errors.New("group permission refresh lease expired or was superseded")

type GroupPermissionRefresh struct {
	GroupID int64
	ChatID int64
	LeaseUntil time.Time
	Title string
	Username string
	MemberCount int
	IsVerified bool
}

// ListGroupPermissionRefreshes claims stale shared-group permission snapshots
// using a database lease. A group remains globally shared; the tenant/account
// only identifies the Telegram identity used for this read-only refresh.
func (s *Service) ListGroupPermissionRefreshes(ctx context.Context, tenantID, accountID string, limit int) ([]GroupPermissionRefresh, error) {
	if !validUUID(accountID) {
		return nil, errors.New("invalid Telegram account id")
	}
	if limit < 1 || limit > 25 {
		limit = 5
	}
	out := []GroupPermissionRefresh{}
	err := withTenantTx(ctx, s.db, tenantID, func(tx *sql.Tx) error {
		rows, err := tx.QueryContext(ctx, `WITH due AS (
			SELECT g.id
			FROM public_groups g
			WHERE (g.permissions_next_check_at IS NULL OR g.permissions_next_check_at <= NOW())
			  AND (g.permissions_check_lease_until IS NULL OR g.permissions_check_lease_until <= NOW())
			  AND EXISTS (
				SELECT 1 FROM public_group_sources src
				WHERE src.group_id=g.id AND src.tenant_id=$1 AND src.telegram_account_id=$2
			  )
			ORDER BY COALESCE(g.permissions_next_check_at,g.permissions_observed_at,g.last_seen_at),g.id
			LIMIT $3 FOR UPDATE OF g SKIP LOCKED
		), claimed AS (
			UPDATE public_groups g
			SET permissions_check_lease_until=NOW()+INTERVAL '5 minutes',
			    permissions_check_tenant_id=$1,
			    permissions_check_account_id=$2
			FROM due WHERE g.id=due.id
			RETURNING g.id,g.telegram_chat_id,g.permissions_check_lease_until
		)
		SELECT id,telegram_chat_id,permissions_check_lease_until FROM claimed ORDER BY id`, tenantID, accountID, limit)
		if err != nil { return err }
		defer rows.Close()
		for rows.Next() {
			var item GroupPermissionRefresh
			if err := rows.Scan(&item.GroupID,&item.ChatID,&item.LeaseUntil); err != nil { return err }
			out = append(out,item)
		}
		return rows.Err()
	})
	return out, err
}

// RecordGroupPermissionRefresh persists only observed values. Unknown permission
// fields remain NULL/unchanged; no missing rights are inferred as allowed.
func (s *Service) RecordGroupPermissionRefresh(ctx context.Context, tenantID, accountID string, item GroupPermissionRefresh, defaultPermissions, accountRights []byte, refreshErr error) error {
	if !validUUID(accountID) || item.GroupID <= 0 || item.ChatID == 0 || item.LeaseUntil.IsZero() {
		return errors.New("invalid group permission refresh result")
	}
	if len(defaultPermissions) > 0 && !json.Valid(defaultPermissions) {
		return errors.New("invalid default permission snapshot")
	}
	if len(accountRights) > 0 && !json.Valid(accountRights) {
		return errors.New("invalid account permission snapshot")
	}
	errText := ""
	if refreshErr != nil {
		errText = refreshErr.Error()
		if len(errText) > 500 { errText = errText[:500] }
	}
	return withTenantTx(ctx, s.db, tenantID, func(tx *sql.Tx) error {
		var result sql.Result
		var err error
		if refreshErr != nil {
			result, err = tx.ExecContext(ctx, `UPDATE public_groups
				SET permissions_check_attempts=permissions_check_attempts+1,
				    permissions_check_error=$5,
				    permissions_next_check_at=NOW()+LEAST(INTERVAL '24 hours',INTERVAL '30 minutes' * POWER(2,LEAST(permissions_check_attempts,5))),
				    permissions_check_lease_until=NULL,permissions_check_tenant_id=NULL,permissions_check_account_id=NULL
				WHERE id=$1 AND permissions_check_tenant_id=$2 AND permissions_check_account_id=$3 AND permissions_check_lease_until=$4`,
				item.GroupID,tenantID,accountID,item.LeaseUntil,errText)
		} else {
			result, err = tx.ExecContext(ctx, `UPDATE public_groups
				SET default_permissions=COALESCE(NULLIF($5::jsonb,'null'::jsonb),default_permissions),
				    permissions_observed_at=CASE WHEN $5::jsonb IS NULL OR $5::jsonb='null'::jsonb THEN permissions_observed_at ELSE NOW() END,
				    permissions_check_attempts=0,permissions_check_error=NULL,permissions_next_check_at=NOW()+INTERVAL '24 hours',
				    permissions_check_lease_until=NULL,permissions_check_tenant_id=NULL,permissions_check_account_id=NULL,
				    member_count=GREATEST(member_count,$6),
				    title=COALESCE(NULLIF($7,''),title),
				    username=COALESCE(NULLIF($8,''),username),
				    is_verified=is_verified OR $9
				WHERE id=$1 AND permissions_check_tenant_id=$2 AND permissions_check_account_id=$3 AND permissions_check_lease_until=$4`,
				item.GroupID,tenantID,accountID,item.LeaseUntil,nullJSON(defaultPermissions),max(0,item.MemberCount),item.Title,item.Username,item.IsVerified)
			if err != nil { return fmt.Errorf("store tenant account rights snapshot: %w",err) }
		}
		return nil
	})
}
