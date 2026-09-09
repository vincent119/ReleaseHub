package application

import (
	"context"
	"fmt"
	"time"

	"github.com/google/uuid"

	identity "github.com/vincent119/ReleaseHub/Server/internal/identity/domain"
)

// OIDCClaims is the normalized provider identity used by the application layer.
type OIDCClaims struct {
	Issuer   string
	Subject  string
	Username string
	Groups   []string
}

// OIDCAuthentication contains normalized claims and the server-only refresh credential.
type OIDCAuthentication struct {
	Claims       OIDCClaims
	RefreshToken string
}

// OIDCProvider defines the provider operations required by the BFF flow.
type OIDCProvider interface {
	AuthorizationURL(state, verifier, nonce string) string
	ExchangeAndVerify(context.Context, string, string, string) (OIDCAuthentication, error)
	RefreshAndVerify(context.Context, string) (OIDCAuthentication, error)
	LogoutURL() string
	VerifyLogoutToken(context.Context, string) (string, string, error)
}

// BackchannelLogout validates a provider event and revokes all sessions for the bound identity.
func (f *AuthFlow) BackchannelLogout(ctx context.Context, rawToken string) error {
	if f.provider == nil {
		return fmt.Errorf("OIDC authentication is unavailable")
	}
	issuer, subject, err := f.provider.VerifyLogoutToken(ctx, rawToken)
	if err != nil {
		return fmt.Errorf("verify back-channel logout: %w", err)
	}
	user, err := f.identity.FindUserByIdentity(ctx, issuer, subject)
	if err != nil {
		return err
	}
	return f.sessions.RevokeUserSessions(ctx, user.ID)
}

// AuthFlow coordinates OIDC state verification, identity resolution, and session creation.
type AuthFlow struct {
	provider             OIDCProvider
	states               *LoginStateCodec
	identity             *IdentityService
	sessions             *SessionService
	protector            *TokenProtector
	identitySyncInterval time.Duration
	groups               OIDCGroupSynchronizer
}

// OIDCGroupSynchronizer applies configured provider group mappings to a local user.
type OIDCGroupSynchronizer interface {
	SyncOIDCGroups(context.Context, uuid.UUID, string, []string) error
}

// AuthFlowOption configures an optional authentication integration.
type AuthFlowOption func(*AuthFlow) error

// WithOIDCGroupSynchronizer enables fail-closed releasehub_group synchronization.
func WithOIDCGroupSynchronizer(synchronizer OIDCGroupSynchronizer) AuthFlowOption {
	return func(flow *AuthFlow) error {
		if synchronizer == nil {
			return fmt.Errorf("OIDC group synchronizer is required")
		}
		flow.groups = synchronizer
		return nil
	}
}

// NewAuthFlow creates the BFF authentication use case.
func NewAuthFlow(provider OIDCProvider, states *LoginStateCodec, identityService *IdentityService, sessions *SessionService, protector *TokenProtector, identitySyncInterval time.Duration, options ...AuthFlowOption) (*AuthFlow, error) {
	if states == nil || identityService == nil || sessions == nil || protector == nil || identitySyncInterval <= 0 {
		return nil, fmt.Errorf("login state, identity, session, token protection, and identity sync are required")
	}
	flow := &AuthFlow{provider: provider, states: states, identity: identityService, sessions: sessions, protector: protector, identitySyncInterval: identitySyncInterval}
	for _, option := range options {
		if err := option(flow); err != nil {
			return nil, err
		}
	}
	return flow, nil
}

// Begin creates one signed login attempt and provider redirect URL.
func (f *AuthFlow) Begin() (cookieValue, redirectURL string, err error) {
	if f.provider == nil {
		return "", "", fmt.Errorf("OIDC authentication is unavailable")
	}
	cookieValue, state, verifier, nonce, err := f.states.New()
	if err != nil {
		return "", "", err
	}
	return cookieValue, f.provider.AuthorizationURL(state, verifier, nonce), nil
}

