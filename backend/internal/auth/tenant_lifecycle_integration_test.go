package auth

import (
	"context"
	"database/sql"
	"fmt"
	"os"
	"testing"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/stdlib"
)

func openTenantLifecycleIntegrationDB(t *testing.T) (*sql.DB, func()) {
	t.Helper()
	dsn := os.Getenv("TEST_DATABASE_URL")
	if dsn == "" {
		t.Skip("TEST_DATABASE_URL is not configured")
	}
	adminConfig, err := pgx.ParseConfig(dsn)
	if err != nil {
		t.Fatal(err)
	}
	adminDB := stdlib.OpenDB(*adminConfig)
	schema := fmt.Sprintf("tgqf_auth_lifecycle_%d", time.Now().UnixNano())
	if _, err := adminDB.Exec("CREATE SCHEMA " + schema); err != nil {
		adminDB.Close()
		t.Fatal(err)
	}
	config, err := pgx.ParseConfig(dsn)
	if err != nil {
		_, _ = adminDB.Exec("DROP SCHEMA " + schema + " CASCADE")
		adminDB.Close()
		t.Fatal(err)
	}
	config.RuntimeParams["search_path"] = schema
	db := stdlib.OpenDB(*config)
	db.SetMaxOpenConns(4)
	ddl := []string{
		`CREATE TABLE tenants (id UUID PRIMARY KEY,status TEXT NOT NULL,expires_at TIMESTAMPTZ,updated_at TIMESTAMPTZ NOT NULL DEFAULT NOW())`,
		`CREATE TABLE message_reply_jobs (id BIGSERIAL PRIMARY KEY,tenant_id UUID NOT NULL,status TEXT NOT NULL,lease_until TIMESTAMPTZ,last_error TEXT NOT NULL DEFAULT '',updated_at TIMESTAMPTZ NOT NULL DEFAULT NOW())`,
		`CREATE TABLE keyword_action_jobs (id BIGSERIAL PRIMARY KEY,tenant_id UUID NOT NULL,status TEXT NOT NULL,lease_until TIMESTAMPTZ,last_error TEXT NOT NULL DEFAULT '',updated_at TIMESTAMPTZ NOT NULL DEFAULT NOW())`,
		`CREATE TABLE account_profile_update_jobs (id BIGSERIAL PRIMARY KEY,tenant_id UUID NOT NULL,status TEXT NOT NULL,lease_until TIMESTAMPTZ,last_error TEXT NOT NULL DEFAULT '',updated_at TIMESTAMPTZ NOT NULL DEFAULT NOW())`,
		`CREATE TABLE tenant_join_tasks (id UUID PRIMARY KEY,tenant_id UUID NOT NULL,status TEXT NOT NULL,updated_at TIMESTAMPTZ NOT NULL DEFAULT NOW())`,
		`CREATE TABLE tenant_join_task_accounts (id BIGSERIAL PRIMARY KEY,tenant_id UUID NOT NULL,task_id UUID NOT NULL,status TEXT NOT NULL,lease_until TIMESTAMPTZ,pause_requested BOOLEAN NOT NULL DEFAULT FALSE,cancel_requested BOOLEAN NOT NULL DEFAULT FALSE,last_error TEXT NOT NULL DEFAULT '',updated_at TIMESTAMPTZ NOT NULL DEFAULT NOW())`,
		`CREATE TABLE activation_cards (id UUID PRIMARY KEY,code_hash BYTEA NOT NULL,duration_days INTEGER NOT NULL,status TEXT NOT NULL,activated_tenant_id UUID,activated_at TIMESTAMPTZ,expires_at TIMESTAMPTZ)`,
	}
	for _, statement := range ddl {
		if _, err := db.Exec(statement); err != nil {
			db.Close()
			_, _ = adminDB.Exec("DROP SCHEMA " + schema + " CASCADE")
			adminDB.Close()
			t.Fatal(err)
		}
	}
	cleanup := func() {
		db.Close()
		_, _ = adminDB.Exec("DROP SCHEMA " + schema + " CASCADE")
		adminDB.Close()
	}
	return db, cleanup
}

