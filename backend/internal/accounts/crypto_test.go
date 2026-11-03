package accounts

import (
	"bytes"
	"encoding/base64"
	"testing"
)

func setTestEncryptionKey(t *testing.T, value byte) {
	t.Helper()
	key := bytes.Repeat([]byte{value}, 32)
	t.Setenv("SESSION_ENCRYPTION_KEY", base64.StdEncoding.EncodeToString(key))
}

func TestEncryptDecryptRoundTrip(t *testing.T) {
	setTestEncryptionKey(t, 7)
	plain := []byte("MTGO1.example-session-payload")
	encrypted, err := encrypt(plain)
	if err != nil {
		t.Fatalf("encrypt: %v", err)
	}
	if bytes.Equal(encrypted, plain) {
		t.Fatal("ciphertext must not equal plaintext")
	}
	got, err := decrypt(encrypted)
	if err != nil {
		t.Fatalf("decrypt: %v", err)
	}
	if !bytes.Equal(got, plain) {
		t.Fatalf("decrypted value mismatch: got %q, want %q", got, plain)
	}
}

func TestDecryptRejectsWrongKey(t *testing.T) {
	setTestEncryptionKey(t, 1)
	encrypted, err := encrypt([]byte("secret"))
	if err != nil {
		t.Fatalf("encrypt: %v", err)
	}
	setTestEncryptionKey(t, 2)
	if _, err := decrypt(encrypted); err == nil {
		t.Fatal("expected authentication failure for wrong key")
	}
}

func TestDecryptRejectsTruncatedCiphertext(t *testing.T) {
	setTestEncryptionKey(t, 3)
	if _, err := decrypt([]byte("short")); err == nil {
		t.Fatal("expected error for truncated ciphertext")
	}
}
