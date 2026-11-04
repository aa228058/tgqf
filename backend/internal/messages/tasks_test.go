package messages

import (
	"strings"
	"testing"
)

func TestNormalizeUnifiedTaskFilter(t *testing.T) {
	filter, err := normalizeUnifiedTaskFilter(UnifiedTaskFilter{Type: " PROFILE_UPDATE ", Limit: 500, Offset: -1})
	if err != nil {
		t.Fatal(err)
	}
	if filter.Type != "profile_update" || filter.Limit != 50 || filter.Offset != 0 {
		t.Fatalf("unexpected normalized filter: %+v", filter)
	}
	if _, err := normalizeUnifiedTaskFilter(UnifiedTaskFilter{Type: "unknown"}); err == nil {
		t.Fatal("expected invalid task type to be rejected")
	}
	if _, err := normalizeUnifiedTaskFilter(UnifiedTaskFilter{AccountID: "not-a-uuid"}); err == nil {
		t.Fatal("expected invalid account id to be rejected")
	}
	if _, err := normalizeUnifiedTaskFilter(UnifiedTaskFilter{Offset: 10000001}); err == nil {
		t.Fatal("expected excessive pagination offset to be rejected")
	}
}


func TestUnifiedTaskActionsMatchQueueTransitions(t *testing.T) {
	tests := []struct {
		taskType, status string
		pause, resume, retry, cancel bool
	}{
		{"message_reply", "pending", true, false, false, true},
		{"message_reply", "processing", false, false, false, false},
		{"message_reply", "failed", true, false, true, true},
		{"message_reply", "paused", false, true, false, true},
		{"message_reply", "sent", false, false, false, false},
		{"keyword_action", "pending", true, false, false, true},
		{"profile_update", "pending", true, false, false, true},
		{"profile_update", "failed", false, false, true, false},
		{"profile_update", "paused", false, true, false, true},
		{"group_join", "queued", true, false, false, true},
		{"group_join", "processing", true, false, false, true},
		{"group_join", "request_pending", false, false, false, true},
		{"group_join", "failed", false, false, true, false},
		{"group_join", "paused", false, true, false, true},
		{"unknown", "pending", false, false, false, false},
	}
	for _, test := range tests {
		pause, resume, retry, cancel := unifiedTaskActions(test.taskType, test.status)
		if pause != test.pause || resume != test.resume || retry != test.retry || cancel != test.cancel {
			t.Errorf("%s/%s: got pause=%v resume=%v retry=%v cancel=%v",
				test.taskType, test.status, pause, resume, retry, cancel)
		}
	}
}


func TestNormalizeUnifiedTaskFilterSearchQuery(t *testing.T) {
	filter, err := normalizeUnifiedTaskFilter(UnifiedTaskFilter{Query: "  @Example Group  "})
	if err != nil {
		t.Fatal(err)
	}
	if filter.Query != "@Example Group" {
		t.Fatalf("query was not trimmed: %q", filter.Query)
	}
	if _, err := normalizeUnifiedTaskFilter(UnifiedTaskFilter{Query: strings.Repeat("x", 121)}); err == nil {
		t.Fatal("expected oversized task search query to be rejected")
	}
}
