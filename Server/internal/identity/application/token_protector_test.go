package application_test

import (
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/vincent119/ReleaseHub/Server/internal/identity/application"
)

func TestTokenProtectorEncryptsRefreshTokenAndRejectsTampering(t *testing.T) {
	protector, err := application.NewTokenProtector([]byte("12345678901234567890123456789012"))
	require.NoError(t, err)
	ciphertext, err := protector.Encrypt("refresh-token")
	require.NoError(t, err)
	require.NotContains(t, string(ciphertext), "refresh-token")
	plaintext, err := protector.Decrypt(ciphertext)
	require.NoError(t, err)
	require.Equal(t, "refresh-token", plaintext)
	ciphertext[len(ciphertext)-1] ^= 1
	_, err = protector.Decrypt(ciphertext)
	require.Error(t, err)
}
