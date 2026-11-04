package messages

import (
	"context"
	"testing"
	"time"
)

func TestPrunePublishedMessageEventsKeepsPendingAndRecentRows(t *testing.T) {
	db, cleanup := openMigratedIntegrationDB(t)
	defer cleanup()
	ctx := context.Background()
	tenantID := "99999999-9999-4999-8999-999999999999"
	if _, err := db.ExecContext(ctx, `INSERT INTO tenants(id,status,expires_at) VALUES($1,'active',NOW()+INTERVAL '30 days')`, tenantID); err != nil {
		t.Fatal(err)
	}
	if _, err := db.ExecContext(ctx, `INSERT INTO message_event_outbox(tenant_id,event_type,payload,published_at)
		VALUES($1,'message.created','{}'::jsonb,NOW()-INTERVAL '40 days')`, tenantID); err != nil {
		t.Fatal(err)
	}
	if _, err := db.ExecContext(ctx, `INSERT INTO message_event_outbox(tenant_id,event_type,payload,published_at)
		VALUES($1,'message.created','{}'::jsonb,NOW()-INTERVAL '2 days')`, tenantID); err != nil {
		t.Fatal(err)
	}
	if _, err := db.ExecContext(ctx, `INSERT INTO message_event_outbox(tenant_id,event_type,payload)
		VALUES($1,'message.created','{}'::jsonb)`, tenantID); err != nil {
		t.Fatal(err)
	}
	removed, err := New(db).PrunePublishedMessageEvents(ctx, 30*24*time.Hour, 100)
	if err != nil {
		t.Fatal(err)
	}
	if removed != 1 {
		t.Fatalf("pruned rows=%d, want 1", removed)
	}
	var remaining int
	if err := db.QueryRowContext(ctx, `SELECT COUNT(*) FROM message_event_outbox WHERE tenant_id=$1`, tenantID).Scan(&remaining); err != nil {
		t.Fatal(err)
	}
	if remaining != 2 {
		t.Fatalf("remaining outbox rows=%d, want recent+pending (2)", remaining)
	}
	if _, err := New(db).PrunePublishedMessageEvents(ctx, time.Hour, 100); err == nil {
		t.Fatal("retention below 24 hours should be rejected")
	}
}
