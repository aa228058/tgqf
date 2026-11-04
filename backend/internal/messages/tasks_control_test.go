package messages

import (
	"context"
	"testing"
)

func TestUnifiedTaskControlRejectsInvalidInputBeforeDatabaseAccess(t *testing.T) {
	service := &Service{}
	if err := service.ChangeUnifiedTaskStatus(context.Background(), "", "message_reply", "id", "pause"); err == nil {
		t.Fatal("expected missing tenant to be rejected")
	}
	if err := service.ChangeUnifiedTaskStatus(context.Background(), "tenant", "message_reply", "id", "execute"); err == nil {
		t.Fatal("expected unsupported action to be rejected")
	}
	if err := service.ChangeUnifiedTaskStatus(context.Background(), "tenant", "group_join", "not-a-row-id", "cancel"); err == nil {
		t.Fatal("expected malformed group-join child ID to be rejected before database access")
	}
	if err := service.ChangeUnifiedTaskStatus(context.Background(), "tenant", "message_reply", "not-a-uuid", "cancel"); err == nil {
		t.Fatal("expected malformed UUID task ID to be rejected before database access")
	}
	if err := service.ChangeUnifiedTaskStatus(context.Background(), "tenant", "unknown", "550e8400-e29b-41d4-a716-446655440000", "cancel"); err == nil {
		t.Fatal("expected unsupported task type to be rejected before database access")
	}
}

func TestDeriveJoinTaskParentStatus(t *testing.T) {
	tests := []struct {
		name     string
		children []string
		want     string
	}{
		{"active child keeps parent queued", []string{"joined", "queued"}, "queued"},
		{"approval remains active", []string{"request_pending", "joined"}, "queued"},
		{"paused children pause parent", []string{"paused", "joined"}, "paused"},
		{"queued child takes precedence over paused", []string{"paused", "queued"}, "queued"},
		{"failed terminal child fails parent", []string{"joined", "failed"}, "failed"},
		{"all cancelled children cancel parent", []string{"cancelled", "cancelled"}, "cancelled"},
		{"successful terminal children complete parent", []string{"joined", "already_member", "skipped"}, "completed"},
		{"unknown state is conservatively active", []string{"future_state"}, "queued"},
		{"empty task is not left queued", nil, "completed"},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			if got := deriveJoinTaskParentStatus(test.children); got != test.want {
				t.Fatalf("deriveJoinTaskParentStatus(%v) = %q, want %q", test.children, got, test.want)
			}
		})
	}
}
