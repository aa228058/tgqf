package messages

import (
	"context"
	"strconv"
	"testing"
)

func TestKeywordActionClaimsRotateAcrossTenants(t *testing.T) {
	db, cleanup := openGroupIntegrationDB(t)
	defer cleanup()
	ctx := context.Background()
	tenantA := "11111111-1111-4111-8111-111111111111"
	tenantB := "22222222-2222-4222-8222-222222222222"
	accountA := "aaaaaaaa-aaaa-4aaa-8aaa-aaaaaaaaaaaa"
	accountB := "bbbbbbbb-bbbb-4bbb-8bbb-bbbbbbbbbbbb"
	for _, tenant := range []struct{ id, account string }{{tenantA, accountA}, {tenantB, accountB}} {
		if _, err := db.ExecContext(ctx, `INSERT INTO tenants(id,status,expires_at) VALUES($1,'active',NOW()+INTERVAL '30 days')`, tenant.id); err != nil { t.Fatal(err) }
		if _, err := db.ExecContext(ctx, `INSERT INTO telegram_accounts(tenant_id,id,status,session_ciphertext) VALUES($1,$2,'authorized',$3)`, tenant.id, tenant.account, []byte{1}); err != nil { t.Fatal(err) }
	}
	for _, ddl := range []string{
		`CREATE TABLE message_reply_jobs(tenant_id UUID,telegram_account_id UUID,status TEXT,lease_until TIMESTAMPTZ)`,
		`CREATE TABLE account_profile_update_jobs(tenant_id UUID,account_id UUID,status TEXT,lease_until TIMESTAMPTZ)`,
		// tenant_join_task_accounts is part of the shared integration schema.
		// Do not recreate it here: doing so makes this test fail with 42P07.
		`CREATE TABLE keyword_action_jobs(
			id UUID PRIMARY KEY,tenant_id UUID NOT NULL,telegram_account_id UUID NOT NULL,
			chat_id BIGINT NOT NULL DEFAULT 1,reply_to_message_id BIGINT NOT NULL DEFAULT 1,
			message_text TEXT NOT NULL DEFAULT 'hello',action_type TEXT NOT NULL DEFAULT 'text',
			contact_phone TEXT NOT NULL DEFAULT '',contact_first_name TEXT NOT NULL DEFAULT '',contact_last_name TEXT NOT NULL DEFAULT '',
			media_asset_id UUID,media_type TEXT NOT NULL DEFAULT '',media_caption TEXT NOT NULL DEFAULT '',
			media_album_items JSONB NOT NULL DEFAULT '[]'::jsonb,attempts INTEGER NOT NULL DEFAULT 0,
			status TEXT NOT NULL DEFAULT 'pending',available_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
			lease_until TIMESTAMPTZ,last_error TEXT NOT NULL DEFAULT '',telegram_message_id BIGINT,
			telegram_message_ids JSONB NOT NULL DEFAULT '[]'::jsonb,created_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
			updated_at TIMESTAMPTZ NOT NULL DEFAULT NOW())`,
	} {
		if _, err := db.ExecContext(ctx, ddl); err != nil { t.Fatal(err) }
	}
	for i := 0; i < 2; i++ {
		for _, tenant := range []struct{ id, account string }{{tenantA, accountA}, {tenantB, accountB}} {
			suffix := 1 + i*2
			if tenant.id == tenantB { suffix++ }
			jobID := "10000000-0000-4000-8000-00000000000" + strconv.Itoa(suffix)
			if _, err := db.ExecContext(ctx, `INSERT INTO keyword_action_jobs(id,tenant_id,telegram_account_id) VALUES($1,$2,$3)`, jobID, tenant.id, tenant.account); err != nil { t.Fatal(err) }
		}
	}
	service := New(db)
	first, err := service.ClaimKeywordActionJobs(ctx, 1)
	if err != nil { t.Fatal(err) }
	if _, err := db.ExecContext(ctx, `UPDATE telegram_accounts SET telegram_send_next_at=NOW()-INTERVAL '1 second'`); err != nil { t.Fatal(err) }
	second, err := service.ClaimKeywordActionJobs(ctx, 1)
	if err != nil { t.Fatal(err) }
	if _, err := db.ExecContext(ctx, `UPDATE telegram_accounts SET telegram_send_next_at=NOW()-INTERVAL '1 second'`); err != nil { t.Fatal(err) }
	third, err := service.ClaimKeywordActionJobs(ctx, 1)
	if err != nil { t.Fatal(err) }
	if len(first) != 1 || len(second) != 1 || len(third) != 1 { t.Fatalf("claim batch sizes: %d/%d/%d, want 1 each", len(first), len(second), len(third)) }
	if first[0].TenantID != tenantA || second[0].TenantID != tenantB || third[0].TenantID != tenantA {
		t.Fatalf("tenant claim order=%s -> %s -> %s, want A -> B -> A", first[0].TenantID, second[0].TenantID, third[0].TenantID)
	}
}
