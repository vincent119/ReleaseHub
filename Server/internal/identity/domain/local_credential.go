package domain

import "github.com/google/uuid"

// LocalCredential contains password-verification state without exposing plaintext credentials.
type LocalCredential struct {
	UserID             uuid.UUID
	Username           string
	PasswordHash       []byte
	MustChangePassword bool
	Disabled           bool
}
