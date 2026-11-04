package messages

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

func encryptJoinInviteHash(plain string) ([]byte, error) {
	raw := strings.TrimSpace(os.Getenv("SESSION_ENCRYPTION_KEY"))
	var key []byte
	if decoded, err := base64.StdEncoding.DecodeString(raw); err == nil && len(decoded) == 32 {
		key = decoded
	}
	if key == nil {
		if decoded, err := base64.RawStdEncoding.DecodeString(raw); err == nil && len(decoded) == 32 {
			key = decoded
		}
	}
	if key == nil {
		if decoded, err := hex.DecodeString(raw); err == nil && len(decoded) == 32 {
			key = decoded
		}
	}
	if len(key) != 32 {
		return nil, errors.New("SESSION_ENCRYPTION_KEY must be configured to store private invite links")
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
	if _, err := rand.Read(nonce); err != nil {
		return nil, err
	}
	return gcm.Seal(nonce, nonce, []byte(plain), nil), nil
}

func decryptJoinInviteHash(ciphertext []byte) (string, error) {
	raw := strings.TrimSpace(os.Getenv("SESSION_ENCRYPTION_KEY"))
	var key []byte
	if decoded, err := base64.StdEncoding.DecodeString(raw); err == nil && len(decoded) == 32 {
		key = decoded
	}
	if key == nil {
		if decoded, err := base64.RawStdEncoding.DecodeString(raw); err == nil && len(decoded) == 32 {
			key = decoded
		}
	}
	if key == nil {
		if decoded, err := hex.DecodeString(raw); err == nil && len(decoded) == 32 {
			key = decoded
		}
	}
	if len(key) != 32 {
		return "", errors.New("SESSION_ENCRYPTION_KEY is not configured")
	}
	block, err := aes.NewCipher(key)
	if err != nil {
		return "", err
	}
	gcm, err := cipher.NewGCM(block)
	if err != nil {
		return "", err
	}
	if len(ciphertext) < gcm.NonceSize()+gcm.Overhead() {
		return "", errors.New("encrypted invite hash is invalid")
	}
	plain, err := gcm.Open(nil, ciphertext[:gcm.NonceSize()], ciphertext[gcm.NonceSize():], nil)
	if err != nil {
		return "", errors.New("unable to decrypt private invite hash")
	}
	return string(plain), nil
}
