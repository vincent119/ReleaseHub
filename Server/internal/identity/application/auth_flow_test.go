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

func TestAuthFlowCompletesLoginAndBackchannelLogoutRevokesSession(t *testing.T) {
	now := time.Date(2026, 9, 1, 0, 0, 0, 0, time.UTC)
	clock := &mutableClock{now: now}
	identities := &memoryIdentities{users: map[uuid.UUID]identity.User{}, bindings: map[string]uuid.UUID{}}
	identityService, err := application.NewIdentityService(identities)
	require.NoError(t, err)
	sessions := &memorySessions{sessions: map[string]identity.Session{}}
	sessionService, err := application.NewSessionService(sessions, clock, 30*time.Minute, time.Hour)
	require.NoError(t, err)
	states, err := application.NewLoginStateCodec([]byte("12345678901234567890123456789012"), clock, 5*time.Minute)
	require.NoError(t, err)
	protector, err := application.NewTokenProtector([]byte("12345678901234567890123456789012"))
	require.NoError(t, err)
	provider := &fakeOIDCProvider{authentication: application.OIDCAuthentication{Claims: application.OIDCClaims{Issuer: "https://issuer.example", Subject: "subject-1", Username: "vincent", Groups: []string{"viewer"}}, RefreshToken: "refresh-token"}}
	groupSync := &recordingGroupSynchronizer{}
	flow, err := application.NewAuthFlow(provider, states, identityService, sessionService, protector, 5*time.Minute, application.WithOIDCGroupSynchronizer(groupSync))
	require.NoError(t, err)

	loginCookie, redirectURL, err := flow.Begin()
	require.NoError(t, err)
	require.Equal(t, "https://issuer.example/authorize", redirectURL)
	user, sessionToken, csrfToken, err := flow.Complete(context.Background(), loginCookie, provider.state, "code")
	require.NoError(t, err)
	require.Equal(t, "vincent", user.Username)
	_, authenticatedUser, err := flow.Authenticate(context.Background(), sessionToken)
	require.NoError(t, err)
	require.Equal(t, user.ID, authenticatedUser.ID)
	require.Equal(t, 1, groupSync.calls)
	clock.now = now.Add(5 * time.Minute)
	_, _, err = flow.Authenticate(context.Background(), sessionToken)
	require.NoError(t, err)
	require.True(t, provider.refreshed)
	require.Equal(t, 2, groupSync.calls)

	provider.logoutIssuer, provider.logoutSubject = provider.authentication.Claims.Issuer, provider.authentication.Claims.Subject
	require.NoError(t, flow.BackchannelLogout(context.Background(), "logout-token"))
	_, _, err = flow.Authenticate(context.Background(), sessionToken)
	require.Error(t, err)
	_, err = flow.Logout(context.Background(), sessionToken, csrfToken)
	require.Error(t, err)
}

func TestAuthFlowFailsClosedWhenOIDCGroupSynchronizationFails(t *testing.T) {
	now := time.Date(2026, 9, 1, 0, 0, 0, 0, time.UTC)
	clock := &mutableClock{now: now}
	identities := &memoryIdentities{users: map[uuid.UUID]identity.User{}, bindings: map[string]uuid.UUID{}}
	identityService, _ := application.NewIdentityService(identities)
	sessions := &memorySessions{sessions: map[string]identity.Session{}}
	sessionService, _ := application.NewSessionService(sessions, clock, 30*time.Minute, time.Hour)
	states, _ := application.NewLoginStateCodec([]byte("12345678901234567890123456789012"), clock, 5*time.Minute)
	protector, _ := application.NewTokenProtector([]byte("12345678901234567890123456789012"))
	provider := &fakeOIDCProvider{authentication: application.OIDCAuthentication{Claims: application.OIDCClaims{Issuer: "https://issuer.example", Subject: "subject-1", Username: "vincent", Groups: []string{"unknown"}}, RefreshToken: "refresh-token"}}
	groupSync := &recordingGroupSynchronizer{err: errors.New("group synchronization unavailable")}
	flow, err := application.NewAuthFlow(provider, states, identityService, sessionService, protector, 5*time.Minute, application.WithOIDCGroupSynchronizer(groupSync))
	require.NoError(t, err)
	loginCookie, _, err := flow.Begin()
	require.NoError(t, err)
	_, _, _, err = flow.Complete(context.Background(), loginCookie, provider.state, "code")
	require.ErrorContains(t, err, "synchronize OIDC group claims")
	require.Empty(t, sessions.sessions)
}

