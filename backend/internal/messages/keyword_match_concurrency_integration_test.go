package messages

import (
	"context"
	"sync"
	"testing"
	"time"
)

func TestConcurrentIncomingUpdateCreatesOneKeywordMatchAndAction(t *testing.T) {
	db, cleanup := openGroupIntegrationDB(t)
	defer cleanup()
	ctx := context.Background()
	tenantID := "12121212-1212-4212-8212-121212121212"
	accountID := "abababab-abab-4bab-8bab-abababababab"
	if _, err := db.ExecContext(ctx, `INSERT INTO tenants(id,status,expires_at) VALUES($1,'active',NOW()+INTERVAL '30 days')`, tenantID); err != nil {
		t.Fatal(err)
	}
	if _, err := db.ExecContext(ctx, `INSERT INTO telegram_accounts(tenant_id,id,status,session_ciphertext) VALUES($1,$2,'authorized',decode('01','hex'))`, tenantID, accountID); err != nil {
		t.Fatal(err)
	}

	// Keep this integration fixture self-contained while matching the columns
	// used by the production message/keyword migrations.
	ddl := []string{
		`CREATE TABLE tenant_message_settings (tenant_id UUID PRIMARY KEY, settings JSONB NOT NULL, updated_at TIMESTAMPTZ NOT NULL DEFAULT NOW())`,
		`CREATE TABLE tenant_message_chats (
			id UUID PRIMARY KEY DEFAULT gen_random_uuid(), tenant_id UUID NOT NULL, telegram_account_id UUID NOT NULL,
			chat_id BIGINT NOT NULL, chat_title TEXT NOT NULL DEFAULT '', chat_type TEXT NOT NULL,
			username TEXT NOT NULL DEFAULT '', member_count INTEGER NOT NULL DEFAULT 0, is_verified BOOLEAN NOT NULL DEFAULT FALSE,
			photo_id TEXT NOT NULL DEFAULT '', last_seen_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
			created_at TIMESTAMPTZ NOT NULL DEFAULT NOW(), updated_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
			UNIQUE(tenant_id,telegram_account_id,chat_id))`,
		`CREATE TABLE unified_messages (
			id UUID PRIMARY KEY DEFAULT gen_random_uuid(), tenant_id UUID NOT NULL, telegram_account_id UUID NOT NULL,
			chat_id BIGINT NOT NULL, message_id BIGINT NOT NULL, chat_type TEXT NOT NULL, chat_title TEXT NOT NULL DEFAULT '',
			sender_id BIGINT NOT NULL DEFAULT 0, reply_to_message_id BIGINT, reply_to_own_message BOOLEAN NOT NULL DEFAULT FALSE,
			is_outgoing BOOLEAN NOT NULL DEFAULT FALSE, message_text TEXT NOT NULL DEFAULT '', media_type TEXT NOT NULL DEFAULT '',
			media_metadata JSONB NOT NULL DEFAULT '{}'::jsonb, grouped_id BIGINT, message_date TIMESTAMPTZ NOT NULL,
			processed_at TIMESTAMPTZ, created_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
			UNIQUE(telegram_account_id,chat_id,message_id))`,
		`CREATE TABLE telegram_incoming_update_dedup (
			tenant_id UUID NOT NULL, telegram_account_id UUID NOT NULL, chat_id BIGINT NOT NULL, message_id BIGINT NOT NULL,
			first_received_at TIMESTAMPTZ NOT NULL DEFAULT NOW(), PRIMARY KEY(tenant_id,telegram_account_id,chat_id,message_id))`,
		`CREATE TABLE tenant_keyword_rules (
			id UUID PRIMARY KEY DEFAULT gen_random_uuid(), tenant_id UUID NOT NULL, name TEXT NOT NULL, match_mode TEXT NOT NULL,
			keywords JSONB NOT NULL DEFAULT '[]'::jsonb, exclude_terms JSONB NOT NULL DEFAULT '[]'::jsonb,
			chat_types JSONB NOT NULL DEFAULT '[]'::jsonb, account_ids JSONB NOT NULL DEFAULT '[]'::jsonb,
			chat_ids JSONB NOT NULL DEFAULT '[]'::jsonb, media_types JSONB NOT NULL DEFAULT '[]'::jsonb,
			chat_titles JSONB NOT NULL DEFAULT '[]'::jsonb, chat_usernames JSONB NOT NULL DEFAULT '[]'::jsonb,
			sender_ids JSONB NOT NULL DEFAULT '[]'::jsonb, sender_usernames JSONB NOT NULL DEFAULT '[]'::jsonb,
			sender_names JSONB NOT NULL DEFAULT '[]'::jsonb, keyword_groups JSONB NOT NULL DEFAULT '[]'::jsonb,
			group_operator TEXT NOT NULL DEFAULT 'any', reply_text TEXT NOT NULL DEFAULT '',
			reply_text_by_chat_type JSONB NOT NULL DEFAULT '{}'::jsonb, action_type TEXT NOT NULL DEFAULT 'text',
			contact_phone TEXT NOT NULL DEFAULT '', contact_first_name TEXT NOT NULL DEFAULT '', contact_last_name TEXT NOT NULL DEFAULT '',
			media_asset_id UUID, media_type TEXT NOT NULL DEFAULT '', media_caption TEXT NOT NULL DEFAULT '',
			media_album_items JSONB NOT NULL DEFAULT '[]'::jsonb, case_sensitive BOOLEAN NOT NULL DEFAULT FALSE,
			enabled BOOLEAN NOT NULL DEFAULT TRUE, created_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
			updated_at TIMESTAMPTZ NOT NULL DEFAULT NOW())`,
		`CREATE TABLE tenant_keyword_matches (
			id BIGSERIAL PRIMARY KEY, tenant_id UUID NOT NULL, rule_id UUID NOT NULL, telegram_account_id UUID NOT NULL,
			chat_id BIGINT NOT NULL, message_id BIGINT NOT NULL, matched_terms JSONB NOT NULL DEFAULT '[]'::jsonb,
			matched_at TIMESTAMPTZ NOT NULL DEFAULT NOW(), UNIQUE(rule_id,telegram_account_id,chat_id,message_id))`,
		`CREATE TABLE keyword_action_jobs (
			id UUID PRIMARY KEY DEFAULT gen_random_uuid(), tenant_id UUID NOT NULL, keyword_match_id BIGINT NOT NULL UNIQUE,
			telegram_account_id UUID NOT NULL, chat_id BIGINT NOT NULL, reply_to_message_id BIGINT NOT NULL,
			message_text TEXT NOT NULL DEFAULT '', action_type TEXT NOT NULL DEFAULT 'text', contact_phone TEXT NOT NULL DEFAULT '',
			contact_first_name TEXT NOT NULL DEFAULT '', contact_last_name TEXT NOT NULL DEFAULT '', media_asset_id UUID,
			media_type TEXT NOT NULL DEFAULT '', media_caption TEXT NOT NULL DEFAULT '',
			media_album_items JSONB NOT NULL DEFAULT '[]'::jsonb, target_user_id BIGINT, attempts INTEGER NOT NULL DEFAULT 0,
			status TEXT NOT NULL DEFAULT 'pending', available_at TIMESTAMPTZ NOT NULL DEFAULT NOW(), lease_until TIMESTAMPTZ,
			last_error TEXT NOT NULL DEFAULT '', telegram_message_id BIGINT, telegram_message_ids JSONB NOT NULL DEFAULT '[]'::jsonb,
			created_at TIMESTAMPTZ NOT NULL DEFAULT NOW(), updated_at TIMESTAMPTZ NOT NULL DEFAULT NOW())`,
		`CREATE TABLE message_event_outbox (
			id BIGSERIAL PRIMARY KEY, tenant_id UUID NOT NULL, event_type TEXT NOT NULL, payload JSONB NOT NULL,
			created_at TIMESTAMPTZ NOT NULL DEFAULT NOW(), published_at TIMESTAMPTZ)`,
	}
	for _, statement := range ddl {
		if _, err := db.ExecContext(ctx, statement); err != nil {
			t.Fatal(err)
		}
	}
	if _, err := New(db).CreateKeywordRule(ctx, tenantID, KeywordRule{
		Name: "concurrent match", MatchMode: "any", Keywords: []string{"hello"},
		ChatTypes: []string{"private"}, ActionType: "text", ReplyText: "Thanks for your message",
		Enabled: true,
	}); err != nil {
		t.Fatal(err)
	}
	incoming := Incoming{
		AccountID: accountID, ChatID: 12345, MessageID: 987,
		ChatType: "private", Text: "hello there", MessageDate: time.Now().UTC(),
	}
	start := make(chan struct{})
	var wg sync.WaitGroup
	errs := make(chan error, 2)
	for i := 0; i < 2; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			<-start
			_, err := New(db).Ingest(ctx, tenantID, incoming)
			errs <- err
		}()
	}
	close(start)
	wg.Wait()
	close(errs)
	for err := range errs {
		if err != nil {
			t.Fatal(err)
		}
	}
	var matches, actions, dedup int
	if err := db.QueryRowContext(ctx, `SELECT COUNT(*) FROM tenant_keyword_matches WHERE tenant_id=$1`, tenantID).Scan(&matches); err != nil {
		t.Fatal(err)
	}
	if err := db.QueryRowContext(ctx, `SELECT COUNT(*) FROM keyword_action_jobs WHERE tenant_id=$1`, tenantID).Scan(&actions); err != nil {
		t.Fatal(err)
	}
	if err := db.QueryRowContext(ctx, `SELECT COUNT(*) FROM telegram_incoming_update_dedup WHERE tenant_id=$1`, tenantID).Scan(&dedup); err != nil {
		t.Fatal(err)
	}
	if matches != 1 || actions != 1 || dedup != 1 {
		t.Fatalf("concurrent duplicate ingest produced matches/actions/dedup=%d/%d/%d, want 1/1/1", matches, actions, dedup)
	}
}
