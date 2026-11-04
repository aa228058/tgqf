package messages

import (
	"testing"
	"time"
)

func TestJoinTaskCooldownSecondsAddsBufferAfterCeiling(t *testing.T) {
	cases := []struct {
		name string
		wait time.Duration
		want int64
	}{
		{"exact seconds", 2 * time.Minute, 121},
		{"fractional second", 2*time.Minute + 100*time.Millisecond, 122},
		{"single second", time.Second, 2},
		{"zero", 0, 0},
		{"negative", -time.Second, 0},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			if got := joinTaskCooldownSeconds(tc.wait); got != tc.want {
				t.Fatalf("joinTaskCooldownSeconds(%s)=%d, want %d", tc.wait, got, tc.want)
			}
		})
	}
}

func TestJoinTaskRetryDelayIsBoundedExponential(t *testing.T) {
	cases := []struct {
		attempt int
		want    time.Duration
	}{
		{0, 30 * time.Second},
		{1, 30 * time.Second},
		{2, 60 * time.Second},
		{3, 2 * time.Minute},
		{4, 4 * time.Minute},
		{5, 8 * time.Minute},
		{6, 16 * time.Minute},
		{7, 30 * time.Minute},
		{20, 30 * time.Minute},
	}
	for _, tc := range cases {
		if got := joinTaskRetryDelay(tc.attempt); got != tc.want {
			t.Fatalf("joinTaskRetryDelay(%d)=%s, want %s", tc.attempt, got, tc.want)
		}
	}
}

func TestNormalizeJoinApprovalResult(t *testing.T) {
	cases := []struct {
		status, wantTask, wantApplication string
		terminal                          bool
	}{
		{"member", "joined", "approved", true},
		{"administrator", "joined", "approved", true},
		{"creator", "joined", "approved", true},
		{"owner", "joined", "approved", true},
		{"banned", "failed", "unknown", true},
		{"left", "failed", "unknown", true},
		{"invalid", "failed", "unknown", true},
		{"expired", "failed", "unknown", true},
		{"request_pending", "request_pending", "request_pending", false},
		{"valid", "request_pending", "request_pending", false},
		{"unknown", "request_pending", "request_pending", false},
	}
	for _, tc := range cases {
		t.Run(tc.status, func(t *testing.T) {
			task, application, terminal := normalizeJoinApprovalResult(tc.status)
			if task != tc.wantTask || application != tc.wantApplication || terminal != tc.terminal {
				t.Fatalf("got (%q,%q,%v), want (%q,%q,%v)", task, application, terminal, tc.wantTask, tc.wantApplication, tc.terminal)
			}
		})
	}
}

func TestNormalizeJoinApprovalResultIsCaseInsensitive(t *testing.T) {
	task, application, terminal := normalizeJoinApprovalResult(" MEMBER ")
	if task != "joined" || application != "approved" || !terminal {
		t.Fatalf("unexpected normalized result: %q %q %v", task, application, terminal)
	}
}
