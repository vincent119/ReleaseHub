package application_test

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/stretchr/testify/require"

	"github.com/vincent119/ReleaseHub/Server/internal/identity/application"
	identity "github.com/vincent119/ReleaseHub/Server/internal/identity/domain"
)

type fixedClock struct{ now time.Time }

func (c fixedClock) Now() time.Time { return c.now }

type memorySessions struct {
	sessions              map[string]identity.Session
	findError, touchError error
}

func (m *memorySessions) CreateSession(_ context.Context, session identity.Session) error {
	m.sessions[string(session.TokenHash)] = session
	return nil
}
func (m *memorySessions) FindSessionByTokenHash(_ context.Context, hash []byte) (identity.Session, error) {
	if m.findError != nil {
		return identity.Session{}, m.findError
	}
	session, ok := m.sessions[string(hash)]
	if !ok {
		return identity.Session{}, application.ErrSessionInvalid
	}
	return session, nil
}
func (m *memorySessions) TouchSession(_ context.Context, id uuid.UUID, lastSeenAt, idleExpiresAt time.Time) error {
	if m.touchError != nil {
		return m.touchError
	}
	for key, session := range m.sessions {
		if session.ID == id {
			session.LastSeenAt, session.IdleExpiresAt = lastSeenAt, idleExpiresAt
			m.sessions[key] = session
			return nil
		}
	}
	return errors.New("not found")
}
func (m *memorySessions) RevokeSession(_ context.Context, id uuid.UUID, revokedAt time.Time) error {
	for key, session := range m.sessions {
		if session.ID == id {
			session.RevokedAt = &revokedAt
			m.sessions[key] = session
			return nil
		}
	}
	return errors.New("not found")
}
func (m *memorySessions) RevokeUserSessions(_ context.Context, userID uuid.UUID, revokedAt time.Time) error {
	for key, session := range m.sessions {
		if session.UserID == userID {
			session.RevokedAt = &revokedAt
			m.sessions[key] = session
		}
	}
	return nil
}
func (m *memorySessions) UpdateProviderCredential(_ context.Context, id uuid.UUID, ciphertext []byte, verifiedAt time.Time) error {
	for key, session := range m.sessions {
		if session.ID == id {
			session.RefreshTokenCiphertext, session.IdentityVerifiedAt = append([]byte(nil), ciphertext...), verifiedAt
			m.sessions[key] = session
			return nil
		}
	}
	return errors.New("not found")
}

func TestSessionServiceCreatesOpaqueTokensAndHonorsAbsoluteExpiry(t *testing.T) {
	t.Parallel()
	now := time.Date(2026, 9, 1, 0, 0, 0, 0, time.UTC)
	repository := &memorySessions{sessions: map[string]identity.Session{}}
	service, err := application.NewSessionService(repository, fixedClock{now: now}, 30*time.Minute, time.Hour)
	require.NoError(t, err)

	sessionToken, csrfToken, err := service.Create(context.Background(), uuid.New(), []byte("encrypted-refresh-token"))
	require.NoError(t, err)
	require.NotEmpty(t, sessionToken)
	require.NotEmpty(t, csrfToken)
	require.NotEqual(t, sessionToken, csrfToken)

	session, err := service.Authenticate(context.Background(), sessionToken)
	require.NoError(t, err)
	require.Equal(t, now.Add(30*time.Minute), session.IdleExpiresAt)
	require.NotContains(t, string(session.TokenHash), sessionToken)
	require.NoError(t, service.ValidateCSRF(session, csrfToken))
	require.Error(t, service.ValidateCSRF(session, "unexpected"))
	require.NoError(t, service.Revoke(context.Background(), session.ID))
	_, err = service.Authenticate(context.Background(), sessionToken)
	require.ErrorContains(t, err, "inactive")
	require.ErrorIs(t, err, application.ErrSessionInvalid)
}

func TestSessionServiceRejectsIdleAndAbsoluteExpiry(t *testing.T) {
	now := time.Date(2026, 9, 1, 0, 0, 0, 0, time.UTC)
	clock := &mutableClock{now: now}
	repository := &memorySessions{sessions: map[string]identity.Session{}}
	service, err := application.NewSessionService(repository, clock, 30*time.Minute, time.Hour)
	require.NoError(t, err)
	token, _, err := service.Create(context.Background(), uuid.New(), []byte("encrypted-refresh-token"))
	require.NoError(t, err)
	clock.now = now.Add(30 * time.Minute)
	_, err = service.Authenticate(context.Background(), token)
	require.ErrorContains(t, err, "inactive")
	require.ErrorIs(t, err, application.ErrSessionInvalid)
}

func TestSessionServicePreservesTerminalAndInfrastructureErrorClassification(t *testing.T) {
	now := time.Date(2026, 9, 15, 4, 0, 0, 0, time.UTC)
	repository := &memorySessions{sessions: map[string]identity.Session{}}
	service, err := application.NewSessionService(repository, fixedClock{now: now}, 30*time.Minute, time.Hour)
	require.NoError(t, err)
	_, token, _, err := service.CreateLocal(context.Background(), uuid.New())
	require.NoError(t, err)

	repository.touchError = application.ErrSessionInvalid
	_, err = service.Authenticate(context.Background(), token)
	require.ErrorIs(t, err, application.ErrSessionInvalid)

	repository.touchError = nil
	repository.findError = errors.New("database unavailable")
	_, err = service.Authenticate(context.Background(), token)
	require.ErrorContains(t, err, "database unavailable")
	require.NotErrorIs(t, err, application.ErrSessionInvalid)
}