const lifecycleTenantID = "11111111-1111-4111-8111-111111111111"
const lifecycleJoinTaskID = "22222222-2222-4222-8222-222222222222"

func seedLifecycleQueues(t *testing.T, db *sql.DB, tenantID string) {
	t.Helper()
	ctx := context.Background()
	for _, table := range []string{"message_reply_jobs", "keyword_action_jobs", "account_profile_update_jobs"} {
		for _, state := range []string{"pending", "live", "expired"} {
			status := state
			if state == "live" || state == "expired" {
				status = "processing"
			}
			var lease any
			switch state {
			case "live":
				lease = time.Now().Add(5 * time.Minute)
			case "expired":
				lease = time.Now().Add(-5 * time.Minute)
			}
			if _, err := db.ExecContext(ctx, "INSERT INTO "+table+"(tenant_id,status,lease_until) VALUES($1,$2,$3)", tenantID, status, lease); err != nil {
				t.Fatalf("seed %s %s: %v", table, state, err)
			}
		}
	}
	if _, err := db.ExecContext(ctx, `INSERT INTO tenant_join_tasks(id,tenant_id,status) VALUES($1,$2,'queued')`, lifecycleJoinTaskID, tenantID); err != nil {
		t.Fatal(err)
	}
	for _, state := range []string{"queued", "live", "expired", "request_pending"} {
		status := state
		var lease any
		pauseRequested := false
		if state == "live" || state == "expired" {
			status = "processing"
			if state == "live" {
				lease = time.Now().Add(5 * time.Minute)
				pauseRequested = false
			} else {
				lease = time.Now().Add(-5 * time.Minute)
			}
		}
		if _, err := db.ExecContext(ctx, `INSERT INTO tenant_join_task_accounts(tenant_id,task_id,status,lease_until,pause_requested) VALUES($1,$2,$3,$4,$5)`, tenantID, lifecycleJoinTaskID, status, lease, pauseRequested); err != nil {
			t.Fatalf("seed join child %s: %v", state, err)
		}
	}
}

