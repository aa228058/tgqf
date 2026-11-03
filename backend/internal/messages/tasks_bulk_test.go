package messages

import "testing"

func TestNormalizeBulkTaskIDs(t *testing.T) {
	valid := []string{"550e8400-e29b-41d4-a716-446655440000"}
	if ids, err := normalizeBulkTaskIDs("message_reply", "cancel", valid); err != nil || len(ids) != 1 {
		t.Fatalf("valid bulk request rejected: ids=%v err=%v", ids, err)
	}
	if _, err := normalizeBulkTaskIDs("message_reply", "cancel", []string{valid[0], valid[0]}); err == nil {
		t.Fatal("duplicate IDs must be rejected")
	}
	if _, err := normalizeBulkTaskIDs("group_join", "retry", []string{"not-a-number"}); err == nil {
		t.Fatal("invalid join child ID must be rejected")
	}
	if _, err := normalizeBulkTaskIDs("profile_update", "execute", valid); err == nil {
		t.Fatal("unsupported action must be rejected")
	}
	if _, err := normalizeBulkTaskIDs("profile_update", "cancel", make([]string, 101)); err == nil {
		t.Fatal("oversized batch must be rejected")
	}
}
