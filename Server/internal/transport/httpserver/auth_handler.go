package httpserver

import (
	"context"
	"net/http"
	"net/url"
	"time"

	"github.com/gin-gonic/gin"

	identityapp "github.com/vincent119/ReleaseHub/Server/internal/identity/application"
	identity "github.com/vincent119/ReleaseHub/Server/internal/identity/domain"
	contract "github.com/vincent119/ReleaseHub/Server/internal/transport/openapi"
)

type authFlow interface {
	Begin() (string, string, error)
	Complete(context.Context, string, string, string) (identity.User, string, string, error)
	Authenticate(context.Context, string) (identity.Session, identity.User, error)
	ValidateMutation(context.Context, string, string) (identity.Session, identity.User, error)
	Logout(context.Context, string, string) (string, error)
	BackchannelLogout(context.Context, string) error
}

type authHandler struct {
	flow           authFlow
	webRedirectURL string
	cookieSecure   bool
	loginStateTTL  time.Duration
	sessionTTL     time.Duration
}

func (h *authHandler) authenticateMutation(c *gin.Context, csrfToken string) (identity.Session, identity.User, bool) {
	if !h.mutationOriginAllowed(c) {
		return identity.Session{}, identity.User{}, false
	}
	if h.flow == nil {
		respondError(c, http.StatusUnauthorized, "SESSION_REQUIRED", "Authentication is required")
		return identity.Session{}, identity.User{}, false
	}
	token, ok := sessionToken(c)
	if !ok {
		respondError(c, http.StatusUnauthorized, "SESSION_REQUIRED", "Authentication is required")
		return identity.Session{}, identity.User{}, false
	}
	session, user, err := h.flow.ValidateMutation(c.Request.Context(), token, csrfToken)
	if err != nil {
		respondError(c, http.StatusForbidden, "MUTATION_REJECTED", "State-changing request was rejected")
		return identity.Session{}, identity.User{}, false
	}
	return session, user, true
}

func (h *authHandler) mutationOriginAllowed(c *gin.Context) bool {
	if sameOrigin(c.Request, h.webRedirectURL) {
		return true
	}
	respondError(c, http.StatusForbidden, "ORIGIN_REJECTED", "Request origin is not allowed")
	return false
}

func (h *authHandler) BackchannelLogout(c *gin.Context) {
	if h.flow == nil {
		respondError(c, http.StatusServiceUnavailable, "AUTH_UNAVAILABLE", "Authentication is unavailable")
		return
	}
	logoutToken := c.PostForm("logout_token")
	if logoutToken == "" {
		respondError(c, http.StatusBadRequest, "LOGOUT_TOKEN_REQUIRED", "Logout token is required")
		return
	}
	if err := h.flow.BackchannelLogout(c.Request.Context(), logoutToken); err != nil {
		respondError(c, http.StatusBadRequest, "LOGOUT_TOKEN_INVALID", "Logout token was rejected")
		return
	}
	c.Status(http.StatusNoContent)
}

func (h *authHandler) BeginAuthLogin(c *gin.Context) {
	c.Header("Cache-Control", "no-store")
	if h.flow == nil {
		respondError(c, http.StatusServiceUnavailable, "AUTH_UNAVAILABLE", "Authentication is unavailable")
		return
	}
	cookieValue, redirectURL, err := h.flow.Begin()
	if err != nil {
		respondError(c, http.StatusServiceUnavailable, "AUTH_LOGIN_FAILED", "Unable to start authentication")
		return
	}
	writeCookie(c, h.loginCookie(cookieValue))
	c.Redirect(http.StatusFound, redirectURL)
}

func (h *authHandler) CompleteAuthCallback(c *gin.Context, params contract.CompleteAuthCallbackParams) {
	c.Header("Cache-Control", "no-store")
	if h.flow == nil {
		respondError(c, http.StatusServiceUnavailable, "AUTH_UNAVAILABLE", "Authentication is unavailable")
		return
	}
	cookieValue, ok := loginStateCookie(c)
	if !ok {
		respondError(c, http.StatusBadRequest, "AUTH_STATE_MISSING", "Authentication state is missing")
		return
	}
	_, sessionToken, csrfToken, err := h.flow.Complete(c.Request.Context(), cookieValue, params.State, params.Code)
	writeCookie(c, h.expiredCookie(loginCookieName, "/api/v1/auth/callback", true))
	if err != nil {
		respondError(c, http.StatusUnauthorized, "AUTH_CALLBACK_FAILED", "Authentication callback was rejected")
		return
	}
	h.setSessionCookies(c, sessionToken, csrfToken)
	c.Redirect(http.StatusFound, h.webRedirectURL)
}

func loginStateCookie(c *gin.Context) (string, bool) {
	value, err := c.Cookie(loginCookieName)
	return value, err == nil
}

