package domain

import (
	"crypto/hmac"
	"crypto/sha256"
	"encoding/base64"
	"encoding/json"
	"errors"
	"time"

	"github.com/google/uuid"
)

const cursorVersion = 1

var ErrInvalidCursor = errors.New("audit query cursor is invalid")

// CursorPosition is the exclusive keyset boundary for the next page.
type CursorPosition struct {
	OccurredAt time.Time
	EventID    uuid.UUID
}

// CursorCodec authenticates cursor positions and their normalized filter fingerprint.
type CursorCodec struct{ key []byte }

type cursorPayload struct {
	Version     int       `json:"version"`
	OccurredAt  time.Time `json:"occurredAt"`
	EventID     uuid.UUID `json:"eventId"`
	Fingerprint string    `json:"fingerprint"`
}

// NewCursorCodec creates an authenticated cursor codec.
func NewCursorCodec(key []byte) (*CursorCodec, error) {
	if len(key) < 32 {
		return nil, errors.New("audit cursor key must contain at least 32 bytes")
	}
	return &CursorCodec{key: append([]byte(nil), key...)}, nil
}

// Encode creates an opaque authenticated cursor.
func (c *CursorCodec) Encode(position CursorPosition, fingerprint string) (string, error) {
	if position.OccurredAt.IsZero() || position.EventID == uuid.Nil || fingerprint == "" {
		return "", ErrInvalidCursor
	}
	payload, err := json.Marshal(cursorPayload{cursorVersion, position.OccurredAt.UTC(), position.EventID, fingerprint})
	if err != nil {
		return "", ErrInvalidCursor
	}
	signature := c.sign(payload)
	return base64.RawURLEncoding.EncodeToString(payload) + "." + base64.RawURLEncoding.EncodeToString(signature), nil
}

// Decode verifies and decodes a cursor for the expected normalized filter.
func (c *CursorCodec) Decode(value, fingerprint string) (*CursorPosition, error) {
	if value == "" {
		return nil, nil
	}
	parts := splitCursor(value)
	if len(parts) != 2 {
		return nil, ErrInvalidCursor
	}
	payload, payloadErr := base64.RawURLEncoding.DecodeString(parts[0])
	signature, signatureErr := base64.RawURLEncoding.DecodeString(parts[1])
	if payloadErr != nil || signatureErr != nil || !hmac.Equal(signature, c.sign(payload)) {
		return nil, ErrInvalidCursor
	}
	var decoded cursorPayload
	if err := json.Unmarshal(payload, &decoded); err != nil || decoded.Version != cursorVersion || decoded.OccurredAt.IsZero() || decoded.EventID == uuid.Nil || decoded.Fingerprint != fingerprint {
		return nil, ErrInvalidCursor
	}
	return &CursorPosition{OccurredAt: decoded.OccurredAt.UTC(), EventID: decoded.EventID}, nil
}

func (c *CursorCodec) sign(payload []byte) []byte {
	mac := hmac.New(sha256.New, c.key)
	_, _ = mac.Write(payload)
	return mac.Sum(nil)
}

func splitCursor(value string) []string {
	for index := range value {
		if value[index] == '.' {
			return []string{value[:index], value[index+1:]}
		}
	}
	return nil
}
