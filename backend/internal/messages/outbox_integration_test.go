package messages

import (
	"context"
	"encoding/json"
	"testing"
	"time"
)

func TestPrunePublishedMessageEventsKeepsPendingAndRecentRows(t *testing.T) {
	db, cleanup := openGroupIntegrationDB(t)
	defer cleanup()
	ctx := context.Background()
	tenantID := "99999999-9999-4999-8999-999999999999"
	if _, err := db.ExecContext(ctx, `INSERT INTO tenants(id,status,expires_at) VALUES($1,'active',NOW()+INTERVAL '30 days')`, tenantID); err != nil {
		t.Fatal(err)
	}
	if _, err := db.ExecContext(ctx, `CREATE TABLE message_event_outbox (id BIGSERIAL PRIMARY KEY, tenant_id UUID NOT NULL, event_type TEXT NOT NULL, payload JSONB NOT NULL, created_at TIMESTAMPTZ NOT NULL DEFAULT NOW(), published_at TIMESTAMPTZ)`); err != nil {
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

func TestPublishPendingMessageEventsMovesPostgresOutboxToRedis(t *testing.T) {
	db, cleanup := openGroupIntegrationDB(t)
	defer cleanup()
	redisClient := openIncomingStreamRedis(t)
	ctx := context.Background()
	tenantID := "88888888-8888-4888-8888-888888888888"
	if _, err := db.ExecContext(ctx, `CREATE TABLE message_event_outbox (
		id BIGSERIAL PRIMARY KEY, tenant_id UUID NOT NULL, event_type TEXT NOT NULL,
		payload JSONB NOT NULL, created_at TIMESTAMPTZ NOT NULL DEFAULT NOW(), published_at TIMESTAMPTZ
	)`); err != nil {
		t.Fatal(err)
	}
	if _, err := db.ExecContext(ctx, `INSERT INTO message_event_outbox(tenant_id,event_type,payload)
		VALUES($1,'message.created','{"message_id":42}'::jsonb)`, tenantID); err != nil {
		t.Fatal(err)
	}

	published, err := New(db, redisClient).PublishPendingMessageEvents(ctx, 10)
	if err != nil {
		t.Fatalf("publish PostgreSQL outbox: %v", err)
	}
	if published != 1 {
		t.Fatalf("published=%d, want 1", published)
	}
	events, err := New(db, redisClient).ReadMessageEvents(ctx, tenantID, "0-0", 0, 10)
	if err != nil {
		t.Fatalf("read tenant Redis stream: %v", err)
	}
	if len(events) != 1 {
		t.Fatalf("published Redis event count=%d, want 1: %+v", len(events), events)
	}
	var payload map[string]int
	if err := json.Unmarshal([]byte(events[0].Payload), &payload); err != nil {
		t.Fatalf("decode published Redis payload: %v", err)
	}
	if payload["message_id"] != 42 {
		t.Fatalf("published Redis payload=%s, want message_id=42", events[0].Payload)
	}
	var publishedAt time.Time
	if err := db.QueryRowContext(ctx, `SELECT published_at FROM message_event_outbox WHERE tenant_id=$1`, tenantID).Scan(&publishedAt); err != nil {
		t.Fatal(err)
	}
	if publishedAt.IsZero() {
		t.Fatal("outbox row was not marked published after Redis accepted it")
	}
}
