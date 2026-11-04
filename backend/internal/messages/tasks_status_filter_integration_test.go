package messages

import (
	"context"
	"testing"
)

func TestUnifiedTaskStatusFiltersNormalizeQueueStates(t *testing.T) {
	db, cleanup := openGroupIntegrationDB(t)
	defer cleanup()
	ctx := context.Background()
	tenantID := "11111111-1111-4111-8111-111111111111"
	accountID := "aaaaaaaa-aaaa-4aaa-8aaa-aaaaaaaaaaaa"
	if _, err := db.ExecContext(ctx, `INSERT INTO tenants(id,status,expires_at) VALUES($1,'active',NOW()+INTERVAL '30 days')`, tenantID); err != nil {
		t.Fatal(err)
	}
	if _, err := db.ExecContext(ctx, `INSERT INTO telegram_accounts(tenant_id,id,status,session_ciphertext,display_name,phone) VALUES($1,$2,'authorized',$3,'Test account','+10000000000')`, tenantID, accountID, []byte{1}); err != nil {
		t.Fatal(err)
	}
	ddl := []string{
		`CREATE TABLE message_reply_jobs (
			id UUID PRIMARY KEY, tenant_id UUID NOT NULL, telegram_account_id UUID NOT NULL,
			status TEXT NOT NULL, message_type TEXT NOT NULL DEFAULT 'text', attempts INTEGER NOT NULL DEFAULT 0,
			available_at TIMESTAMPTZ NOT NULL DEFAULT NOW(), lease_until TIMESTAMPTZ,
			last_error TEXT NOT NULL DEFAULT '', created_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
			updated_at TIMESTAMPTZ NOT NULL DEFAULT NOW())`,
		`CREATE TABLE keyword_action_jobs (
			id UUID PRIMARY KEY, tenant_id UUID NOT NULL, telegram_account_id UUID NOT NULL,
			status TEXT NOT NULL, attempts INTEGER NOT NULL DEFAULT 0,
			available_at TIMESTAMPTZ NOT NULL DEFAULT NOW(), lease_until TIMESTAMPTZ,
			last_error TEXT NOT NULL DEFAULT '', created_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
			updated_at TIMESTAMPTZ NOT NULL DEFAULT NOW())`,
		`CREATE TABLE account_profile_update_jobs (
			id UUID PRIMARY KEY, tenant_id UUID NOT NULL, account_id UUID NOT NULL, batch_id UUID,
			status TEXT NOT NULL, attempts INTEGER NOT NULL DEFAULT 0,
			next_run_at TIMESTAMPTZ NOT NULL DEFAULT NOW(), lease_until TIMESTAMPTZ,
			last_error TEXT NOT NULL DEFAULT '', created_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
			updated_at TIMESTAMPTZ NOT NULL DEFAULT NOW())`,
	}
	for _, statement := range ddl {
		if _, err := db.ExecContext(ctx, statement); err != nil {
			t.Fatal(err)
		}
	}
	joinTaskID := "33333333-3333-4333-8333-333333333333"
	if _, err := db.ExecContext(ctx, `INSERT INTO tenant_join_tasks(id,tenant_id,name,status) VALUES($1,$2,'Join target','queued')`, joinTaskID, tenantID); err != nil {
		t.Fatal(err)
	}
	if _, err := db.ExecContext(ctx, `INSERT INTO message_reply_jobs(id,tenant_id,telegram_account_id,status) VALUES
		('44444444-4444-4444-8444-444444444441',$1,$2,'pending')`, tenantID, accountID); err != nil {
		t.Fatal(err)
	}
	if _, err := db.ExecContext(ctx, `INSERT INTO keyword_action_jobs(id,tenant_id,telegram_account_id,status) VALUES
		('55555555-5555-4555-8555-555555555551',$1,$2,'sent')`, tenantID, accountID); err != nil {
		t.Fatal(err)
	}
	joinAccounts := []string{
		"bbbbbbbb-bbbb-4bbb-8bbb-bbbbbbbbbbb1",
		"bbbbbbbb-bbbb-4bbb-8bbb-bbbbbbbbbbb2",
		"bbbbbbbb-bbbb-4bbb-8bbb-bbbbbbbbbbb3",
		"bbbbbbbb-bbbb-4bbb-8bbb-bbbbbbbbbbb4",
	}
	for _, joinAccount := range joinAccounts {
		if _, err := db.ExecContext(ctx, `INSERT INTO telegram_accounts(tenant_id,id,status,session_ciphertext) VALUES($1,$2,'authorized',$3)`, tenantID, joinAccount, []byte{1}); err != nil {
			t.Fatal(err)
		}
	}
	if _, err := db.ExecContext(ctx, `INSERT INTO tenant_join_task_accounts(tenant_id,task_id,telegram_account_id,status) VALUES
		($1,$2,$3,'queued'),($1,$4,$5,'joined'),($1,$4,$6,'already_member'),($1,$4,$7,'skipped')`, tenantID, joinTaskID, accountID, joinTaskID, joinAccounts[0], joinAccounts[1], joinAccounts[2]); err != nil {
		t.Fatal(err)
	}
	if _, err := db.ExecContext(ctx, `INSERT INTO tenant_join_task_accounts(tenant_id,task_id,telegram_account_id,status,approval_check_lease_until)
		VALUES($1,$2,$3,'request_pending',NOW()-INTERVAL '1 minute')`, tenantID, joinTaskID, joinAccounts[3]); err != nil {
		t.Fatal(err)
	}
	// A second tenant has its own pending task; it must never leak into this
	// tenant's unified list or pagination total.
	otherTenant := "22222222-2222-4222-8222-222222222222"
	otherAccount := "bbbbbbbb-bbbb-4bbb-8bbb-bbbbbbbbbbbb"
	if _, err := db.ExecContext(ctx, `INSERT INTO tenants(id,status,expires_at) VALUES($1,'active',NOW()+INTERVAL '30 days')`, otherTenant); err != nil {
		t.Fatal(err)
	}
	if _, err := db.ExecContext(ctx, `INSERT INTO telegram_accounts(tenant_id,id,status,session_ciphertext) VALUES($1,$2,'authorized',$3)`, otherTenant, otherAccount, []byte{1}); err != nil {
		t.Fatal(err)
	}
	if _, err := db.ExecContext(ctx, `INSERT INTO message_reply_jobs(id,tenant_id,telegram_account_id,status) VALUES
		('66666666-6666-4666-8666-666666666661',$1,$2,'pending')`, otherTenant, otherAccount); err != nil {
		t.Fatal(err)
	}

	service := New(db)
	queued, total, err := service.ListUnifiedTasks(ctx, tenantID, UnifiedTaskFilter{Status: "queued", Limit: 50})
	if err != nil {
		t.Fatal(err)
	}
	if total != 2 || len(queued) != 2 {
		t.Fatalf("queued filter returned %d rows (total %d), want pending + queued = 2", len(queued), total)
	}
	completed, total, err := service.ListUnifiedTasks(ctx, tenantID, UnifiedTaskFilter{Status: "completed", Limit: 50})
	if err != nil {
		t.Fatal(err)
	}
	if total != 4 || len(completed) != 4 {
		t.Fatalf("completed filter returned %d rows (total %d), want sent + joined + already_member + skipped = 4", len(completed), total)
	}
	page, pageTotal, err := service.ListUnifiedTasks(ctx, tenantID, UnifiedTaskFilter{Limit: 1, Offset: 1})
	if err != nil {
		t.Fatal(err)
	}
	if pageTotal != 7 || len(page) != 1 {
		t.Fatalf("paginated task list returned %d rows (total %d), want 1 row from a tenant total of 7", len(page), pageTotal)
	}
	summary, err := service.GetUnifiedTaskSummary(ctx, tenantID)
	if err != nil {
		t.Fatal(err)
	}
	if summary.Total != 7 {
		t.Fatalf("tenant task summary total=%d, want 7; cross-tenant task leaked or queue omitted", summary.Total)
	}
	if summary.ExpiredLeases != 1 {
		t.Fatalf("expired lease summary=%d, want the overdue join approval check lease to be included", summary.ExpiredLeases)
	}
}
