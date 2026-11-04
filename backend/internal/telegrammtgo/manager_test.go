package telegrammtgo

import (
	"testing"
	"time"
)

func TestExpirePhoneLoginRemovesOnlyMatchingPendingChallenge(t *testing.T) {
	manager := NewManager(RuntimeConfig{})
	entry := &pendingPhone{expiresAt: time.Now().Add(-time.Second)}
	manager.pendingPhone["account-a"] = entry
	manager.expirePhoneLogin("account-a", entry)
	if manager.pendingPhone["account-a"] != nil {
		t.Fatal("expired phone challenge remained registered")
	}
}

func TestExpireQRLoginRemovesOnlyMatchingPendingChallenge(t *testing.T) {
	manager := NewManager(RuntimeConfig{})
	entry := &pendingQR{expiresAt: time.Now().Add(-time.Second)}
	manager.pendingQR["account-a"] = entry
	manager.expireQRLogin("account-a", entry)
	if manager.pendingQR["account-a"] != nil {
		t.Fatal("expired QR challenge remained registered")
	}
}

func TestOldLoginExpiryDoesNotRemoveReplacementChallenge(t *testing.T) {
	manager := NewManager(RuntimeConfig{})
	oldEntry := &pendingPhone{expiresAt: time.Now().Add(-time.Second)}
	newEntry := &pendingPhone{expiresAt: time.Now().Add(time.Minute)}
	manager.pendingPhone["account-a"] = newEntry
	manager.expirePhoneLogin("account-a", oldEntry)
	if manager.pendingPhone["account-a"] != newEntry {
		t.Fatal("old expiry removed a replacement login challenge")
	}
}
