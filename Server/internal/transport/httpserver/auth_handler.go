package httpserver

import (
	"context"
	"errors"
	"net/http"
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
	local          localAuth
	webRedirectURL string
	cookieSecure   bool
	loginStateTTL  time.Duration
	sessionTTL     time.Duration
}

func (h *authHandler) authenticateMutation(c *gin.Context, csrfToken string) (identity.Session, identity.User, bool) {
	session, user, ok := h.authenticateMutationForPasswordChange(c, csrfToken)
	if ok && h.local != nil && session.AuthenticationMethod == "local" {
		mustChange, err := h.local.MustChangePassword(c.Request.Context(), user.ID)
		if err != nil {
			if errors.Is(err, context.Canceled) {
				respondRequestCanceled(c)
				return identity.Session{}, identity.User{}, false
			}
			respondError(c, http.StatusInternalServerError, "SESSION_STATE_UNAVAILABLE", "Authentication state is unavailable")
			return identity.Session{}, identity.User{}, false
		}
		if mustChange {
			respondError(c, http.StatusForbidden, "PASSWORD_CHANGE_REQUIRED", "The initial password must be changed")
			return identity.Session{}, identity.User{}, false
		}
	}
	return session, user, ok
}

func (h *authHandler) authenticateMutationForPasswordChange(c *gin.Context, csrfToken string) (identity.Session, identity.User, bool) {
	if !h.mutationOriginAllowed(c) {
		return identity.Session{}, identity.User{}, false
	}
	token, ok := h.requiredSessionToken(c)
	if !ok {
		return identity.Session{}, identity.User{}, false
	}
	session, user, err := h.flow.ValidateMutation(c.Request.Context(), token, csrfToken)
	if err != nil {
		h.respondMutationAuthenticationError(c, err)
		return identity.Session{}, identity.User{}, false
	}
	return session, user, true
}

func (h *authHandler) requiredSessionToken(c *gin.Context) (string, bool) {
	if h.flow != nil {
		if token, ok := sessionToken(c); ok {
			return token, true
		}
	}
	respondError(c, http.StatusUnauthorized, "SESSION_REQUIRED", "Authentication is required")
	return "", false
}

func (h *authHandler) respondMutationAuthenticationError(c *gin.Context, err error) {
	if h.respondCommonAuthenticationError(c, err) {
		return
	}
	if errors.Is(err, identityapp.ErrCSRFInvalid) {
		respondError(c, http.StatusForbidden, "MUTATION_REJECTED", "State-changing request was rejected")
		return
	}
	recordRequestError(c, "validate authenticated mutation", err)
	respondError(c, http.StatusServiceUnavailable, "SESSION_STATE_UNAVAILABLE", "Authentication state is unavailable")
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

func (h *authHandler) GetAuthSession(c *gin.Context) {
	c.Header("Cache-Control", "no-store")
	session, user, ok := h.authenticateAllowPasswordChange(c)
	if !ok {
		return
	}
	state, ok := h.passwordChangeState(c, session, user)
	if !ok {
		return
	}
	c.JSON(http.StatusOK, authSessionResponse(c, session, user, state))
}

type passwordChangeState struct{ mustChange, available bool }

func (h *authHandler) passwordChangeState(c *gin.Context, session identity.Session, user identity.User) (passwordChangeState, bool) {
	state := passwordChangeState{available: h.local != nil && session.AuthenticationMethod == "local"}
	if !state.available {
		return state, true
	}
	mustChange, err := h.local.MustChangePassword(c.Request.Context(), user.ID)
	if err != nil {
		respondPasswordStateError(c, err)
		return passwordChangeState{}, false
	}
	state.mustChange = mustChange
	return state, true
}

func authSessionResponse(c *gin.Context, session identity.Session, user identity.User, state passwordChangeState) contract.AuthSessionResponse {
	return contract.AuthSessionResponse{Data: contract.AuthSession{
		UserId: user.ID, Username: user.Username, MustChangePassword: state.mustChange,
		PasswordChangeAvailable: state.available, IdleExpiresAt: session.IdleExpiresAt.UTC(),
		AbsoluteExpiresAt: session.AbsoluteExpiresAt.UTC(),
	}, Meta: responseMeta(c)}
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

func logoutResult(redirectURL string) contract.LogoutResult {
	result := contract.LogoutResult{LoggedOut: true}
	if redirectURL != "" {
		result.RedirectUrl = &redirectURL
	}
	return result
}

func (h *authHandler) authenticate(c *gin.Context) (identity.Session, identity.User, bool) {
	session, user, ok := h.authenticateAllowPasswordChange(c)
	if ok && h.local != nil && session.AuthenticationMethod == "local" {
		mustChange, err := h.local.MustChangePassword(c.Request.Context(), user.ID)
		if err != nil {
			if errors.Is(err, context.Canceled) {
				respondRequestCanceled(c)
				return identity.Session{}, identity.User{}, false
			}
			respondError(c, http.StatusInternalServerError, "SESSION_STATE_UNAVAILABLE", "Authentication state is unavailable")
			return identity.Session{}, identity.User{}, false
		}
		if !mustChange {
			return session, user, true
		}
		respondError(c, http.StatusForbidden, "PASSWORD_CHANGE_REQUIRED", "The initial password must be changed")
		return identity.Session{}, identity.User{}, false
	}
	return session, user, ok
}

func (h *authHandler) authenticateAllowPasswordChange(c *gin.Context) (identity.Session, identity.User, bool) {
	token, ok := h.requiredSessionToken(c)
	if !ok {
		return identity.Session{}, identity.User{}, false
	}
	session, user, err := h.flow.Authenticate(c.Request.Context(), token)
	if err != nil {
		if !h.respondCommonAuthenticationError(c, err) {
			recordRequestError(c, "authenticate session", err)
			respondError(c, http.StatusServiceUnavailable, "SESSION_STATE_UNAVAILABLE", "Authentication state is unavailable")
		}
		return identity.Session{}, identity.User{}, false
	}
	return session, user, true
}

func (h *authHandler) respondCommonAuthenticationError(c *gin.Context, err error) bool {
	if errors.Is(err, context.Canceled) {
		respondRequestCanceled(c)
		return true
	}
	if !errors.Is(err, identityapp.ErrSessionInvalid) {
		return false
	}
	h.clearSessionCookies(c)
	respondError(c, http.StatusUnauthorized, "SESSION_INVALID", "Authentication session is invalid")
	return true
}

func respondPasswordStateError(c *gin.Context, err error) {
	if errors.Is(err, context.Canceled) {
		respondRequestCanceled(c)
		return
	}
	respondError(c, http.StatusInternalServerError, "SESSION_STATE_UNAVAILABLE", "Authentication state is unavailable")
}

func responseMeta(c *gin.Context) contract.ResponseMeta {
	return contract.ResponseMeta{RequestId: c.GetHeader(requestIDHeader), Timestamp: time.Now().UTC()}
}
func respondRequestCanceled(c *gin.Context) {
	c.Status(statusClientClosedRequest)
}

var _ authFlow = (*identityapp.AuthFlow)(nil)
var _ localAuth = (*identityapp.LocalAuthService)(nil)
