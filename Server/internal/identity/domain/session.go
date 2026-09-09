// Package domain defines identity business concepts without infrastructure dependencies.
package domain

import (
	"time"

	"github.com/google/uuid"
)

// Session represents a server-side browser session. Secrets are never exposed after creation.
type Session struct {
	ID                     uuid.UUID
	UserID                 uuid.UUID
	TokenHash              []byte
	CSRFTokenHash          []byte
	RefreshTokenCiphertext []byte
	IdentityVerifiedAt     time.Time
	CreatedAt              time.Time
	LastSeenAt             time.Time
	IdleExpiresAt          time.Time
	AbsoluteExpiresAt      time.Time
	RevokedAt              *time.Time
	AuthenticationMethod   string
}

// ActiveAt reports whether the session is valid at the supplied time.
func (s Session) ActiveAt(now time.Time) bool {
	return s.RevokedAt == nil && now.Before(s.IdleExpiresAt) && now.Before(s.AbsoluteExpiresAt)
}
