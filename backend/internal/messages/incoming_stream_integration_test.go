package messages

import (
	"context"
	"os"
	"testing"
	"time"

	"github.com/redis/go-redis/v9"
)

func openIncomingStreamRedis(t *testing.T) *redis.Client {
	t.Helper()
	raw := os.Getenv("TEST_REDIS_URL")
	if raw == "" {
		if os.Getenv("CI") == "true" {
			t.Fatal("TEST_REDIS_URL is required in CI; refusing to skip Redis Streams integration tests")
		}
		t.Skip("TEST_REDIS_URL is not configured")
	}
	opts, err := redis.ParseURL(raw)
	if err != nil {
		t.Fatal(err)
	}
	client := redis.NewClient(opts)
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	if err := client.Ping(ctx).Err(); err != nil {
		t.Fatalf("connect to Redis integration service: %v", err)
	}
	t.Cleanup(func() { _ = client.Del(context.Background(), incomingUpdateStream).Err(); _ = client.Close() })
	return client
}

func TestIncomingUpdateStreamConsumerRecoveryAndAck(t *testing.T) {
	client := openIncomingStreamRedis(t)
	ctx := context.Background()
	service := New(nil, client)
	tenantID := "11111111-1111-4111-8111-111111111111"
	incoming := Incoming{AccountID: "aaaaaaaa-aaaa-4aaa-8aaa-aaaaaaaaaaaa", ChatID: 12345, MessageID: 678, Text: "integration"}
	id, err := service.EnqueueIncomingUpdate(ctx, tenantID, incoming)
	if err != nil {
		t.Fatalf("enqueue incoming update: %v", err)
	}

	first, err := service.ReadIncomingUpdates(ctx, "worker-crashed", 10)
	if err != nil {
		t.Fatalf("first consumer read: %v", err)
	}
	if len(first) != 1 || first[0].ID != id || first[0].TenantID != tenantID || first[0].Incoming.MessageID != incoming.MessageID {
		t.Fatalf("first consumer received unexpected event: %+v", first)
	}

	// ReadIncomingUpdates reclaims only entries idle for at least 30 seconds.
	time.Sleep(31 * time.Second)
	reclaimed, err := service.ReadIncomingUpdates(ctx, "worker-recovery", 10)
	if err != nil {
		t.Fatalf("reclaim pending update: %v", err)
	}
	if len(reclaimed) != 1 || reclaimed[0].ID != id {
		t.Fatalf("recovery consumer did not reclaim pending event: %+v", reclaimed)
	}
	pending, err := client.XPending(ctx, incomingUpdateStream, incomingUpdateGroup).Result()
	if err != nil {
		t.Fatalf("inspect pending entries: %v", err)
	}
	if pending.Count != 1 {
		t.Fatalf("pending count before ack=%d, want 1", pending.Count)
	}

	if err := service.AckIncomingUpdate(ctx, id); err != nil {
		t.Fatalf("ack and delete incoming update: %v", err)
	}
	pending, err = client.XPending(ctx, incomingUpdateStream, incomingUpdateGroup).Result()
	if err != nil {
		t.Fatalf("inspect pending entries after ack: %v", err)
	}
	length, err := client.XLen(ctx, incomingUpdateStream).Result()
	if err != nil {
		t.Fatalf("inspect stream length: %v", err)
	}
	if pending.Count != 0 || length != 0 {
		t.Fatalf("after ack pending=%d stream length=%d, want 0/0", pending.Count, length)
	}
}

func TestIncomingUpdateStreamRedisOutageAndRecovery(t *testing.T) {
	healthy := openIncomingStreamRedis(t)
	ctx := context.Background()
	tenantID := "22222222-2222-4222-8222-222222222222"
	incoming := Incoming{AccountID: "bbbbbbbb-bbbb-4bbb-8bbb-bbbbbbbbbbbb", ChatID: 22334, MessageID: 679, Text: "recovery"}
	broken := redis.NewClient(&redis.Options{Addr: "127.0.0.1:1", DialTimeout: 100 * time.Millisecond, ReadTimeout: 100 * time.Millisecond, WriteTimeout: 100 * time.Millisecond, MaxRetries: 0})
	defer broken.Close()
	unavailable := New(nil, broken)
	if _, err := unavailable.EnqueueIncomingUpdate(ctx, tenantID, incoming); err == nil {
		t.Fatal("enqueue unexpectedly succeeded while Redis was unavailable")
	}
	recovered := New(nil, healthy)
	id, err := recovered.EnqueueIncomingUpdate(ctx, tenantID, incoming)
	if err != nil {
		t.Fatalf("enqueue after Redis recovery: %v", err)
	}
	events, err := recovered.ReadIncomingUpdates(ctx, "recovered-worker", 10)
	if err != nil {
		t.Fatalf("read after Redis recovery: %v", err)
	}
	if len(events) != 1 || events[0].ID != id {
		t.Fatalf("recovered event mismatch: %+v", events)
	}
	if err := recovered.AckIncomingUpdate(ctx, id); err != nil {
		t.Fatalf("ack recovered event: %v", err)
	}
}
