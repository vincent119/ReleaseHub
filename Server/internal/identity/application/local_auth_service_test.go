package application_test

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/stretchr/testify/require"
	"golang.org/x/crypto/bcrypt"

	"github.com/vincent119/ReleaseHub/Server/internal/identity/application"
	identity "github.com/vincent119/ReleaseHub/Server/internal/identity/domain"
)

func TestLocalAuthBootstrapsLogsInAndRequiresPasswordChange(t *testing.T) {
	userID := uuid.New()
	repository := &localCredentialMemory{credential: identity.LocalCredential{UserID: userID, Username: "admin", MustChangePassword: true}}
	sessions := &localSessionMemory{}
	sessionService, err := application.NewSessionService(sessions, fixedLocalClock{}, time.Minute, time.Hour)
	require.NoError(t, err)
	service, err := application.NewLocalAuthService(repository, sessionService)
	require.NoError(t, err)

	require.NoError(t, service.Bootstrap(context.Background(), "admin"))
	require.NoError(t, bcrypt.CompareHashAndPassword(repository.credential.PasswordHash, []byte("admin")))
	user, mustChange, session, sessionToken, csrfToken, err := service.Login(context.Background(), "admin", "admin")
	require.NoError(t, err)
	require.Equal(t, userID, user.ID)
	require.True(t, mustChange)
	require.NotEmpty(t, sessionToken)
	require.NotEmpty(t, csrfToken)
	require.Equal(t, fixedLocalClock{}.Now().Add(time.Minute), session.IdleExpiresAt)
	require.Equal(t, fixedLocalClock{}.Now().Add(time.Hour), session.AbsoluteExpiresAt)
	require.Equal(t, "local", sessions.created.AuthenticationMethod)

	require.NoError(t, service.ChangePassword(context.Background(), userID, "admin", "new-password"))
	require.False(t, repository.credential.MustChangePassword)
	require.NoError(t, bcrypt.CompareHashAndPassword(repository.credential.PasswordHash, []byte("new-password")))
	require.Equal(t, 1, sessions.revocations)
}

func TestLocalAuthRejectsInvalidCredentialsAndShortNewPassword(t *testing.T) {
	hash, err := bcrypt.GenerateFromPassword([]byte("admin"), bcrypt.MinCost)
	require.NoError(t, err)
	repository := &localCredentialMemory{credential: identity.LocalCredential{UserID: uuid.New(), Username: "admin", PasswordHash: hash, MustChangePassword: true}}
	sessionService, err := application.NewSessionService(&localSessionMemory{}, fixedLocalClock{}, time.Minute, time.Hour)
	require.NoError(t, err)
	service, err := application.NewLocalAuthService(repository, sessionService)
	require.NoError(t, err)

	_, _, _, _, _, err = service.Login(context.Background(), "admin", "wrong")
	require.Error(t, err)
	_, _, _, _, _, err = service.Login(context.Background(), "admin", string(make([]byte, 73)))
	require.Error(t, err)
	require.Error(t, service.ChangePassword(context.Background(), repository.credential.UserID, "admin", "short"))
	require.Error(t, service.ChangePassword(context.Background(), repository.credential.UserID, "admin", string(make([]byte, 73))))
}

func TestLocalAuthCreatesUserWithRequiredPasswordChange(t *testing.T) {
	repository := &localCredentialMemory{}
	sessionService, err := application.NewSessionService(&localSessionMemory{}, fixedLocalClock{}, time.Minute, time.Hour)
	require.NoError(t, err)
	service, err := application.NewLocalAuthService(repository, sessionService)
	require.NoError(t, err)

	user, err := service.CreateUser(context.Background(), uuid.New(), "request-1", "operator", "initial-password")
	require.NoError(t, err)
	require.Equal(t, "operator", user.Username)
	require.True(t, repository.credential.MustChangePassword)
	require.NoError(t, bcrypt.CompareHashAndPassword(repository.credential.PasswordHash, []byte("initial-password")))
}

func TestLocalAuthRejectsInvalidUserCreationInput(t *testing.T) {
	repository := &localCredentialMemory{}
	sessionService, err := application.NewSessionService(&localSessionMemory{}, fixedLocalClock{}, time.Minute, time.Hour)
	require.NoError(t, err)
	service, err := application.NewLocalAuthService(repository, sessionService)
	require.NoError(t, err)

	_, err = service.CreateUser(context.Background(), uuid.New(), "request-1", "invalid username", "initial-password")
	require.True(t, errors.Is(err, application.ErrInvalidLocalUser))
	_, err = service.CreateUser(context.Background(), uuid.New(), "request-2", "operator", "short")
	require.True(t, errors.Is(err, application.ErrInvalidLocalUser))
}

type localCredentialMemory struct{ credential identity.LocalCredential }

func (m *localCredentialMemory) BootstrapManager(_ context.Context, hash []byte) error {
	m.credential.PasswordHash = append([]byte(nil), hash...)
	return nil
}
func (m *localCredentialMemory) FindLocalCredential(_ context.Context, username string) (identity.LocalCredential, error) {
	return m.credential, nil
}
func (m *localCredentialMemory) FindLocalCredentialByUserID(_ context.Context, _ uuid.UUID) (identity.LocalCredential, error) {
	return m.credential, nil
}
func (m *localCredentialMemory) UpdatePassword(_ context.Context, _ uuid.UUID, hash []byte) error {
	m.credential.PasswordHash = append([]byte(nil), hash...)
	m.credential.MustChangePassword = false
	return nil
}
func (m *localCredentialMemory) CreateLocalUser(_ context.Context, _ uuid.UUID, _ string, username string, hash []byte) (identity.User, error) {
	m.credential = identity.LocalCredential{UserID: uuid.New(), Username: username, PasswordHash: append([]byte(nil), hash...), MustChangePassword: true}
	return identity.User{ID: m.credential.UserID, Username: username}, nil
}

type localSessionMemory struct {
	created     identity.Session
	revocations int
}

func (m *localSessionMemory) CreateSession(_ context.Context, session identity.Session) error {
	m.created = session
	return nil
}
func (m *localSessionMemory) FindSessionByTokenHash(context.Context, []byte) (identity.Session, error) {
	return m.created, nil
}
func (m *localSessionMemory) TouchSession(context.Context, uuid.UUID, time.Time, time.Time) error {
	return nil
}
func (m *localSessionMemory) RevokeSession(context.Context, uuid.UUID, time.Time) error { return nil }
func (m *localSessionMemory) RevokeUserSessions(context.Context, uuid.UUID, time.Time) error {
	m.revocations++
	return nil
}
func (m *localSessionMemory) UpdateProviderCredential(context.Context, uuid.UUID, []byte, time.Time) error {
	return nil
}

type fixedLocalClock struct{}

func (fixedLocalClock) Now() time.Time { return time.Date(2026, 9, 8, 0, 0, 0, 0, time.UTC) }
