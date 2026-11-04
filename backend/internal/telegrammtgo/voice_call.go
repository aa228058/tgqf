package telegrammtgo

import (
	"context"
	"crypto/rand"
	"crypto/sha256"
	"encoding/binary"
	"errors"
	"fmt"
	"math/big"
	"strings"

	"github.com/mtgo-labs/mtgo/tg"
)

// VoiceCallRequestResult describes the Telegram-side result of starting an
// outgoing native voice call. It deliberately does not claim that audio is
// connected: this operation only starts the native ringing flow.
type VoiceCallRequestResult struct {
	CallID int64
	AccessHash int64
	State string
}

// RequestVoiceCall asks Telegram to place a native one-to-one voice call.
// The target must already be resolvable in this account's MTGo peer cache;
// callers must not pass arbitrary user IDs obtained from untrusted input.
func (m *Manager) RequestVoiceCall(ctx context.Context, accountID string, targetUserID int64) (VoiceCallRequestResult, error) {
	accountID = strings.TrimSpace(accountID)
	if accountID == "" || targetUserID <= 0 {
		return VoiceCallRequestResult{}, errors.New("account and target Telegram user are required")
	}

	m.mu.RLock()
	client := m.clients[accountID]
	m.mu.RUnlock()
	if client == nil || !client.IsConnected() {
		return VoiceCallRequestResult{}, errors.New("Telegram account is not connected")
	}

	peer, err := client.ResolvePeerCache(targetUserID)
	if err != nil {
		return VoiceCallRequestResult{}, fmt.Errorf("target user is not in this account's Telegram peer cache: %w", err)
	}
	userPeer, ok := peer.(*tg.InputPeerUser)
	if !ok || userPeer.UserID != targetUserID || userPeer.AccessHash == 0 {
		return VoiceCallRequestResult{}, errors.New("target must resolve to a Telegram user with a valid access hash")
	}

	// Telegram requires a DH-derived g_a_hash in phone.requestCall. We obtain
	// the current Telegram DH parameters instead of inventing local constants.
	dh, err := client.Raw().MessagesGetDHConfig(ctx, &tg.MessagesGetDHConfigRequest{Version: 0, RandomLength: 256})
	if err != nil {
		return VoiceCallRequestResult{}, fmt.Errorf("get Telegram call DH configuration: %w", err)
	}
	params, ok := dh.(*tg.MessagesDHConfig)
	if !ok || len(params.P) == 0 || params.G < 2 {
		return VoiceCallRequestResult{}, fmt.Errorf("Telegram returned unsupported DH configuration %T", dh)
	}
	p := new(big.Int).SetBytes(params.P)
	if p.BitLen() < 2048 || !p.ProbablyPrime(20) {
		return VoiceCallRequestResult{}, errors.New("Telegram returned an invalid or undersized DH prime")
	}
	generatorOK := false
	mod := func(n int64) int64 { return new(big.Int).Mod(p, big.NewInt(n)).Int64() }
	switch params.G {
	case 2:
		generatorOK = mod(8) == 7
	case 3:
		generatorOK = mod(3) == 2
	case 4:
		generatorOK = true
	case 5:
		generatorOK = mod(5) == 1 || mod(5) == 4
	case 6:
		generatorOK = mod(24) == 19 || mod(24) == 23
	case 7:
		generatorOK = mod(7) == 3 || mod(7) == 5 || mod(7) == 6
	}
	if !generatorOK {
		return VoiceCallRequestResult{}, errors.New("Telegram returned an invalid DH generator")
	}

	// Generate a fresh ephemeral exponent and the corresponding g_a value.
	// The exponent is intentionally not persisted or logged. This endpoint is
	// the dial/ring action only; it does not negotiate or expose media keys.
	aBytes := make([]byte, 256)
	if _, err := rand.Read(aBytes); err != nil {
		return VoiceCallRequestResult{}, fmt.Errorf("generate call DH exponent: %w", err)
	}
	a := new(big.Int).SetBytes(aBytes)
	if a.Sign() == 0 {
		a.SetInt64(1)
	}
	g := big.NewInt(int64(params.G))
	gAValue := new(big.Int).Exp(g, a, p)
	if gAValue.Cmp(big.NewInt(1)) <= 0 || gAValue.Cmp(new(big.Int).Sub(p, big.NewInt(1))) >= 0 {
		return VoiceCallRequestResult{}, errors.New("generated invalid Telegram call DH value")
	}
	gABytes := gAValue.Bytes()
	if len(gABytes) > 256 {
		return VoiceCallRequestResult{}, errors.New("generated Telegram call DH value is oversized")
	}
	gA := make([]byte, 256)
	copy(gA[256-len(gABytes):], gABytes)

	nonce := make([]byte, 256)
	if _, err := rand.Read(nonce); err != nil {
		return VoiceCallRequestResult{}, fmt.Errorf("generate call nonce: %w", err)
	}
	hashInput := make([]byte, 0, len(gA)+len(nonce))
	hashInput = append(hashInput, gA...)
	hashInput = append(hashInput, nonce...)
	gaHash := sha256.Sum256(hashInput)

	var randomIDBytes [4]byte
	if _, err := rand.Read(randomIDBytes[:]); err != nil {
		return VoiceCallRequestResult{}, fmt.Errorf("generate call request ID: %w", err)
	}
	randomID := int32(binary.LittleEndian.Uint32(randomIDBytes[:]))
	if randomID == 0 {
		randomID = 1
	}

	protocol := &tg.PhoneCallProtocol{
		UDPP2p: true,
		UDPReflector: true,
		MinLayer: 65,
		MaxLayer: 92,
		LibraryVersions: []string{"2.4.4"},
	}
	call, err := client.Raw().PhoneRequestCall(ctx, &tg.PhoneRequestCallRequest{
		UserID: &tg.InputUser{UserID: userPeer.UserID, AccessHash: userPeer.AccessHash},
		RandomID: randomID,
		GAHash: gaHash[:],
		Protocol: protocol,
	})
	if err != nil {
		return VoiceCallRequestResult{}, fmt.Errorf("request Telegram native voice call: %w", err)
	}

	switch value := call.(type) {
	case *tg.PhoneCallRequested:
		return VoiceCallRequestResult{CallID: value.ID, AccessHash: value.AccessHash, State: "ringing"}, nil
	case *tg.PhoneCallWaiting:
		return VoiceCallRequestResult{CallID: value.ID, AccessHash: value.AccessHash, State: "ringing"}, nil
	default:
		return VoiceCallRequestResult{}, fmt.Errorf("Telegram returned unexpected outgoing call state %T", call)
	}
}
