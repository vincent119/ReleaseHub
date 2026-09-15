// Package application coordinates identity use cases.
package application

import (
	"context"
	"crypto/rand"
	"crypto/sha256"
	"crypto/subtle"
	"encoding/base64"
	"errors"
	"fmt"
	"time"

	"github.com/google/uuid"

	identity "github.com/vincent119/ReleaseHub/Server/internal/identity/domain"
)

var (
	// ErrSessionInvalid marks a terminal browser session state that requires a new login.
	ErrSessionInvalid = errors.New("session is invalid")
	// ErrCSRFInvalid marks a rejected state-changing request without changing session validity.
	ErrCSRFInvalid = errors.New("CSRF token is invalid")
)

// SessionRepository persists opaque browser sessions.
type SessionRepository interface {
	CreateSession(context.Context, identity.Session) error
	FindSessionByTokenHash(context.Context, []byte) (identity.Session, error)
	TouchSession(context.Context, uuid.UUID, time.Time, time.Time) error
	RevokeSession(context.Context, uuid.UUID, time.Time) error
	RevokeUserSessions(context.Context, uuid.UUID, time.Time) error
	UpdateProviderCredential(context.Context, uuid.UUID, []byte, time.Time) error
}

// RevokeUserSessions invalidates every active session for one local user.
func (s *SessionService) RevokeUserSessions(ctx context.Context, userID uuid.UUID) error {
	if err := s.repository.RevokeUserSessions(ctx, userID, s.clock.Now().UTC()); err != nil {
		return fmt.Errorf("revoke user sessions: %w", err)
	}
	return nil
}

// Clock makes session expiry deterministic in tests.
type Clock interface{ Now() time.Time }

// SystemClock uses the current UTC time in runtime composition.
type SystemClock struct{}

func (SystemClock) Now() time.Time { return time.Now().UTC() }

// SessionService owns opaque token issuance and validation.
type SessionService struct {
	repository  SessionRepository
	clock       Clock
	idleTimeout time.Duration
	absoluteTTL time.Duration
}

// NewSessionService creates a service with explicit expiry bounds.
func NewSessionService(repository SessionRepository, clock Clock, idleTimeout, absoluteTTL time.Duration) (*SessionService, error) {
	if repository == nil || clock == nil {
		return nil, fmt.Errorf("session repository and clock are required")
	}
	if idleTimeout <= 0 || absoluteTTL <= 0 || idleTimeout > absoluteTTL {
		return nil, fmt.Errorf("session expiry configuration is invalid")
	}
	return &SessionService{repository: repository, clock: clock, idleTimeout: idleTimeout, absoluteTTL: absoluteTTL}, nil
}

// Create issues opaque session and CSRF tokens; only their hashes are persisted.
func (s *SessionService) Create(ctx context.Context, userID uuid.UUID, refreshTokenCiphertext []byte) (sessionToken, csrfToken string, err error) {
	if len(refreshTokenCiphertext) == 0 {
		return "", "", fmt.Errorf("encrypted provider refresh token is required")
	}
	_, sessionToken, csrfToken, err = s.create(ctx, userID, refreshTokenCiphertext, "oidc")
	return sessionToken, csrfToken, err
}

// CreateLocal issues a session that does not depend on an OIDC refresh credential.
func (s *SessionService) CreateLocal(ctx context.Context, userID uuid.UUID) (session identity.Session, sessionToken, csrfToken string, err error) {
	return s.create(ctx, userID, nil, "local")
}

func (s *SessionService) create(ctx context.Context, userID uuid.UUID, refreshTokenCiphertext []byte, method string) (session identity.Session, sessionToken, csrfToken string, err error) {
	sessionToken, err = newOpaqueToken()
	if err != nil {
		return identity.Session{}, "", "", err
	}
	csrfToken, err = newOpaqueToken()
	if err != nil {
		return identity.Session{}, "", "", err
	}
	now := s.clock.Now().UTC()
	session = identity.Session{ID: uuid.New(), UserID: userID, TokenHash: hash(sessionToken), CSRFTokenHash: hash(csrfToken), RefreshTokenCiphertext: append([]byte(nil), refreshTokenCiphertext...), IdentityVerifiedAt: now, CreatedAt: now, LastSeenAt: now, IdleExpiresAt: now.Add(s.idleTimeout), AbsoluteExpiresAt: now.Add(s.absoluteTTL), AuthenticationMethod: method}
	if err := s.repository.CreateSession(ctx, session); err != nil {
		return identity.Session{}, "", "", fmt.Errorf("create session: %w", err)
	}
	return session, sessionToken, csrfToken, nil
}

// UpdateProviderCredential rotates the encrypted refresh token after successful identity sync.
func (s *SessionService) UpdateProviderCredential(ctx context.Context, sessionID uuid.UUID, ciphertext []byte) error {
	if len(ciphertext) == 0 {
		return fmt.Errorf("encrypted provider refresh token is required")
	}
	if err := s.repository.UpdateProviderCredential(ctx, sessionID, ciphertext, s.clock.Now().UTC()); err != nil {
		return fmt.Errorf("update provider credential: %w", err)
	}
	return nil
}

// IdentitySyncDue reports whether the provider identity must be revalidated.
func (s *SessionService) IdentitySyncDue(session identity.Session, interval time.Duration) bool {
	return !s.clock.Now().UTC().Before(session.IdentityVerifiedAt.Add(interval))
}

// Authenticate validates a token and renews only its idle expiry.
func (s *SessionService) Authenticate(ctx context.Context, token string) (identity.Session, error) {
	session, err := s.repository.FindSessionByTokenHash(ctx, hash(token))
	if err != nil {
		return identity.Session{}, fmt.Errorf("find session: %w", err)
	}
	now := s.clock.Now().UTC()
	if !session.ActiveAt(now) {
		return identity.Session{}, fmt.Errorf("%w: inactive", ErrSessionInvalid)
	}
	idleExpiresAt := now.Add(s.idleTimeout)
	if idleExpiresAt.After(session.AbsoluteExpiresAt) {
		idleExpiresAt = session.AbsoluteExpiresAt
	}
	if err := s.repository.TouchSession(ctx, session.ID, now, idleExpiresAt); err != nil {
		return identity.Session{}, fmt.Errorf("touch session: %w", err)
	}
	session.LastSeenAt, session.IdleExpiresAt = now, idleExpiresAt
	return session, nil
}

// ValidateCSRF verifies a request token against the session-bound token hash.
func (s *SessionService) ValidateCSRF(session identity.Session, token string) error {
	if token == "" || subtle.ConstantTimeCompare(session.CSRFTokenHash, hash(token)) != 1 {
		return ErrCSRFInvalid
	}
	return nil
}

// Revoke invalidates the session immediately.
func (s *SessionService) Revoke(ctx context.Context, sessionID uuid.UUID) error {
	if err := s.repository.RevokeSession(ctx, sessionID, s.clock.Now().UTC()); err != nil {
		return fmt.Errorf("revoke session: %w", err)
	}
	return nil
}

func hash(token string) []byte { sum := sha256.Sum256([]byte(token)); return sum[:] }
func newOpaqueToken() (string, error) {
	bytes := make([]byte, 32)
	if _, err := rand.Read(bytes); err != nil {
		return "", fmt.Errorf("generate opaque token: %w", err)
	}
	return base64.RawURLEncoding.EncodeToString(bytes), nil
}
