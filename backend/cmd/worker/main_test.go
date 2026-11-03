package main

import (
	"testing"
	"time"
)

func TestReconnectDelay(t *testing.T) {
	tests := []struct {
		attempt int
		want time.Duration
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