// Complete verifies callback state and creates an opaque local session.
func (f *AuthFlow) Complete(ctx context.Context, cookieValue, state, code string) (identity.User, string, string, error) {
	if f.provider == nil {
		return identity.User{}, "", "", fmt.Errorf("OIDC authentication is unavailable")
	}
	loginState, err := f.states.Verify(cookieValue, state)
	if err != nil {
		return identity.User{}, "", "", fmt.Errorf("verify login state: %w", err)
	}
	authentication, err := f.provider.ExchangeAndVerify(ctx, code, loginState.Verifier, loginState.Nonce)
	if err != nil {
		return identity.User{}, "", "", fmt.Errorf("verify OIDC callback: %w", err)
	}
	claims := authentication.Claims
	user, err := f.identity.ResolveOIDCIdentity(ctx, claims.Issuer, claims.Subject, claims.Username)
	if err != nil {
		return identity.User{}, "", "", err
	}
	if f.groups != nil {
		if err := f.groups.SyncOIDCGroups(ctx, user.ID, claims.Issuer, claims.Groups); err != nil {
			return identity.User{}, "", "", fmt.Errorf("synchronize OIDC group claims: %w", err)
		}
	}
	ciphertext, err := f.protector.Encrypt(authentication.RefreshToken)
	if err != nil {
		return identity.User{}, "", "", err
	}
	sessionToken, csrfToken, err := f.sessions.Create(ctx, user.ID, ciphertext)
	if err != nil {
		return identity.User{}, "", "", err
	}
	return user, sessionToken, csrfToken, nil
}

// Authenticate resolves the current active local account.
func (f *AuthFlow) Authenticate(ctx context.Context, token string) (identity.Session, identity.User, error) {
	session, err := f.sessions.Authenticate(ctx, token)
	if err != nil {
		return identity.Session{}, identity.User{}, err
	}
	user, err := f.identity.FindUser(ctx, session.UserID)
	if err != nil {
		return identity.Session{}, identity.User{}, err
	}
	if session.AuthenticationMethod == "local" {
		return session, user, nil
	}
	if f.provider == nil {
		_ = f.sessions.Revoke(ctx, session.ID)
		return identity.Session{}, identity.User{}, fmt.Errorf("OIDC authentication is unavailable")
	}
	if !f.sessions.IdentitySyncDue(session, f.identitySyncInterval) {
		return session, user, nil
	}
	refreshToken, err := f.protector.Decrypt(session.RefreshTokenCiphertext)
	if err != nil {
		_ = f.sessions.Revoke(ctx, session.ID)
		return identity.Session{}, identity.User{}, err
	}
	authentication, err := f.provider.RefreshAndVerify(ctx, refreshToken)
	if err != nil {
		_ = f.sessions.Revoke(ctx, session.ID)
		return identity.Session{}, identity.User{}, fmt.Errorf("refresh OIDC identity: %w", err)
	}
	claims := authentication.Claims
	refreshedUser, err := f.identity.ResolveOIDCIdentity(ctx, claims.Issuer, claims.Subject, claims.Username)
	if err != nil || refreshedUser.ID != user.ID {
		_ = f.sessions.Revoke(ctx, session.ID)
		return identity.Session{}, identity.User{}, fmt.Errorf("refreshed OIDC identity does not match session")
	}
	if f.groups != nil {
		if err := f.groups.SyncOIDCGroups(ctx, refreshedUser.ID, claims.Issuer, claims.Groups); err != nil {
			_ = f.sessions.Revoke(ctx, session.ID)
			return identity.Session{}, identity.User{}, fmt.Errorf("synchronize refreshed OIDC group claims: %w", err)
		}
	}
	ciphertext, err := f.protector.Encrypt(authentication.RefreshToken)
	if err != nil {
		_ = f.sessions.Revoke(ctx, session.ID)
		return identity.Session{}, identity.User{}, err
	}
	if err := f.sessions.UpdateProviderCredential(ctx, session.ID, ciphertext); err != nil {
		return identity.Session{}, identity.User{}, err
	}
	return session, refreshedUser, nil
}

// Logout validates CSRF, revokes the local session, and returns the provider logout URL when available.
func (f *AuthFlow) Logout(ctx context.Context, token, csrfToken string) (string, error) {
	if _, _, err := f.ValidateMutation(ctx, token, csrfToken); err != nil {
		return "", err
	}
	session, _, err := f.Authenticate(ctx, token)
	if err != nil {
		return "", err
	}
	if err := f.sessions.Revoke(ctx, session.ID); err != nil {
		return "", err
	}
	if session.AuthenticationMethod == "local" {
		return "", nil
	}
	if f.provider == nil {
		return "", fmt.Errorf("OIDC authentication is unavailable")
	}
	return f.provider.LogoutURL(), nil
}

// ValidateMutation authenticates a browser session and verifies its CSRF token for a state-changing request.
func (f *AuthFlow) ValidateMutation(ctx context.Context, token, csrfToken string) (identity.Session, identity.User, error) {
	session, user, err := f.Authenticate(ctx, token)
	if err != nil {
		return identity.Session{}, identity.User{}, err
	}
	if err := f.sessions.ValidateCSRF(session, csrfToken); err != nil {
		return identity.Session{}, identity.User{}, err
	}
	return session, user, nil
}
