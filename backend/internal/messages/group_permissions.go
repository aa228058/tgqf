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

// ListGroupPermissionRefreshes claims due permission snapshots for this
// tenant/account. The lease is source-scoped: one account cannot starve other
// tenants' independent account-rights snapshots for the same shared group.
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
			SELECT src.group_id
			FROM public_group_sources src
			JOIN public_groups g ON g.id=src.group_id
			WHERE src.tenant_id=$1 AND src.telegram_account_id=$2
			  AND (src.rights_next_check_at IS NULL OR src.rights_next_check_at <= NOW())
			  AND (src.rights_check_lease_until IS NULL OR src.rights_check_lease_until <= NOW())
			ORDER BY COALESCE(src.rights_next_check_at,src.account_rights_observed_at,src.last_seen_at),src.group_id
			LIMIT $3 FOR UPDATE OF src SKIP LOCKED
		), claimed AS (
			UPDATE public_group_sources src
			SET rights_check_lease_until=NOW()+INTERVAL '5 minutes'
			FROM due WHERE src.group_id=due.group_id AND src.tenant_id=$1 AND src.telegram_account_id=$2
			RETURNING src.group_id,src.rights_check_lease_until
		)
		SELECT claimed.group_id,g.telegram_chat_id,claimed.rights_check_lease_until
		FROM claimed JOIN public_groups g ON g.id=claimed.group_id
		ORDER BY claimed.group_id`, tenantID, accountID, limit)
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
// fields remain unchanged; no missing rights are inferred as allowed.
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
		if refreshErr != nil {
			result, err := tx.ExecContext(ctx, `UPDATE public_group_sources
				SET rights_check_attempts=rights_check_attempts+1,
				    rights_check_error=$5,
				    rights_next_check_at=NOW()+LEAST(INTERVAL '24 hours',INTERVAL '30 minutes' * POWER(2,LEAST(rights_check_attempts,5))),
				    rights_check_lease_until=NULL
				WHERE group_id=$1 AND tenant_id=$2 AND telegram_account_id=$3 AND rights_check_lease_until=$4`,
				item.GroupID,tenantID,accountID,item.LeaseUntil,errText)
			if err != nil { return err }
			n, _ := result.RowsAffected()
			if n == 0 { return ErrGroupPermissionLeaseLost }
			return nil
		}

		result, err := tx.ExecContext(ctx, `UPDATE public_group_sources
			SET account_rights_snapshot=COALESCE(NULLIF($5::jsonb,'null'::jsonb),account_rights_snapshot),
			    account_rights_observed_at=CASE WHEN $5::jsonb IS NULL OR $5::jsonb='null'::jsonb THEN account_rights_observed_at ELSE NOW() END,
			    rights_check_attempts=0,rights_check_error=NULL,rights_next_check_at=NOW()+INTERVAL '24 hours',
			    rights_check_lease_until=NULL
			WHERE group_id=$1 AND tenant_id=$2 AND telegram_account_id=$3 AND rights_check_lease_until=$4`,
			item.GroupID,tenantID,accountID,item.LeaseUntil,nullJSON(accountRights))
		if err != nil { return err }
		n, _ := result.RowsAffected()
		if n == 0 { return ErrGroupPermissionLeaseLost }

		_, err = tx.ExecContext(ctx, `UPDATE public_groups
			SET default_permissions=COALESCE(NULLIF($2::jsonb,'null'::jsonb),default_permissions),
			    permissions_observed_at=CASE WHEN $2::jsonb IS NULL OR $2::jsonb='null'::jsonb THEN permissions_observed_at ELSE NOW() END,
			    member_count=GREATEST(member_count,$3),
			    title=COALESCE(NULLIF($4,''),title),
			    username=COALESCE(NULLIF($5,''),username),
			    is_verified=is_verified OR $6,
			    last_seen_at=GREATEST(last_seen_at,NOW()),updated_at=NOW()
			WHERE id=$1 AND telegram_chat_id=$7`,
			item.GroupID,nullJSON(defaultPermissions),max(0,item.MemberCount),item.Title,item.Username,item.IsVerified,item.ChatID)
		if err != nil { return fmt.Errorf("update shared group permission snapshot: %w",err) }
		return nil
	})
}
