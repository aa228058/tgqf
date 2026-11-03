package accounts

import (
	"crypto/aes"
	"crypto/cipher"
	"crypto/rand"
	"encoding/base64"
	"encoding/hex"
	"errors"
	"os"
	"strings"
)

func encryptionKey() ([]byte, error) {
	raw := strings.TrimSpace(os.Getenv("SESSION_ENCRYPTION_KEY"))
	if raw == "" {
		return nil, errors.New("SESSION_ENCRYPTION_KEY is not configured")
	}
	if b, e := base64.StdEncoding.DecodeString(raw); e == nil && len(b) == 32 {
		return b, nil
	}
	if b, e := base64.RawStdEncoding.DecodeString(raw); e == nil && len(b) == 32 {
		return b, nil
	}
	if b, e := hex.DecodeString(raw); e == nil && len(b) == 32 {
		return b, nil
	}
	return nil, errors.New("SESSION_ENCRYPTION_KEY must be a 32-byte key encoded as base64 or hex")
}

func encrypt(plain []byte) ([]byte, error) {
	key, err := encryptionKey()
	if err != nil {
		return nil, err
	}
	block, err := aes.NewCipher(key)
	if err != nil {
		return nil, err
	}
	gcm, err := cipher.NewGCM(block)
	if err != nil {
		return nil, err
	}
	nonce := make([]byte, gcm.NonceSize())
	if _, err = rand.Read(nonce); err != nil {
		return nil, err
	}
	return gcm.Seal(nonce, nonce, plain, nil), nil
}

// decrypt reverses encrypt's nonce-prefixed AES-256-GCM representation.
// Authentication failures are returned without exposing plaintext or key material.
func decrypt(ciphertext []byte) ([]byte, error) {
	key, err := encryptionKey()
	if err != nil {
		return nil, err
	}
	block, err := aes.NewCipher(key)
	if err != nil {
		return nil, err
	}
	gcm, err := cipher.NewGCM(block)
	if err != nil {
		return nil, err
	}
	if len(ciphertext) < gcm.NonceSize()+gcm.Overhead() {
		return nil, errors.New("encrypted value is too short")
	}
	nonce := ciphertext[:gcm.NonceSize()]
	plaintext, err := gcm.Open(nil, nonce, ciphertext[gcm.NonceSize():], nil)
	if err != nil {
		return nil, errors.New("unable to decrypt encrypted value")
	}
	return plaintext, nil
}
