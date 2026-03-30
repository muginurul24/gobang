package crypto

import (
	"crypto/aes"
	"crypto/cipher"
	"crypto/rand"
	"crypto/sha256"
	"encoding/base64"
	"fmt"
)

type Sealer struct {
	key [32]byte
}

func NewSealer(secret string) *Sealer {
	return &Sealer{key: sha256.Sum256([]byte(secret))}
}

func (s *Sealer) Seal(plain string) (string, error) {
	block, err := aes.NewCipher(s.key[:])
	if err != nil {
		return "", fmt.Errorf("create cipher: %w", err)
	}

	gcm, err := cipher.NewGCM(block)
	if err != nil {
		return "", fmt.Errorf("create gcm: %w", err)
	}

	nonce := make([]byte, gcm.NonceSize())
	if _, err := rand.Read(nonce); err != nil {
		return "", fmt.Errorf("generate nonce: %w", err)
	}

	sealed := gcm.Seal(nonce, nonce, []byte(plain), nil)
	return base64.RawStdEncoding.EncodeToString(sealed), nil
}

func (s *Sealer) Open(cipherText string) (string, error) {
	payload, err := base64.RawStdEncoding.DecodeString(cipherText)
	if err != nil {
		return "", fmt.Errorf("decode cipher text: %w", err)
	}

	block, err := aes.NewCipher(s.key[:])
	if err != nil {
		return "", fmt.Errorf("create cipher: %w", err)
	}

	gcm, err := cipher.NewGCM(block)
	if err != nil {
		return "", fmt.Errorf("create gcm: %w", err)
	}

	if len(payload) < gcm.NonceSize() {
		return "", fmt.Errorf("invalid cipher payload")
	}

	nonce := payload[:gcm.NonceSize()]
	encrypted := payload[gcm.NonceSize():]

	plain, err := gcm.Open(nil, nonce, encrypted, nil)
	if err != nil {
		return "", fmt.Errorf("decrypt cipher text: %w", err)
	}

	return string(plain), nil
}