func TestAuthFlowRevokesSessionWhenIdentityRefreshFails(t *testing.T) {
	now := time.Date(2026, 9, 1, 0, 0, 0, 0, time.UTC)
	clock := &mutableClock{now: now}
	identities := &memoryIdentities{users: map[uuid.UUID]identity.User{}, bindings: map[string]uuid.UUID{}}
	identityService, _ := application.NewIdentityService(identities)
	sessions := &memorySessions{sessions: map[string]identity.Session{}}
	sessionService, _ := application.NewSessionService(sessions, clock, 30*time.Minute, time.Hour)
	states, _ := application.NewLoginStateCodec([]byte("12345678901234567890123456789012"), clock, 5*time.Minute)
	protector, _ := application.NewTokenProtector([]byte("12345678901234567890123456789012"))
	provider := &fakeOIDCProvider{authentication: application.OIDCAuthentication{Claims: application.OIDCClaims{Issuer: "https://issuer.example", Subject: "subject-1", Username: "vincent"}, RefreshToken: "refresh-token"}}
	flow, err := application.NewAuthFlow(provider, states, identityService, sessionService, protector, 5*time.Minute)
	require.NoError(t, err)
	loginCookie, _, _ := flow.Begin()
	_, sessionToken, _, err := flow.Complete(context.Background(), loginCookie, provider.state, "code")
	require.NoError(t, err)
	clock.now = now.Add(5 * time.Minute)
	provider.refreshErr = errors.New("provider rejected refresh")
	_, _, err = flow.Authenticate(context.Background(), sessionToken)
	require.ErrorContains(t, err, "refresh OIDC identity")
	provider.refreshErr = nil
	_, _, err = flow.Authenticate(context.Background(), sessionToken)
	require.ErrorContains(t, err, "inactive")
}

func TestAuthFlowSupportsLocalSessionWithoutOIDCProvider(t *testing.T) {
	now := time.Date(2026, 9, 8, 0, 0, 0, 0, time.UTC)
	clock := &mutableClock{now: now}
	user := identity.User{ID: uuid.New(), Username: "admin"}
	identities := &memoryIdentities{users: map[uuid.UUID]identity.User{user.ID: user}, bindings: map[string]uuid.UUID{}}
	identityService, _ := application.NewIdentityService(identities)
	sessions := &memorySessions{sessions: map[string]identity.Session{}}
	sessionService, _ := application.NewSessionService(sessions, clock, 30*time.Minute, time.Hour)
	states, _ := application.NewLoginStateCodec([]byte("12345678901234567890123456789012"), clock, 5*time.Minute)
	protector, _ := application.NewTokenProtector([]byte("12345678901234567890123456789012"))
	flow, err := application.NewAuthFlow(nil, states, identityService, sessionService, protector, 5*time.Minute)
	require.NoError(t, err)
	sessionToken, csrfToken, err := sessionService.CreateLocal(context.Background(), user.ID)
	require.NoError(t, err)

	session, authenticatedUser, err := flow.Authenticate(context.Background(), sessionToken)
	require.NoError(t, err)
	require.Equal(t, "local", session.AuthenticationMethod)
	require.Equal(t, user, authenticatedUser)
	_, _, err = flow.Begin()
	require.ErrorContains(t, err, "unavailable")
	redirectURL, err := flow.Logout(context.Background(), sessionToken, csrfToken)
	require.NoError(t, err)
	require.Empty(t, redirectURL)
}

type fakeOIDCProvider struct {
	authentication              application.OIDCAuthentication
	state, verifier             string
	logoutIssuer, logoutSubject string
	refreshed                   bool
	refreshErr                  error
}

type recordingGroupSynchronizer struct {
	calls int
	err   error
}

func (s *recordingGroupSynchronizer) SyncOIDCGroups(_ context.Context, _ uuid.UUID, _ string, groups []string) error {
	s.calls++
	if len(groups) == 0 {
		return errors.New("expected normalized OIDC group claim")
	}
	return s.err
}

func (p *fakeOIDCProvider) AuthorizationURL(state, verifier, _ string) string {
	p.state, p.verifier = state, verifier
	return "https://issuer.example/authorize"
}
func (p *fakeOIDCProvider) ExchangeAndVerify(context.Context, string, string, string) (application.OIDCAuthentication, error) {
	return p.authentication, nil
}
func (p *fakeOIDCProvider) RefreshAndVerify(context.Context, string) (application.OIDCAuthentication, error) {
	p.refreshed = true
	return p.authentication, p.refreshErr
}
func (*fakeOIDCProvider) LogoutURL() string { return "https://issuer.example/logout" }
func (p *fakeOIDCProvider) VerifyLogoutToken(context.Context, string) (string, string, error) {
	return p.logoutIssuer, p.logoutSubject, nil
}

type memoryIdentities struct {
	users    map[uuid.UUID]identity.User
	bindings map[string]uuid.UUID
}

func (m *memoryIdentities) FindOrCreateIdentity(_ context.Context, issuer, subject, username string) (identity.User, error) {
	key := issuer + "\x00" + subject
	if id, ok := m.bindings[key]; ok {
		return m.users[id], nil
	}
	for _, user := range m.users {
		if user.Username == username {
			return identity.User{}, errors.New("username conflict")
		}
	}
	user := identity.User{ID: uuid.New(), Username: username}
	m.users[user.ID], m.bindings[key] = user, user.ID
	return user, nil
}
func (m *memoryIdentities) FindUser(_ context.Context, userID uuid.UUID) (identity.User, error) {
	user, ok := m.users[userID]
	if !ok {
		return identity.User{}, errors.New("not found")
	}
	return user, nil
}
func (m *memoryIdentities) FindUserByIdentity(_ context.Context, issuer, subject string) (identity.User, error) {
	id, ok := m.bindings[issuer+"\x00"+subject]
	if !ok {
		return identity.User{}, errors.New("not found")
	}
	return m.users[id], nil
}
