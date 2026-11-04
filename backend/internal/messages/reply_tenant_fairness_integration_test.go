package messages

import (
	"context"
	"testing"
)

func TestReplyClaimsRotateAcrossTenantsWithBacklogs(t *testing.T) {
	db, cleanup := openGroupIntegrationDB(t)
	defer cleanup()
	ctx := context.Background()

	ddl := []string{
		`CREATE TABLE message_reply_jobs (
			id UUID PRIMARY KEY, tenant_id UUID NOT NULL, telegram_account_id UUID NOT NULL,
			chat_id BIGINT NOT NULL DEFAULT 1, reply_to_message_id BIGINT NOT NULL DEFAULT 1,
			message_text TEXT NOT NULL DEFAULT 'hello', message_type TEXT NOT NULL DEFAULT 'text',
			contact_phone TEXT NOT NULL DEFAULT '', contact_first_name TEXT NOT NULL DEFAULT '',
			contact_last_name TEXT NOT NULL DEFAULT '', attempts INTEGER NOT NULL DEFAULT 0,
			status TEXT NOT NULL DEFAULT 'pending', available_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
			lease_until TIMESTAMPTZ, telegram_message_id BIGINT, last_error TEXT NOT NULL DEFAULT '',
			created_at TIMESTAMPTZ NOT NULL DEFAULT NOW(), updated_at TIMESTAMPTZ NOT NULL DEFAULT NOW())`,
		`CREATE TABLE keyword_action_jobs (tenant_id UUID, telegram_account_id UUID, status TEXT, lease_until TIMESTAMPTZ)`,
		`CREATE TABLE account_profile_update_jobs (tenant_id UUID, account_id UUID, status TEXT, lease_until TIMESTAMPTZ)`,
	}
	for _, statement := range ddl {
		if _, err := db.ExecContext(ctx, statement); err != nil {
			t.Fatal(err)
		}
	}

	tenants := []string{
		"11111111-1111-4111-8111-111111111111",
		"22222222-2222-4222-8222-222222222222",
	}
	accounts := [][]string{
		{"aaaaaaaa-aaaa-4aaa-8aaa-aaaaaaaaaaaa", "bbbbbbbb-bbbb-4bbb-8bbb-bbbbbbbbbbbb"},
		{"cccccccc-cccc-4ccc-8ccc-cccccccccccc", "dddddddd-dddd-4ddd-8ddd-dddddddddddd"},
	}
	for i, tenantID := range tenants {
		if _, err := db.ExecContext(ctx, `INSERT INTO tenants(id,status,expires_at) VALUES($1,'active',NOW()+INTERVAL '30 days')`, tenantID); err != nil {
			t.Fatal(err)
		}
		for j, accountID := range accounts[i] {
			if _, err := db.ExecContext(ctx, `INSERT INTO telegram_accounts(tenant_id,id,status,session_ciphertext) VALUES($1,$2,'authorized',$3)`, tenantID, accountID, []byte{1}); err != nil {
				t.Fatal(err)
			}
			jobID := "30000000-0000-4000-8000-00000000000" + string(rune('1'+i*2+j))
			if _, err := db.ExecContext(ctx, `INSERT INTO message_reply_jobs(id,tenant_id,telegram_account_id) VALUES($1,$2,$3)`, jobID, tenantID, accountID); err != nil {
				t.Fatal(err)
			}
		}
	}

	service := New(db)
	wantTenants := []string{tenants[0], tenants[1], tenants[0]}
	for i, wantTenant := range wantTenants {
		jobs, err := service.ClaimPendingReplies(ctx, 1)
		if err != nil {
			t.Fatalf("claim %d: %v", i+1, err)
		}
		if len(jobs) != 1 {
			t.Fatalf("claim %d returned %d jobs, want exactly one", i+1, len(jobs))
		}
		if jobs[0].TenantID != wantTenant {
			t.Fatalf("claim %d selected tenant %s, want fair rotation to %s", i+1, jobs[0].TenantID, wantTenant)
		}
	}
}
