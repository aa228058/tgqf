package main

import (
	"context"
	"errors"
	"fmt"
	"os"
	"strings"
	"testing"
	"time"

	"github.com/aa228058/tgqf/backend/internal/accounts"
	"github.com/aa228058/tgqf/backend/internal/storage"
)

func TestReconnectDelay(t *testing.T) {
	tests := []struct {
		attempt int
		want    time.Duration
	}{
		{attempt: 0, want: 30 * time.Second},
		{attempt: 1, want: 30 * time.Second},
		{attempt: 2, want: 60 * time.Second},
		{attempt: 3, want: 2 * time.Minute},
		{attempt: 4, want: 4 * time.Minute},
		{attempt: 5, want: 8 * time.Minute},
		{attempt: 6, want: 15 * time.Minute},
		{attempt: 7, want: 15 * time.Minute},
	}
	for _, tt := range tests {
		if got := reconnectDelay(tt.attempt); got != tt.want {
			t.Errorf("reconnectDelay(%d) = %s, want %s", tt.attempt, got, tt.want)
		}
	}
}

func TestScheduleReconnect(t *testing.T) {
	now := time.Date(2026, 11, 3, 12, 0, 0, 0, time.UTC)
	state := reconnectState{}
	for attempt := 1; attempt <= 7; attempt++ {
		delay := scheduleReconnect(&state, now)
		if state.attempts != attempt {
			t.Fatalf("attempt = %d, want %d", state.attempts, attempt)
		}
		if delay != reconnectDelay(attempt) {
			t.Fatalf("attempt %d delay = %s, want %s", attempt, delay, reconnectDelay(attempt))
		}
		if !state.nextTry.Equal(now.Add(delay)) {
			t.Fatalf("attempt %d nextTry = %s, want %s", attempt, state.nextTry, now.Add(delay))
		}
		if now.Before(state.nextTry) {
			// A reconciliation tick before the deadline must skip this account.
			continue
		}
		t.Fatalf("retry deadline was not in the future")
	}
}

func TestRuntimeConfigChanged(t *testing.T) {
	tests := []struct {
		name           string
		currentSession string
		currentProxy   string
		desiredSession string
		desiredProxy   string
		want           bool
	}{
		{name: "unchanged config", currentSession: "session-a", currentProxy: "proxy-a", desiredSession: "session-a", desiredProxy: "proxy-a", want: false},
		{name: "session changed", currentSession: "session-a", currentProxy: "proxy-a", desiredSession: "session-b", desiredProxy: "proxy-a", want: true},
		{name: "proxy changed", currentSession: "session-a", currentProxy: "proxy-a", desiredSession: "session-a", desiredProxy: "proxy-b", want: true},
		{name: "proxy removed", currentSession: "session-a", currentProxy: "proxy-a", desiredSession: "session-a", desiredProxy: "", want: true},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := runtimeConfigChanged(tt.currentSession, tt.currentProxy, tt.desiredSession, tt.desiredProxy); got != tt.want {
				t.Fatalf("runtimeConfigChanged() = %v, want %v", got, tt.want)
			}
		})
	}
}

func TestGenerateRandomTelegramUsername(t *testing.T) {
	tests := []struct {
		name     string
		settings accounts.UsernameGenerationSettings
		prefix   string
		length   int
	}{
		{name: "tenant prefix alphanumeric", settings: accounts.UsernameGenerationSettings{Prefix: "brand_", RandomLength: 12, Charset: "alphanumeric"}, prefix: "brand_", length: 18},
		{name: "letters only", settings: accounts.UsernameGenerationSettings{Prefix: "team", RandomLength: 8, Charset: "letters"}, prefix: "team", length: 12},
		{name: "empty prefix", settings: accounts.UsernameGenerationSettings{Prefix: "", RandomLength: 10, Charset: "alphanumeric"}, length: 10},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			for i := 0; i < 100; i++ {
				username, err := generateRandomTelegramUsername(tt.settings)
				if err != nil {
					t.Fatal(err)
				}
				if len(username) != tt.length {
					t.Fatalf("length %d, want %d: %q", len(username), tt.length, username)
				}
				if tt.prefix != "" && !strings.HasPrefix(username, tt.prefix) {
					t.Fatalf("username %q missing prefix %q", username, tt.prefix)
				}
				if username[0] < 'a' || username[0] > 'z' {
					t.Fatalf("username must start with lowercase letter: %q", username)
				}
				for _, ch := range username {
					if !((ch >= 'a' && ch <= 'z') || (ch >= '0' && ch <= '9') || ch == '_') {
						t.Fatalf("invalid character in %q", username)
					}
					if tt.settings.Charset == "letters" && !(ch >= 'a' && ch <= 'z') && ch != '_' {
						t.Fatalf("non-letter in letters-only username %q", username)
					}
				}
			}
		})
	}
}

