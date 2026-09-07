package infrastructure

import (
	"context"
	"crypto/subtle"
	"encoding/json"
	"fmt"
	"net/url"
	"strings"
	"time"

	"github.com/coreos/go-oidc/v3/oidc"
	"golang.org/x/oauth2"

	"github.com/vincent119/ReleaseHub/Server/internal/config"
	"github.com/vincent119/ReleaseHub/Server/internal/identity/application"
)

// OIDCProvider exchanges authorization codes and validates ID tokens.
type OIDCProvider struct {
	oauthConfig       *oauth2.Config
	verifier          *oidc.IDTokenVerifier
	logoutURL         string
	accessTokenMaxTTL time.Duration
}

// NewOIDCProvider discovers the issuer and creates a verifier for the configured client.
func NewOIDCProvider(ctx context.Context, cfg config.OIDCConfig) (*OIDCProvider, error) {
	if strings.TrimSpace(cfg.Issuer) == "" || strings.TrimSpace(cfg.ClientID) == "" || strings.TrimSpace(cfg.RedirectURL) == "" || cfg.AccessTokenMaxTTL <= 0 {
		return nil, fmt.Errorf("OIDC issuer, client ID, and redirect URL are required")
	}
	provider, err := oidc.NewProvider(ctx, cfg.Issuer)
	if err != nil {
		return nil, fmt.Errorf("discover OIDC provider: %w", err)
	}
	var metadata struct {
		EndSessionEndpoint string `json:"end_session_endpoint"`
	}
	if err := provider.Claims(&metadata); err != nil {
		return nil, fmt.Errorf("decode OIDC provider metadata: %w", err)
	}
	return &OIDCProvider{
		oauthConfig:       &oauth2.Config{ClientID: cfg.ClientID, ClientSecret: cfg.ClientSecret, Endpoint: provider.Endpoint(), RedirectURL: cfg.RedirectURL, Scopes: []string{oidc.ScopeOpenID, "profile"}},
		verifier:          provider.Verifier(&oidc.Config{ClientID: cfg.ClientID}),
		logoutURL:         buildLogoutURL(metadata.EndSessionEndpoint, cfg.LogoutURL, cfg.ClientID, cfg.PostLogoutRedirectURL),
		accessTokenMaxTTL: cfg.AccessTokenMaxTTL,
	}, nil
}

// AuthorizationURL returns the provider redirect URL with PKCE challenge and offline access request.
func (p *OIDCProvider) AuthorizationURL(state, verifier, nonce string) string {
	return p.oauthConfig.AuthCodeURL(state, oauth2.AccessTypeOffline, oauth2.S256ChallengeOption(verifier), oidc.Nonce(nonce))
}

// ExchangeAndVerify exchanges a callback code and validates the provider-issued ID token.
func (p *OIDCProvider) ExchangeAndVerify(ctx context.Context, code, verifier, nonce string) (application.OIDCAuthentication, error) {
	token, err := p.oauthConfig.Exchange(ctx, code, oauth2.VerifierOption(verifier))
	if err != nil {
		return application.OIDCAuthentication{}, fmt.Errorf("exchange authorization code: %w", err)
	}
	return p.verifyAuthentication(ctx, token, nonce)
}

// RefreshAndVerify refreshes provider credentials and revalidates the OIDC identity.
func (p *OIDCProvider) RefreshAndVerify(ctx context.Context, refreshToken string) (application.OIDCAuthentication, error) {
	if refreshToken == "" {
		return application.OIDCAuthentication{}, fmt.Errorf("OIDC refresh token is required")
	}
	token, err := p.oauthConfig.TokenSource(ctx, &oauth2.Token{RefreshToken: refreshToken}).Token()
	if err != nil {
		return application.OIDCAuthentication{}, fmt.Errorf("refresh OIDC token: %w", err)
	}
	if token.RefreshToken == "" {
		token.RefreshToken = refreshToken
	}
	return p.verifyAuthentication(ctx, token, "")
}

