package accounts

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"time"

	"github.com/aa228058/tgqf/backend/internal/telegrammtgo"
)

type AccountGroup struct {
	ChatID int64 `json:"chat_id"`
	ChatType string `json:"chat_type"`
	Title string `json:"title"`
	Username string `json:"username"`
	MembersCount int `json:"members_count"`
	MembershipStatus string `json:"membership_status"`
	IsMember bool `json:"is_member"`
	IsAdmin bool `json:"is_admin"`
	IsCreator bool `json:"is_creator"`
	FirstSeenAt time.Time `json:"first_seen_at"`
	LastSeenAt time.Time `json:"last_seen_at"`
	LastSyncedAt time.Time `json:"last_synced_at"`
	LeftAt *time.Time `json:"left_at,omitempty"`
}

type AccountGroupSyncState struct {
	Status string `json:"status"`
	GroupCount int `json:"group_count"`
	LastStartedAt *time.Time `json:"last_started_at,omitempty"`
	LastSyncedAt *time.Time `json:"last_synced_at,omitempty"`
	LastError string `json:"last_error"`
}

func (s *Service) SaveAccountGroups(ctx context.Context, tenantID, accountID string, groups []telegrammtgo.AccountGroup) error {
	if tenantID == "" || accountID == "" { return errors.New("tenant and account are required") }
	if len(groups) > 1000 { return errors.New("group snapshot exceeds sync limit") }
	return withTenantTx(ctx, s.db, tenantID, func(tx *sql.Tx) error {
		var exists bool
		if err := tx.QueryRowContext(ctx, `SELECT EXISTS(SELECT 1 FROM telegram_accounts WHERE id=$1 AND tenant_id=$2)`, accountID, tenantID).Scan(&exists); err != nil { return err }
		if !exists { return errors.New("account not found") }
		now := time.Now().UTC()
		for _, group := range groups {
			if group.ChatID == 0 { continue }
			if group.ChatType != "group" && group.ChatType != "supergroup" && group.ChatType != "channel" { continue }
			status := "member"
			if group.IsCreator { status = "creator" } else if group.IsAdmin { status = "admin" }
			_, err := tx.ExecContext(ctx, `
				INSERT INTO telegram_account_groups
					(tenant_id,account_id,chat_id,chat_type,title,username,membership_status,is_member,first_seen_at,last_seen_at,last_synced_at,left_at)
				VALUES($1,$2,$3,$4,$5,$6,$7,TRUE,$8,$8,$8,NULL)
				ON CONFLICT(account_id,chat_id) DO UPDATE SET
					chat_type=EXCLUDED.chat_type,title=EXCLUDED.title,username=EXCLUDED.username,
					membership_status=EXCLUDED.membership_status,is_member=TRUE,
					last_seen_at=EXCLUDED.last_seen_at,last_synced_at=EXCLUDED.last_synced_at,left_at=NULL
			`, tenantID, accountID, group.ChatID, group.ChatType, group.Title, group.Username, status, now)
			if err != nil { return fmt.Errorf("save Telegram group %d: %w", group.ChatID, err) }
		}
		if _, err := tx.ExecContext(ctx, `UPDATE telegram_account_groups SET is_member=FALSE,left_at=$3,last_synced_at=$3 WHERE tenant_id=$1 AND account_id=$2 AND is_member=TRUE AND last_synced_at < $3`, tenantID, accountID, now); err != nil { return err }
		_, err := tx.ExecContext(ctx, `
			INSERT INTO telegram_account_group_sync_state(tenant_id,account_id,status,group_count,last_started_at,last_synced_at,last_error,updated_at)
			VALUES($1,$2,'succeeded',$3,$4,$4,'',$4)
			ON CONFLICT(account_id) DO UPDATE SET status='succeeded',group_count=EXCLUDED.group_count,
				last_started_at=EXCLUDED.last_started_at,last_synced_at=EXCLUDED.last_synced_at,last_error='',updated_at=EXCLUDED.updated_at
		`, tenantID, accountID, len(groups), now)
		return err
	})
}

func (s *Service) ListAccountGroups(ctx context.Context, tenantID, accountID string, includeLeft bool) ([]AccountGroup, AccountGroupSyncState, error) {
	out := []AccountGroup{}
	state := AccountGroupSyncState{Status:"never"}
	err := withTenantTx(ctx, s.db, tenantID, func(tx *sql.Tx) error {
		var exists bool
		if err := tx.QueryRowContext(ctx, `SELECT EXISTS(SELECT 1 FROM telegram_accounts WHERE id=$1 AND tenant_id=$2)`, accountID, tenantID).Scan(&exists); err != nil { return err }
		if !exists { return errors.New("account not found") }
		condition := "AND is_member=TRUE"
		if includeLeft { condition = "" }
		rows, err := tx.QueryContext(ctx, `SELECT chat_id,chat_type,title,username,membership_status,is_member,first_seen_at,last_seen_at,last_synced_at,left_at FROM telegram_account_groups WHERE tenant_id=$1 AND account_id=$2 `+condition+` ORDER BY title,chat_id`, tenantID, accountID)
		if err != nil { return err }
		defer rows.Close()
		for rows.Next() {
			var item AccountGroup
			if err := rows.Scan(&item.ChatID,&item.ChatType,&item.Title,&item.Username,&item.MembershipStatus,&item.IsMember,&item.FirstSeenAt,&item.LastSeenAt,&item.LastSyncedAt,&item.LeftAt); err != nil { return err }
			out = append(out,item)
		}
		if err := rows.Err(); err != nil { return err }
		var started, synced sql.NullTime
		err = tx.QueryRowContext(ctx, `SELECT status,group_count,last_started_at,last_synced_at,last_error FROM telegram_account_group_sync_state WHERE tenant_id=$1 AND account_id=$2`, tenantID, accountID).Scan(&state.Status,&state.GroupCount,&started,&synced,&state.LastError)
		if err != nil && err != sql.ErrNoRows { return err }
		if started.Valid { state.LastStartedAt=&started.Time }
		if synced.Valid { state.LastSyncedAt=&synced.Time }
		return nil
	})
	return out,state,err
}
