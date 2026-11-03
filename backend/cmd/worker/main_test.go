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

func TestRuntimeConfigChanged(t *testing.T) {
	tests := []struct {
		name string
		currentSession string
		currentProxy string
		desiredSession string
		desiredProxy string
		want bool
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