func (p *OIDCProvider) verifyAuthentication(ctx context.Context, token *oauth2.Token, expectedNonce string) (application.OIDCAuthentication, error) {
	remaining := time.Until(token.Expiry)
	if token.Expiry.IsZero() || remaining <= 0 || remaining > p.accessTokenMaxTTL {
		return application.OIDCAuthentication{}, fmt.Errorf("OIDC access token lifetime exceeds the configured maximum")
	}
	if token.RefreshToken == "" {
		return application.OIDCAuthentication{}, fmt.Errorf("OIDC token response does not contain a refresh token")
	}
	rawIDToken, ok := token.Extra("id_token").(string)
	if !ok || rawIDToken == "" {
		return application.OIDCAuthentication{}, fmt.Errorf("OIDC token response does not contain an ID token")
	}
	idToken, err := p.verifier.Verify(ctx, rawIDToken)
	if err != nil {
		return application.OIDCAuthentication{}, fmt.Errorf("verify ID token: %w", err)
	}
	var claims struct {
		PreferredUsername string     `json:"preferred_username"`
		Username          string     `json:"username"`
		Email             string     `json:"email"`
		Nonce             string     `json:"nonce"`
		Groups            groupClaim `json:"releasehub_group"`
	}
	if err := idToken.Claims(&claims); err != nil {
		return application.OIDCAuthentication{}, fmt.Errorf("decode ID token claims: %w", err)
	}
	username := firstNonEmpty(claims.PreferredUsername, claims.Username, claims.Email)
	if username == "" {
		return application.OIDCAuthentication{}, fmt.Errorf("OIDC ID token does not contain a username")
	}
	if expectedNonce != "" && subtle.ConstantTimeCompare([]byte(claims.Nonce), []byte(expectedNonce)) != 1 {
		return application.OIDCAuthentication{}, fmt.Errorf("OIDC ID token nonce does not match login attempt")
	}
	return application.OIDCAuthentication{Claims: application.OIDCClaims{Issuer: idToken.Issuer, Subject: idToken.Subject, Username: username, Groups: []string(claims.Groups)}, RefreshToken: token.RefreshToken}, nil
}

// LogoutURL returns the provider logout endpoint or a configured Cognito-style fallback.
func (p *OIDCProvider) LogoutURL() string { return p.logoutURL }

// VerifyLogoutToken validates an OIDC back-channel logout event for a subject.
func (p *OIDCProvider) VerifyLogoutToken(ctx context.Context, rawToken string) (string, string, error) {
	token, err := p.verifier.Verify(ctx, rawToken)
	if err != nil {
		return "", "", fmt.Errorf("verify logout token: %w", err)
	}
	var claims struct {
		Events map[string]json.RawMessage `json:"events"`
		Nonce  string                     `json:"nonce"`
	}
	if err := token.Claims(&claims); err != nil {
		return "", "", fmt.Errorf("decode logout token claims: %w", err)
	}
	if _, ok := claims.Events["http://schemas.openid.net/event/backchannel-logout"]; !ok {
		return "", "", fmt.Errorf("logout token does not contain a back-channel logout event")
	}
	if claims.Nonce != "" {
		return "", "", fmt.Errorf("logout token must not contain nonce")
	}
	if token.Subject == "" {
		return "", "", fmt.Errorf("logout token subject is required")
	}
	return token.Issuer, token.Subject, nil
}

func buildLogoutURL(discovered, fallback, clientID, redirectURL string) string {
	endpoint, fallbackUsed := strings.TrimSpace(discovered), false
	if endpoint == "" {
		endpoint, fallbackUsed = strings.TrimSpace(fallback), true
	}
	if endpoint == "" {
		return ""
	}
	parsed, err := url.Parse(endpoint)
	if err != nil {
		return ""
	}
	query := parsed.Query()
	query.Set("client_id", clientID)
	if redirectURL != "" {
		if fallbackUsed {
			query.Set("logout_uri", redirectURL)
		} else {
			query.Set("post_logout_redirect_uri", redirectURL)
		}
	}
	parsed.RawQuery = query.Encode()
	return parsed.String()
}

func firstNonEmpty(values ...string) string {
	for _, value := range values {
		if value = strings.TrimSpace(value); value != "" {
			return value
		}
	}
	return ""
}

type groupClaim []string

func (c *groupClaim) UnmarshalJSON(data []byte) error {
	var groups []string
	if err := json.Unmarshal(data, &groups); err == nil {
		*c = groups
		return nil
	}
	var group string
	if err := json.Unmarshal(data, &group); err != nil {
		return fmt.Errorf("decode releasehub_group claim: %w", err)
	}
	if strings.TrimSpace(group) == "" {
		*c = nil
		return nil
	}
	*c = []string{group}
	return nil
}
