package messages

import (
	"context"
	"database/sql"
	"errors"
	"strings"
	"unicode/utf8"
)

type GroupCatalogueImportRecord struct {
	ChatID int64
	ChatType string
	Title string
	Username string
	MemberCount int
	IsVerified bool
}

func normalizeGroupCatalogueImportRecord(item GroupCatalogueImportRecord) (GroupCatalogueImportRecord, error) {
	item.ChatType = strings.ToLower(strings.TrimSpace(item.ChatType))
	item.Title = strings.TrimSpace(item.Title)
	item.Username = strings.TrimPrefix(strings.TrimSpace(item.Username), "@")
	if item.ChatID == 0 || (item.ChatType != "group" && item.ChatType != "channel") {
		return item, errors.New("chat_id and chat_type=group|channel are required")
	}
	if item.Title == "" || utf8.RuneCountInString(item.Title) > 255 {
		return item, errors.New("title must contain 1 to 255 characters")
	}
	if item.MemberCount < 0 || item.MemberCount > 2147483647 {
		return item, errors.New("member_count must be between 0 and 2147483647")
	}
	if item.Username != "" {
		if len(item.Username) < 5 || len(item.Username) > 32 {
			return item, errors.New("username must contain 5 to 32 characters")
		}
		for _, r := range item.Username {
			if !((r >= 'a' && r <= 'z') || (r >= 'A' && r <= 'Z') || (r >= '0' && r <= '9') || r == '_') {
				return item, errors.New("username contains invalid characters")
			}
		}
	}
	return item, nil
}

// ImportGroupCatalogue adds new shared catalogue entries only. Existing
// Telegram identities are never overwritten by tenant CSV imports.
func (s *Service) ImportGroupCatalogue(ctx context.Context, tenantID string, records []GroupCatalogueImportRecord) (int, int, error) {
	if len(records) == 0 || len(records) > 10000 {
		return 0, 0, errors.New("import must contain between 1 and 10000 groups")
	}
	normalized := make([]GroupCatalogueImportRecord, len(records))
	for i, item := range records {
		value, err := normalizeGroupCatalogueImportRecord(item)
		if err != nil { return 0, 0, err }
		normalized[i] = value
	}
	inserted, skipped := 0, 0
	err := withTenantTx(ctx, s.db, tenantID, func(tx *sql.Tx) error {
		for _, item := range normalized {
			var groupID int64
			var wasInserted bool
			err := tx.QueryRowContext(ctx, `INSERT INTO public_groups
				(telegram_chat_id,chat_type,title,username,member_count,is_verified,first_seen_at,last_seen_at,created_at,updated_at)
				VALUES($1,$2,$3,$4,$5,$6,NOW(),NOW(),NOW(),NOW())
				ON CONFLICT(chat_type,telegram_chat_id) DO UPDATE
				SET last_seen_at=GREATEST(public_groups.last_seen_at,NOW())
				RETURNING id,(xmax=0)`,
				item.ChatID,item.ChatType,item.Title,item.Username,item.MemberCount,item.IsVerified).Scan(&groupID,&wasInserted)
			if err != nil { return err }
			if wasInserted { inserted++ } else { skipped++ }
			// Every tenant that imports an already-known shared group must still
			// receive its own contribution record; the global row is not tenant-owned.
			if _, err := tx.ExecContext(ctx, `INSERT INTO tenant_group_contributions
				(tenant_id,group_id,first_contributed_at,last_contributed_at,contribution_count)
				VALUES($1,$2,NOW(),NOW(),1)
				ON CONFLICT(tenant_id,group_id) DO UPDATE SET
				last_contributed_at=NOW(),contribution_count=tenant_group_contributions.contribution_count+1`,
				tenantID,groupID); err != nil { return err }
		}
		return nil
	})
	return inserted, skipped, err
}
