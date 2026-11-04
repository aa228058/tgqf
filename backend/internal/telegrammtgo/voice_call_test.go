package telegrammtgo

import (
	"crypto/sha256"
	"testing"
)

func TestTelegramCallGAHashUsesOnlyPublicDHValue(t *testing.T) {
	gA := make([]byte, 256)
	for i := range gA {
		gA[i] = byte(i)
	}
	got := telegramCallGAHash(gA)
	want := sha256.Sum256(gA)
	if got != want {
		t.Fatal("phone.requestCall g_a_hash must be SHA-256 of the 256-byte g_a value")
	}
	extra := make([]byte, 256)
	for i := range extra {
		extra[i] = byte(255 - i)
	}
	combined := append(append([]byte(nil), gA...), extra...)
	if got == sha256.Sum256(combined) {
		t.Fatal("g_a_hash unexpectedly includes unrelated random bytes")
	}
}
