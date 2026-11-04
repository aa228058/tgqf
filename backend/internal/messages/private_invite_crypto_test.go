package messages

import (
	"bytes"
	"encoding/base64"
	"testing"
)

func TestPrivateInviteCryptoRoundTripAndAuthentication(t *testing.T) {
	t.Setenv("SESSION_ENCRYPTION_KEY", base64.StdEncoding.EncodeToString(bytes.Repeat([]byte{7}, 32)))
	encrypted, err := encryptJoinInviteHash("AbCdEfGh123")
	if err != nil {
		t.Fatal(err)
	}
	if bytes.Contains(encrypted, []byte("AbCdEfGh123")) {
		t.Fatal("invite hash stored in plaintext")
	}
	plain, err := decryptJoinInviteHash(encrypted)
	if err != nil {
		t.Fatal(err)
	}
	if plain != "AbCdEfGh123" {
		t.Fatalf("got %q", plain)
	}
	encrypted[len(encrypted)-1] ^= 1
	if _, err := decryptJoinInviteHash(encrypted); err == nil {
		t.Fatal("tampered ciphertext passed authentication")
	}
}

func TestPrivateInviteCryptoRequiresConfiguredKey(t *testing.T) {
	t.Setenv("SESSION_ENCRYPTION_KEY", "")
	if _, err := encryptJoinInviteHash("AbCdEfGh123"); err == nil {
		t.Fatal("expected missing encryption key error")
	}
}
