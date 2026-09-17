package domain

import (
	"encoding/base64"
	"encoding/json"
	"errors"
	"testing"
	"time"

	"github.com/google/uuid"
)

func TestCursorCodecRoundTripAndRejectsTampering(t *testing.T) {
	codec, err := NewCursorCodec([]byte("0123456789abcdef0123456789abcdef"))
	if err != nil {
		t.Fatalf("create codec: %v", err)
	}
	position := CursorPosition{OccurredAt: time.Date(2026, 9, 17, 1, 2, 3, 0, time.UTC), EventID: uuid.New()}
	encoded, err := codec.Encode(position, "filter-a")
	if err != nil {
		t.Fatalf("encode cursor: %v", err)
	}
	decoded, err := codec.Decode(encoded, "filter-a")
	if err != nil || decoded.EventID != position.EventID || !decoded.OccurredAt.Equal(position.OccurredAt) {
		t.Fatalf("decoded cursor = %#v, %v", decoded, err)
	}
	for name, value := range map[string]struct {
		value       string
		fingerprint string
	}{
		"tampered":        {encoded + "x", "filter-a"},
		"filter mismatch": {encoded, "filter-b"},
		"malformed":       {"not-a-cursor", "filter-a"},
	} {
		t.Run(name, func(t *testing.T) {
			if _, decodeErr := codec.Decode(value.value, value.fingerprint); !errors.Is(decodeErr, ErrInvalidCursor) {
				t.Fatalf("decode error = %v", decodeErr)
			}
		})
	}
	unsupported, _ := json.Marshal(cursorPayload{Version: cursorVersion + 1, OccurredAt: position.OccurredAt, EventID: position.EventID, Fingerprint: "filter-a"})
	unsupportedCursor := base64.RawURLEncoding.EncodeToString(unsupported) + "." + base64.RawURLEncoding.EncodeToString(codec.sign(unsupported))
	if _, decodeErr := codec.Decode(unsupportedCursor, "filter-a"); !errors.Is(decodeErr, ErrInvalidCursor) {
		t.Fatalf("unsupported version error = %v", decodeErr)
	}
}