func (h *authHandler) setSessionCookies(c *gin.Context, sessionToken, csrfToken string) {
	writeCookie(c, h.sessionCookie(sessionCookieName, sessionToken, true))
	writeCookie(c, h.sessionCookie(csrfCookieName, csrfToken, false))
}

func (h *authHandler) GetAuthSession(c *gin.Context) {
	c.Header("Cache-Control", "no-store")
	_, user, ok := h.authenticate(c)
	if !ok {
		return
	}
	c.JSON(http.StatusOK, contract.AuthSessionResponse{Data: contract.AuthSession{UserId: user.ID, Username: user.Username}, Meta: responseMeta(c)})
}

func (h *authHandler) LogoutAuthSession(c *gin.Context, params contract.LogoutAuthSessionParams) {
	c.Header("Cache-Control", "no-store")
	if !sameOrigin(c.Request, h.webRedirectURL) {
		respondError(c, http.StatusForbidden, "ORIGIN_REJECTED", "Request origin is not allowed")
		return
	}
	token, ok := sessionToken(c)
	if !ok {
		respondError(c, http.StatusUnauthorized, "SESSION_REQUIRED", "Authentication is required")
		return
	}
	redirectURL, err := h.flow.Logout(c.Request.Context(), token, params.XCSRFToken)
	if err != nil {
		respondError(c, http.StatusForbidden, "SESSION_LOGOUT_REJECTED", "Logout request was rejected")
		return
	}
	h.clearSessionCookies(c)
	c.JSON(http.StatusOK, contract.LogoutResponse{Data: logoutResult(redirectURL), Meta: responseMeta(c)})
}

func sessionToken(c *gin.Context) (string, bool) {
	value, err := c.Cookie(sessionCookieName)
	return value, err == nil
}

func (h *authHandler) clearSessionCookies(c *gin.Context) {
	writeCookie(c, h.expiredCookie(sessionCookieName, "/", true))
	writeCookie(c, h.expiredCookie(csrfCookieName, "/", false))
}

func logoutResult(redirectURL string) contract.LogoutResult {
	result := contract.LogoutResult{LoggedOut: true}
	if redirectURL != "" {
		result.RedirectUrl = &redirectURL
	}
	return result
}

func (h *authHandler) authenticate(c *gin.Context) (identity.Session, identity.User, bool) {
	if h.flow == nil {
		respondError(c, http.StatusUnauthorized, "SESSION_REQUIRED", "Authentication is required")
		return identity.Session{}, identity.User{}, false
	}
	token, ok := sessionToken(c)
	if !ok {
		respondError(c, http.StatusUnauthorized, "SESSION_REQUIRED", "Authentication is required")
		return identity.Session{}, identity.User{}, false
	}
	session, user, err := h.flow.Authenticate(c.Request.Context(), token)
	if err != nil {
		h.clearSessionCookies(c)
		respondError(c, http.StatusUnauthorized, "SESSION_INVALID", "Authentication session is invalid")
		return identity.Session{}, identity.User{}, false
	}
	return session, user, true
}

func responseMeta(c *gin.Context) contract.ResponseMeta {
	return contract.ResponseMeta{RequestId: c.GetHeader(requestIDHeader), Timestamp: time.Now().UTC()}
}
func respondError(c *gin.Context, status int, code, message string) {
	c.JSON(status, contract.ErrorResponse{Code: code, Message: message, RequestId: c.GetHeader(requestIDHeader)})
}

func (h *authHandler) loginCookie(value string) http.Cookie {
	return http.Cookie{
		Name: loginCookieName, Value: value, Path: "/api/v1/auth/callback",
		MaxAge: int(h.loginStateTTL.Seconds()), HttpOnly: true, Secure: h.cookieSecure,
	}
}

func (h *authHandler) sessionCookie(name, value string, httpOnly bool) http.Cookie {
	return http.Cookie{
		Name: name, Value: value, Path: "/", MaxAge: int(h.sessionTTL.Seconds()),
		HttpOnly: httpOnly, Secure: h.cookieSecure,
	}
}

func (h *authHandler) expiredCookie(name, path string, httpOnly bool) http.Cookie {
	return http.Cookie{
		Name: name, Path: path, MaxAge: -1, HttpOnly: httpOnly, Secure: h.cookieSecure,
	}
}

func writeCookie(c *gin.Context, cookie http.Cookie) {
	cookie.SameSite = http.SameSiteLaxMode
	http.SetCookie(c.Writer, &cookie)
}
func sameOrigin(request *http.Request, target string) bool {
	targetURL, err := url.Parse(target)
	if err != nil || targetURL.Scheme == "" || targetURL.Host == "" {
		return false
	}
	source := request.Header.Get("Origin")
	if source == "" {
		source = request.Referer()
	}
	sourceURL, err := url.Parse(source)
	return err == nil && sourceURL.Scheme == targetURL.Scheme && sourceURL.Host == targetURL.Host
}

var _ authFlow = (*identityapp.AuthFlow)(nil)
