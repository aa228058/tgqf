package telegrammtgo

import (
	"encoding/hex"
	"testing"
)

func TestVoiceCallGAHashUsesOnlyGA(t *testing.T) {
	gA := make([]byte, 256)
	for i := range gA {
		gA[i] = byte(i)
	}

	got := voiceCallGAHash(gA)
	const want = "40aff2e9d2d8922e47afd4648e6967497158785fbd1da870e7110266bf944880"
	if hex.EncodeToString(got[:]) != want {
		t.Fatalf("voiceCallGAHash() = %x, want %s", got, want)
	}
}