func TestIsPermanentJoinFailure(t *testing.T) {
	tests := []struct {
		name string
		err  error
		want bool
	}{
		{"expired invite", errors.New("INVITE_HASH_EXPIRED"), true},
		{"private channel", errors.New("CHANNEL_PRIVATE"), true},
		{"already participant", errors.New("USER_ALREADY_PARTICIPANT"), false},
		{"approval request", errors.New("INVITE_REQUEST_SENT"), false},
		{"flood wait", errors.New("FLOOD_WAIT_120"), false},
		{"timeout", errors.New("context deadline exceeded"), false},
		{"temporary rpc", errors.New("INTERNAL"), false},
		{"temporary account restriction", errors.New("USER_RESTRICTED"), false},
		{"temporary channel capacity limit", errors.New("CHANNELS_TOO_MUCH"), false},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := isPermanentJoinFailure(tt.err); got != tt.want {
				t.Fatalf("isPermanentJoinFailure(%v)=%v, want %v", tt.err, got, tt.want)
			}
		})
	}
}

func TestWorkerHeartbeatRestartLifecycle(t *testing.T) {
	databaseURL := os.Getenv("TEST_DATABASE_URL")
	redisURL := os.Getenv("TEST_REDIS_URL")
	if databaseURL == "" || redisURL == "" {
		if os.Getenv("CI") == "true" {
			t.Fatal("TEST_DATABASE_URL and TEST_REDIS_URL are required in CI")
		}
		t.Skip("PostgreSQL and Redis integration services are not configured")
	}
	ctx, cancel := context.WithTimeout(context.Background(), 20*time.Second)
	defer cancel()
	store, err := storage.Open(ctx, databaseURL, redisURL)
	if err != nil {
		t.Fatalf("open integration services: %v", err)
	}
	defer store.Close()

	instanceID := fmt.Sprintf("phase3-restart-%d", time.Now().UnixNano())
	defer store.DB.ExecContext(context.Background(), "DELETE FROM worker_instances WHERE instance_id=$1", instanceID)

	if err := recordWorkerHeartbeat(ctx, store.DB, instanceID, "ci-host", 101, true); err != nil {
		t.Fatalf("record initial worker start: %v", err)
	}
	var status string
	var originalStarted time.Time
	var stoppedAt *time.Time
	if err := store.DB.QueryRowContext(ctx, "SELECT status,started_at,stopped_at FROM worker_instances WHERE instance_id=$1", instanceID).Scan(&status, &originalStarted, &stoppedAt); err != nil {
		t.Fatal(err)
	}
	if status != "running" || stoppedAt != nil {
		t.Fatalf("initial worker state status=%q stopped_at=%v", status, stoppedAt)
	}

	if err := recordWorkerHeartbeat(ctx, store.DB, instanceID, "ci-host", 101, false); err != nil {
		t.Fatalf("record graceful worker stop: %v", err)
	}
	if err := store.DB.QueryRowContext(ctx, "SELECT status,stopped_at FROM worker_instances WHERE instance_id=$1", instanceID).Scan(&status, &stoppedAt); err != nil {
		t.Fatal(err)
	}
	if status != "stopped" || stoppedAt == nil {
		t.Fatalf("stopped worker state status=%q stopped_at=%v", status, stoppedAt)
	}

	if err := recordWorkerHeartbeat(ctx, store.DB, instanceID, "ci-host", 202, true); err != nil {
		t.Fatalf("record worker restart: %v", err)
	}
	var restartedStarted time.Time
	if err := store.DB.QueryRowContext(ctx, "SELECT status,started_at,stopped_at FROM worker_instances WHERE instance_id=$1", instanceID).Scan(&status, &restartedStarted, &stoppedAt); err != nil {
		t.Fatal(err)
	}
	if status != "running" || stoppedAt != nil {
		t.Fatalf("restarted worker state status=%q stopped_at=%v", status, stoppedAt)
	}
	if !restartedStarted.Equal(originalStarted) {
		t.Fatalf("worker started_at changed across restart: before=%s after=%s", originalStarted, restartedStarted)
	}
	var processID int
	if err := store.DB.QueryRowContext(ctx, "SELECT process_id FROM worker_instances WHERE instance_id=$1", instanceID).Scan(&processID); err != nil {
		t.Fatal(err)
	}
	if processID != 202 {
		t.Fatalf("restarted process id=%d, want 202", processID)
	}
}