func assertFrozenLifecycleQueues(t *testing.T, db *sql.DB, tenantID string) {
	t.Helper()
	ctx := context.Background()
	for _, table := range []string{"message_reply_jobs", "keyword_action_jobs", "account_profile_update_jobs"} {
		var pendingPaused, expiredPaused, liveProcessing int
		if err := db.QueryRowContext(ctx, "SELECT COUNT(*) FROM "+table+" WHERE tenant_id=$1 AND status='paused' AND last_error='tenant inactive; manual review required'", tenantID).Scan(&pendingPaused); err != nil {
			t.Fatal(err)
		}
		if err := db.QueryRowContext(ctx, "SELECT COUNT(*) FROM "+table+" WHERE tenant_id=$1 AND status='processing' AND lease_until>NOW()", tenantID).Scan(&liveProcessing); err != nil {
			t.Fatal(err)
		}
		if pendingPaused != 2 || liveProcessing != 1 {
			t.Fatalf("%s: paused=%d live_processing=%d, want paused pending+expired=2 and live processing=1", table, pendingPaused, liveProcessing)
		}
		if err := db.QueryRowContext(ctx, "SELECT COUNT(*) FROM "+table+" WHERE tenant_id=$1 AND status='paused' AND last_error='tenant inactive; manual review required' AND lease_until IS NULL", tenantID).Scan(&expiredPaused); err != nil {
			t.Fatal(err)
		}
		if expiredPaused != 2 {
			t.Fatalf("%s: paused rows with cleared lease=%d, want 2", table, expiredPaused)
		}
	}
	var parentStatus string
	if err := db.QueryRowContext(ctx, "SELECT status FROM tenant_join_tasks WHERE id=$1", lifecycleJoinTaskID).Scan(&parentStatus); err != nil {
		t.Fatal(err)
	}
	if parentStatus != "paused" {
		t.Fatalf("join parent status=%q, want paused", parentStatus)
	}
	var queuedPaused, expiredPaused, liveRequested, requestPending int
	if err := db.QueryRowContext(ctx, `SELECT COUNT(*) FROM tenant_join_task_accounts WHERE tenant_id=$1 AND status='paused' AND lease_until IS NULL`, tenantID).Scan(&queuedPaused); err != nil {
		t.Fatal(err)
	}
	if err := db.QueryRowContext(ctx, `SELECT COUNT(*) FROM tenant_join_task_accounts WHERE tenant_id=$1 AND status='processing' AND lease_until>NOW() AND pause_requested=TRUE`, tenantID).Scan(&liveRequested); err != nil {
		t.Fatal(err)
	}
	if err := db.QueryRowContext(ctx, `SELECT COUNT(*) FROM tenant_join_task_accounts WHERE tenant_id=$1 AND status='request_pending'`, tenantID).Scan(&requestPending); err != nil {
		t.Fatal(err)
	}
	if queuedPaused != 2 || liveRequested != 1 || requestPending != 1 {
		t.Fatalf("join children: paused=%d live_pause_requested=%d request_pending=%d; want 2,1,1", queuedPaused, liveRequested, requestPending)
	}
	if err := db.QueryRowContext(ctx, `SELECT COUNT(*) FROM tenant_join_task_accounts WHERE tenant_id=$1 AND status='paused' AND last_error='tenant inactive; manual review required'`, tenantID).Scan(&expiredPaused); err != nil {
		t.Fatal(err)
	}
	if expiredPaused != 2 {
		t.Fatalf("join expired/queued children paused=%d, want 2", expiredPaused)
	}
}

func TestTenantDisableFreezesQueuedWorkAndPreservesLiveLeases(t *testing.T) {
	db, cleanup := openTenantLifecycleIntegrationDB(t)
	defer cleanup()
	ctx := context.Background()
	if _, err := db.ExecContext(ctx, `INSERT INTO tenants(id,status,expires_at) VALUES($1,'active',NOW()+INTERVAL '30 days')`, lifecycleTenantID); err != nil {
		t.Fatal(err)
	}
	seedLifecycleQueues(t, db, lifecycleTenantID)
	if err := New(db).SetTenantStatus(ctx, lifecycleTenantID, "disabled"); err != nil {
		t.Fatal(err)
	}
	assertFrozenLifecycleQueues(t, db, lifecycleTenantID)
}

func TestTenantExpiryFreezesQueuedWorkAndPreservesLiveLeases(t *testing.T) {
	db, cleanup := openTenantLifecycleIntegrationDB(t)
	defer cleanup()
	ctx := context.Background()
	if _, err := db.ExecContext(ctx, `INSERT INTO tenants(id,status,expires_at) VALUES($1,'active',NOW()+INTERVAL '30 days')`, lifecycleTenantID); err != nil {
		t.Fatal(err)
	}
	seedLifecycleQueues(t, db, lifecycleTenantID)
	if err := New(db).SetTenantExpiry(ctx, lifecycleTenantID, time.Now().Add(-time.Minute)); err != nil {
		t.Fatal(err)
	}
	assertFrozenLifecycleQueues(t, db, lifecycleTenantID)
}

