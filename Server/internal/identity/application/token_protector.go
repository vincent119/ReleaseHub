package application

import (
	"crypto/aes"
	"crypto/cipher"
	"crypto/rand"
	"crypto/sha256"
	"fmt"
)

// TokenProtector encrypts provider credentials before persistence.
type TokenProtector struct{ aead cipher.AEAD }

// NewTokenProtector derives an AES-256-GCM key from the configured session secret.
func NewTokenProtector(secret []byte) (*TokenProtector, error) {
	if len(secret) < 32 {
		return nil, fmt.Errorf("token protection secret must contain at least 32 bytes")
	}
	key := sha256.Sum256(secret)
	block, err := aes.NewCipher(key[:])
	if err != nil {
		return nil, fmt.Errorf("create token cipher: %w", err)
	}
	aead, err := cipher.NewGCM(block)
	if err != nil {
		return nil, fmt.Errorf("create token protector: %w", err)
	}
	return &TokenProtector{aead: aead}, nil
}

// Encrypt seals a token with a random nonce.
func (p *TokenProtector) Encrypt(value string) ([]byte, error) {
	if value == "" {
		return nil, fmt.Errorf("provider refresh token is required")
	}
	nonce := make([]byte, p.aead.NonceSize())
	if _, err := rand.Read(nonce); err != nil {
		return nil, fmt.Errorf("generate token nonce: %w", err)
	}
	return p.aead.Seal(nonce, nonce, []byte(value), nil), nil
}

// Decrypt opens one persisted provider token.
func (p *TokenProtector) Decrypt(value []byte) (string, error) {
	if len(value) < p.aead.NonceSize() {
		return "", fmt.Errorf("encrypted provider token is invalid")
	}
	nonce, ciphertext := value[:p.aead.NonceSize()], value[p.aead.NonceSize():]
	plaintext, err := p.aead.Open(nil, nonce, ciphertext, nil)
	if err != nil {
		return "", fmt.Errorf("decrypt provider token: %w", err)
	}
	return string(plaintext), nil
}
