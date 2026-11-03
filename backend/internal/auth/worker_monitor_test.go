package auth

import (
	"testing"
	"time"
)

func TestWorkerHeartbeatIsStale(t *testing.T) {
	now := time.Date(2026, 11, 4, 12, 0, 0, 0, time.UTC)
	if workerHeartbeatIsStale("running", now.Add(-44*time.Second), now) {
		t.Fatal("fresh worker heartbeat marked stale")
	}
	if !workerHeartbeatIsStale("running", now.Add(-46*time.Second), now) {
		t.Fatal("old worker heartbeat not marked stale")
	}
	if workerHeartbeatIsStale("stopped", now.Add(-time.Hour), now) {
		t.Fatal("stopped worker should not be marked stale")
	}
}