func TestActivationAfterExpiryFreezesOldWorkBeforeRenewal(t *testing.T) {
	db, cleanup := openTenantLifecycleIntegrationDB(t)
	defer cleanup()
	ctx := context.Background()
	if _, err := db.ExecContext(ctx, `INSERT INTO tenants(id,status,expires_at) VALUES($1,'active',NOW()-INTERVAL '1 day')`, lifecycleTenantID); err != nil {
		t.Fatal(err)
	}
	seedLifecycleQueues(t, db, lifecycleTenantID)
	code := "TG-test-renewal-code"
	if _, err := db.ExecContext(ctx, `INSERT INTO activation_cards(id,code_hash,duration_days,status) VALUES($1,$2,30,'unused')`, "33333333-3333-4333-8333-333333333333", tokenHash(code)); err != nil {
		t.Fatal(err)
	}
	if _, err := New(db).Activate(ctx, lifecycleTenantID, code); err != nil {
		t.Fatal(err)
	}
	assertFrozenLifecycleQueues(t, db, lifecycleTenantID)
	var expiry time.Time
	if err := db.QueryRowContext(ctx, "SELECT expires_at FROM tenants WHERE id=$1", lifecycleTenantID).Scan(&expiry); err != nil {
		t.Fatal(err)
	}
	if !expiry.After(time.Now()) {
		t.Fatalf("renewed expiry=%s, want future", expiry)
	}
}


func TestListAuditLogsFiltersAndIncludesActorAndTenant(t *testing.T) {
	db, cleanup := openTenantLifecycleIntegrationDB(t)
	defer cleanup()
	ctx := context.Background()
	if _, err := db.ExecContext(ctx, `ALTER TABLE tenants ADD COLUMN name TEXT NOT NULL DEFAULT ''`); err != nil {
		t.Fatal(err)
	}
	if _, err := db.ExecContext(ctx, `CREATE TABLE users(id UUID PRIMARY KEY,username TEXT NOT NULL)`); err != nil {
		t.Fatal(err)
	}
	if _, err := db.ExecContext(ctx, `CREATE TABLE audit_logs(
		id BIGSERIAL PRIMARY KEY,tenant_id UUID,actor_user_id UUID,action TEXT NOT NULL,
		resource_type TEXT NOT NULL,resource_id TEXT NOT NULL,details JSONB NOT NULL,created_at TIMESTAMPTZ NOT NULL DEFAULT NOW())`); err != nil {
		t.Fatal(err)
	}
	if _, err := db.ExecContext(ctx, `INSERT INTO tenants(id,status,expires_at,name) VALUES($1,'active',NOW()+INTERVAL '30 days','Tenant Alpha')`, lifecycleTenantID); err != nil {
		t.Fatal(err)
	}
	actorID := "44444444-4444-4444-8444-444444444444"
	if _, err := db.ExecContext(ctx, `INSERT INTO users(id,username) VALUES($1,'root-admin')`, actorID); err != nil {
		t.Fatal(err)
	}
	if _, err := db.ExecContext(ctx, `INSERT INTO audit_logs(tenant_id,actor_user_id,action,resource_type,resource_id,details)
		VALUES($1,$2,'admin.tenant.status','tenant',$1,'{"status":"disabled"}'::jsonb),
		(NULL,$2,'admin.cards.issue','activation_cards','','{"count":5}'::jsonb)`, lifecycleTenantID, actorID); err != nil {
		t.Fatal(err)
	}
	items, total, err := New(db).ListAuditLogs(ctx, lifecycleTenantID, "tenant.status", 50, 0)
	if err != nil {
		t.Fatal(err)
	}
	if total != 1 || len(items) != 1 {
		t.Fatalf("filtered audit rows total=%d len=%d, want 1", total, len(items))
	}
	if items[0].TenantName != "Tenant Alpha" || items[0].ActorUsername != "root-admin" || items[0].Action != "admin.tenant.status" {
		t.Fatalf("audit row missing tenant/actor/action: %+v", items[0])
	}
	all, allTotal, err := New(db).ListAuditLogs(ctx, "", "", 1, 0)
	if err != nil {
		t.Fatal(err)
	}
	if allTotal != 2 || len(all) != 1 {
		t.Fatalf("paginated audit rows total=%d len=%d, want total=2 len=1", allTotal, len(all))
	}
}
