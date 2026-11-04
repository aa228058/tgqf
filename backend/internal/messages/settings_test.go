package messages

import "testing"

func TestCollectionSettingsAllows(t *testing.T) {
	base := DefaultCollectionSettings()
	in := Incoming{AccountID: "8f14e45f-ea3e-4e3d-9c2d-5a9f6b8a1234", ChatID: -100123, Text: "Please call me tomorrow"}
	if !base.Allows(in) {
		t.Fatal("default settings should allow incoming text")
	}
	base.IncludeAllAccounts = false
	if base.Allows(in) {
		t.Fatal("account should be rejected when no account is selected")
	}
	base.AccountIDs = []string{in.AccountID}
	if !base.Allows(in) {
		t.Fatal("selected account should be allowed")
	}
	base.ChatIDs = []int64{-100456}
	if base.Allows(in) {
		t.Fatal("unselected group should be rejected")
	}
	base.ChatIDs = []int64{-100123}
	base.Keywords = []string{"CALL"}
	if !base.Allows(in) {
		t.Fatal("keyword matching should be case-insensitive")
	}
	base.Keywords = []string{"invoice"}
	if base.Allows(in) {
		t.Fatal("non-matching keyword should be rejected")
	}
	base.Keywords = []string{}
	base.IncludeText = false
	if base.Allows(in) {
		t.Fatal("text should be rejected when text collection is disabled")
	}
	in.MediaType = "photo"
	if !base.Allows(in) {
		t.Fatal("media should be allowed when media collection is enabled")
	}
	base.Enabled = false
	if base.Allows(in) {
		t.Fatal("disabled collection must reject all messages")
	}
}

func TestValidateCollectionSettings(t *testing.T) {
	settings := DefaultCollectionSettings()
	settings.Keywords = []string{"  hello "}
	got, err := validateSettings(settings)
	if err != nil {
		t.Fatalf("valid settings rejected: %v", err)
	}
	if got.Keywords[0] != "hello" {
		t.Fatalf("keyword was not normalized: %q", got.Keywords[0])
	}
	settings.AccountIDs = []string{"not-a-uuid"}
	if _, err := validateSettings(settings); err == nil {
		t.Fatal("invalid account id accepted")
	}
}
