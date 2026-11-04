package accounts

import (
	"strings"
	"testing"
)

func TestNormalizeAccountListOptions(t *testing.T) {
	got, err := normalizeAccountListOptions(AccountListOptions{Query: "  Alice  ", GroupName: " work ", Status: " AUTHORIZED ", Limit: 0, Offset: -2})
	if err != nil {
		t.Fatal(err)
	}
	if got.Query != "Alice" || got.GroupName != "work" || got.Status != "authorized" || got.Limit != 50 || got.Offset != 0 {
		t.Fatalf("normalized options = %+v", got)
	}
	if _, err := normalizeAccountListOptions(AccountListOptions{Status: "unknown"}); err == nil {
		t.Fatal("unknown status should be rejected")
	}
	if _, err := normalizeAccountListOptions(AccountListOptions{Query: string(make([]byte, 121))}); err == nil {
		t.Fatal("oversized query should be rejected")
	}
}

func TestNormalizeAccountTags(t *testing.T) {
	got, err := normalizeAccountTags([]string{" Work ", "sales", "work", "客户"})
	if err != nil {
		t.Fatal(err)
	}
	want := []string{"Work", "sales", "客户"}
	if len(got) != len(want) {
		t.Fatalf("tags = %#v, want %#v", got, want)
	}
	for i := range want {
		if got[i] != want[i] {
			t.Fatalf("tags = %#v, want %#v", got, want)
		}
	}
	if cleared, err := normalizeAccountTags(nil); err != nil || len(cleared) != 0 {
		t.Fatalf("empty tags should clear: %#v %v", cleared, err)
	}
	for _, input := range [][]string{{""}, {strings.Repeat("x", 33)}, make([]string, 21), {"bad\ntag"}} {
		if _, err := normalizeAccountTags(input); err == nil {
			t.Errorf("invalid tags accepted: %#v", input)
		}
	}
}

func TestNormalizeAccountDisplayName(t *testing.T) {
	got, err := normalizeAccountDisplayName("  客户账号 A  ")
	if err != nil || got != "客户账号 A" {
		t.Fatalf("normalized display name = %q, %v", got, err)
	}
	if got, err := normalizeAccountDisplayName("   "); err != nil || got != "" {
		t.Fatalf("empty display name should clear: %q, %v", got, err)
	}
	if _, err := normalizeAccountDisplayName(strings.Repeat("名", 121)); err == nil {
		t.Fatal("oversized display name accepted")
	}
	if _, err := normalizeAccountDisplayName("bad\nname"); err == nil {
		t.Fatal("control characters accepted")
	}
}
