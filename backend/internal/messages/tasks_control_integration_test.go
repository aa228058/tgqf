package messages

import (
	"context"
	"strconv"
	"testing"
	"time"
)

func TestUnifiedJoinTaskControlsOnlySelectedAccountRow(t *testing.T) {
	db, cleanup := openGroupIntegrationDB(t)
	defer cleanup()
	ctx := context.Background()
	tenantID := "11111111-1111-4111-8111-111111111111"
	accountID := "aaaaaaaa-aaaa-4aaa-8aaa-aaaaaaaaaaaa"
	processingAccountID := "bbbbbbbb-bbbb-4bbb-8bbb-bbbbbbbbbbbb"
	taskID := "33333333-3333-4333-8333-333333333333"
	if _, err := db.ExecContext(ctx, `INSERT INTO tenants(id,status,expires_at) VALUES($1,'active',NOW()+INTERVAL '30 days')`, tenantID); err != nil {
		t.Fatal(err)
	}
	for _, id := range []string{accountID, processingAccountID} {
		if _, err := db.ExecContext(ctx, `INSERT INTO telegram_accounts(tenant_id,id,status,session_ciphertext) VALUES($1,$2,'authorized',$3)`, tenantID, id, []byte{1}); err != nil {
			t.Fatal(err)
		}
	}
	if _, err := db.ExecContext(ctx, `INSERT INTO tenant_join_tasks(id,tenant_id,name,status) VALUES($1,$2,'Join target','queued')`, taskID, tenantID); err != nil {
		t.Fatal(err)
	}
	var queuedID, processingID int64
	if err := db.QueryRowContext(ctx, `INSERT INTO tenant_join_task_accounts(tenant_id,task_id,telegram_account_id,status)
		VALUES($1,$2,$3,'queued') RETURNING id`, tenantID, taskID, accountID).Scan(&queuedID); err != nil {
		t.Fatal(err)
	}
	if err := db.QueryRowContext(ctx, `INSERT INTO tenant_join_task_accounts(tenant_id,task_id,telegram_account_id,status,lease_until)
		VALUES($1,$2,$3,'processing',NOW()+INTERVAL '2 minutes') RETURNING id`, tenantID, taskID, processingAccountID).Scan(&processingID); err != nil {
		t.Fatal(err)
	}

	service := New(db)
	if err := service.ChangeUnifiedTaskStatus(ctx, tenantID, "group_join", strconv.FormatInt(queuedID, 10), "cancel"); err != nil {
		t.Fatal(err)
	}
	var queuedStatus, processingStatus string
	var pauseRequested bool
	var leaseUntil time.Time
	if err := db.QueryRowContext(ctx, `SELECT status FROM tenant_join_task_accounts WHERE id=$1`, queuedID).Scan(&queuedStatus); err != nil {
		t.Fatal(err)
	}
	if err := db.QueryRowContext(ctx, `SELECT status,pause_requested,lease_until FROM tenant_join_task_accounts WHERE id=$1`, processingID).Scan(&processingStatus, &pauseRequested, &leaseUntil); err != nil {
		t.Fatal(err)
	}
	if queuedStatus != "cancelled" {
		t.Fatalf("selected account row status=%q, want cancelled", queuedStatus)
	}
	if processingStatus != "processing" || pauseRequested {
		t.Fatalf("unselected account row changed: status=%q pause_requested=%v", processingStatus, pauseRequested)
	}

	if err := service.ChangeUnifiedTaskStatus(ctx, tenantID, "group_join", strconv.FormatInt(processingID, 10), "pause"); err != nil {
		t.Fatal(err)
	}
	var afterLease time.Time
	if err := db.QueryRowContext(ctx, `SELECT status,pause_requested,lease_until FROM tenant_join_task_accounts WHERE id=$1`, processingID).Scan(&processingStatus, &pauseRequested, &afterLease); err != nil {
		t.Fatal(err)
	}
	if processingStatus != "processing" || !pauseRequested {
		t.Fatalf("in-flight pause must be requested without revoking the lease: status=%q pause_requested=%v", processingStatus, pauseRequested)
	}
	if delta := afterLease.Sub(leaseUntil); delta < -time.Microsecond || delta > time.Microsecond {
		t.Fatalf("in-flight pause changed lease: before=%s after=%s", leaseUntil, afterLease)
	}
}
