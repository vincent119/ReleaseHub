package infrastructure

import (
	"context"
	"crypto/rand"
	"crypto/rsa"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"net/url"
	"testing"
	"time"

	"github.com/coreos/go-oidc/v3/oidc"
	"github.com/go-jose/go-jose/v4"
	"github.com/go-jose/go-jose/v4/jwt"

	"github.com/vincent119/ReleaseHub/Server/internal/config"
)

func TestOIDCProviderUsesPKCEAndVerifiesLoginAndBackchannelTokens(t *testing.T) {
	key, err := rsa.GenerateKey(rand.Reader, 2048)
	if err != nil {
		t.Fatal(err)
	}
	issuer := "https://issuer.example"
	mux := http.NewServeMux()
	mux.HandleFunc("/.well-known/openid-configuration", func(w http.ResponseWriter, _ *http.Request) {
		writeJSON(w, map[string]any{"issuer": issuer, "authorization_endpoint": issuer + "/authorize", "token_endpoint": issuer + "/token", "jwks_uri": issuer + "/keys", "end_session_endpoint": issuer + "/logout", "id_token_signing_alg_values_supported": []string{"RS256"}})
	})
	mux.HandleFunc("/keys", func(w http.ResponseWriter, _ *http.Request) {
		writeJSON(w, jose.JSONWebKeySet{Keys: []jose.JSONWebKey{{Key: &key.PublicKey, KeyID: "test", Algorithm: "RS256", Use: "sig"}}})
	})
	mux.HandleFunc("/token", func(w http.ResponseWriter, r *http.Request) {
		if err := r.ParseForm(); err != nil || (r.Form.Get("grant_type") == "authorization_code" && r.Form.Get("code_verifier") == "") {
			http.Error(w, "missing verifier", http.StatusBadRequest)
			return
		}
		raw := signToken(t, key, map[string]any{"iss": issuer, "sub": "subject-1", "aud": "releasehub", "exp": time.Now().Add(4 * time.Minute).Unix(), "iat": time.Now().Unix(), "preferred_username": "vincent", "nonce": "nonce", "releasehub_group": []string{"engineering-viewers"}})
		writeJSON(w, map[string]any{"access_token": "access", "refresh_token": "refresh", "token_type": "Bearer", "expires_in": 240, "id_token": raw})
	})

	client := &http.Client{Transport: roundTripFunc(func(request *http.Request) (*http.Response, error) {
		recorder := httptest.NewRecorder()
		mux.ServeHTTP(recorder, request)
		return recorder.Result(), nil
	})}
	providerContext := oidc.ClientContext(context.Background(), client)
	provider, err := NewOIDCProvider(providerContext, config.OIDCConfig{Issuer: issuer, ClientID: "releasehub", ClientSecret: "secret", RedirectURL: "https://releasehub.example/api/v1/auth/callback", PostLogoutRedirectURL: "https://releasehub.example/", AccessTokenMaxTTL: 5 * time.Minute})
	if err != nil {
		t.Fatalf("create provider: %v", err)
	}
	authorizationURL := provider.AuthorizationURL("state", "verifier", "nonce")
	parsed, _ := url.Parse(authorizationURL)
	if parsed.Query().Get("code_challenge_method") != "S256" || parsed.Query().Get("code_challenge") == "" {
		t.Fatalf("PKCE challenge missing: %s", authorizationURL)
	}
	if parsed.Query().Get("nonce") != "nonce" {
		t.Fatalf("OIDC nonce missing: %s", authorizationURL)
	}
	authentication, err := provider.ExchangeAndVerify(providerContext, "code", "verifier", "nonce")
	claims := authentication.Claims
	if err != nil || claims.Issuer != issuer || claims.Subject != "subject-1" || claims.Username != "vincent" || len(claims.Groups) != 1 || claims.Groups[0] != "engineering-viewers" || authentication.RefreshToken != "refresh" {
		t.Fatalf("login token verification failed: %#v %v", authentication, err)
	}
	if _, err := provider.RefreshAndVerify(providerContext, authentication.RefreshToken); err != nil {
		t.Fatalf("refresh identity: %v", err)
	}

	logoutToken := signToken(t, key, map[string]any{"iss": issuer, "sub": "subject-1", "aud": "releasehub", "exp": time.Now().Add(time.Minute).Unix(), "iat": time.Now().Unix(), "events": map[string]any{"http://schemas.openid.net/event/backchannel-logout": map[string]any{}}})
	logoutIssuer, subject, err := provider.VerifyLogoutToken(providerContext, logoutToken)
	if err != nil || logoutIssuer != issuer || subject != "subject-1" {
		t.Fatalf("logout token verification failed: %s %s %v", logoutIssuer, subject, err)
	}
}

func signToken(t *testing.T, key *rsa.PrivateKey, claims map[string]any) string {
	t.Helper()
	signer, err := jose.NewSigner(jose.SigningKey{Algorithm: jose.RS256, Key: jose.JSONWebKey{Key: key, KeyID: "test", Algorithm: "RS256", Use: "sig"}}, (&jose.SignerOptions{}).WithType("JWT"))
	if err != nil {
		t.Fatal(err)
	}
	raw, err := jwt.Signed(signer).Claims(claims).Serialize()
	if err != nil {
		t.Fatal(err)
	}
	return raw
}

func writeJSON(w http.ResponseWriter, value any) {
	w.Header().Set("Content-Type", "application/json")
	_ = json.NewEncoder(w).Encode(value)
}

type roundTripFunc func(*http.Request) (*http.Response, error)

func (f roundTripFunc) RoundTrip(request *http.Request) (*http.Response, error) { return f(request) }

func TestGroupClaimAcceptsSingleAndMultipleValues(t *testing.T) {
	for name, raw := range map[string]string{
		"single":   `"viewer"`,
		"multiple": `["viewer","developers"]`,
	} {
		t.Run(name, func(t *testing.T) {
			var claim groupClaim
			if err := json.Unmarshal([]byte(raw), &claim); err != nil {
				t.Fatalf("decode group claim: %v", err)
			}
			if len(claim) == 0 || claim[0] != "viewer" {
				t.Fatalf("unexpected group claim: %#v", claim)
			}
		})
	}
}

func TestBuildLogoutURLUsesKeycloakDiscoveryEndpoint(t *testing.T) {
	value := buildLogoutURL("https://keycloak.example/logout", "", "releasehub", "https://releasehub.example/")
	parsed, err := url.Parse(value)
	if err != nil {
		t.Fatal(err)
	}
	if parsed.Query().Get("post_logout_redirect_uri") != "https://releasehub.example/" {
		t.Fatalf("missing Keycloak redirect: %s", value)
	}
	if parsed.Query().Get("client_id") != "releasehub" {
		t.Fatalf("missing client ID: %s", value)
	}
}

func TestBuildLogoutURLUsesCognitoFallback(t *testing.T) {
	value := buildLogoutURL("", "https://tenant.auth.ap-northeast-1.amazoncognito.com/logout", "releasehub", "https://releasehub.example/")
	parsed, err := url.Parse(value)
	if err != nil {
		t.Fatal(err)
	}
	if parsed.Query().Get("logout_uri") != "https://releasehub.example/" {
		t.Fatalf("missing Cognito logout URI: %s", value)
	}
	if parsed.Query().Get("post_logout_redirect_uri") != "" {
		t.Fatalf("unexpected Keycloak parameter: %s", value)
	}
}
