package messages

import "testing"

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
}
