package application

import (
	"crypto/hmac"
	"crypto/rand"
	"crypto/sha256"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"time"
)

// LoginState carries the short-lived correlation and PKCE verifier for one login attempt.
type LoginState struct {
	State     string    `json:"state"`
	Verifier  string    `json:"verifier"`
	Nonce     string    `json:"nonce"`
	ExpiresAt time.Time `json:"expiresAt"`
}

// LoginStateCodec signs login state so callback processing can reject tampering and expiry.
type LoginStateCodec struct {
	key   []byte
	clock Clock
	ttl   time.Duration
}

// NewLoginStateCodec creates a codec from the configured session secret.
func NewLoginStateCodec(key []byte, clock Clock, ttl time.Duration) (*LoginStateCodec, error) {
	if len(key) < 32 {
		return nil, fmt.Errorf("login state key must contain at least 32 bytes")
	}
	if clock == nil || ttl <= 0 {
		return nil, fmt.Errorf("clock and positive login state TTL are required")
	}
	return &LoginStateCodec{key: key, clock: clock, ttl: ttl}, nil
}

// New creates a signed cookie value and the corresponding OIDC state value.
func (c *LoginStateCodec) New() (string, string, string, string, error) {
	state, err := randomValue(32)
	if err != nil {
		return "", "", "", "", err
	}
	verifier, err := randomValue(48)
	if err != nil {
		return "", "", "", "", err
	}
	nonce, err := randomValue(32)
	if err != nil {
		return "", "", "", "", err
	}
	payload, err := json.Marshal(LoginState{State: state, Verifier: verifier, Nonce: nonce, ExpiresAt: c.clock.Now().UTC().Add(c.ttl)})
	if err != nil {
		return "", "", "", "", fmt.Errorf("marshal login state: %w", err)
	}
	signature := c.sign(payload)
	return base64.RawURLEncoding.EncodeToString(payload) + "." + base64.RawURLEncoding.EncodeToString(signature), state, verifier, nonce, nil
}

// Verify validates the cookie signature, expiry, and callback state value.
func (c *LoginStateCodec) Verify(cookieValue, expectedState string) (LoginState, error) {
	parts := splitCookieValue(cookieValue)
	if len(parts) != 2 {
		return LoginState{}, fmt.Errorf("login state cookie is malformed")
	}
	payload, err := base64.RawURLEncoding.DecodeString(parts[0])
	if err != nil {
		return LoginState{}, fmt.Errorf("decode login state: %w", err)
	}
	signature, err := base64.RawURLEncoding.DecodeString(parts[1])
	if err != nil {
		return LoginState{}, fmt.Errorf("decode login state signature: %w", err)
	}
	if !hmac.Equal(signature, c.sign(payload)) {
		return LoginState{}, fmt.Errorf("login state signature is invalid")
	}
	var state LoginState
	if err := json.Unmarshal(payload, &state); err != nil {
		return LoginState{}, fmt.Errorf("unmarshal login state: %w", err)
	}
	if !c.clock.Now().UTC().Before(state.ExpiresAt) {
		return LoginState{}, fmt.Errorf("login state has expired")
	}
	if !hmac.Equal([]byte(state.State), []byte(expectedState)) {
		return LoginState{}, fmt.Errorf("login state does not match callback")
	}
	return state, nil
}

func (c *LoginStateCodec) sign(payload []byte) []byte {
	mac := hmac.New(sha256.New, c.key)
	_, _ = mac.Write(payload)
	return mac.Sum(nil)
}
func randomValue(size int) (string, error) {
	bytes := make([]byte, size)
	if _, err := rand.Read(bytes); err != nil {
		return "", fmt.Errorf("generate login state: %w", err)
	}
	return base64.RawURLEncoding.EncodeToString(bytes), nil
}
func splitCookieValue(value string) []string {
	for index := range value {
		if value[index] == '.' {
			return []string{value[:index], value[index+1:]}
		}
	}
	return nil
}
