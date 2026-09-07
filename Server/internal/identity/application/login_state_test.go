package application_test

import (
	"testing"
	"time"

	"github.com/stretchr/testify/require"

	"github.com/vincent119/ReleaseHub/Server/internal/identity/application"
)

func TestLoginStateCodecRejectsTamperingExpiryAndWrongState(t *testing.T) {
	now := time.Date(2026, 9, 1, 0, 0, 0, 0, time.UTC)
	clock := &mutableClock{now: now}
	codec, err := application.NewLoginStateCodec([]byte("12345678901234567890123456789012"), clock, 5*time.Minute)
	require.NoError(t, err)
	cookieValue, state, verifier, nonce, err := codec.New()
	require.NoError(t, err)
	require.NotEmpty(t, verifier)
	require.NotEmpty(t, nonce)
	verified, err := codec.Verify(cookieValue, state)
	require.NoError(t, err)
	require.NotEmpty(t, verified.Verifier)
	_, err = codec.Verify(cookieValue, "unexpected")
	require.Error(t, err)
	_, err = codec.Verify(cookieValue+"x", state)
	require.Error(t, err)
	clock.now = now.Add(5 * time.Minute)
	_, err = codec.Verify(cookieValue, state)
	require.ErrorContains(t, err, "expired")
}

type mutableClock struct{ now time.Time }

func (c *mutableClock) Now() time.Time { return c.now }
