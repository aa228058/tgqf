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
}
