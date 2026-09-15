package httpserver

import (
	"context"
	"net/http"

	"github.com/gin-gonic/gin"
	"github.com/google/uuid"

	identity "github.com/vincent119/ReleaseHub/Server/internal/identity/domain"
	contract "github.com/vincent119/ReleaseHub/Server/internal/transport/openapi"
)

type localAuth interface {
	Login(context.Context, string, string) (identity.User, bool, identity.Session, string, string, error)
	ChangePassword(context.Context, uuid.UUID, string, string) error
	MustChangePassword(context.Context, uuid.UUID) (bool, error)
}

func (h *authHandler) LoginLocal(c *gin.Context) {
	c.Header("Cache-Control", "no-store")
	if h.local == nil {
		respondError(c, http.StatusNotFound, "LOCAL_AUTH_UNAVAILABLE", "Local authentication is unavailable")
		return
	}
	request, ok := h.localLoginRequest(c)
	if !ok {
		return
	}
	user, mustChange, session, sessionToken, csrfToken, err := h.local.Login(c.Request.Context(), request.Username, request.Password)
	if err != nil {
		respondError(c, http.StatusUnauthorized, "INVALID_CREDENTIALS", "Username or password is invalid")
		return
	}
	h.setSessionCookies(c, sessionToken, csrfToken)
	c.JSON(http.StatusOK, contract.AuthSessionResponse{Data: contract.AuthSession{
		UserId:                  user.ID,
		Username:                user.Username,
		MustChangePassword:      mustChange,
		PasswordChangeAvailable: true,
		IdleExpiresAt:           session.IdleExpiresAt.UTC(),
		AbsoluteExpiresAt:       session.AbsoluteExpiresAt.UTC(),
	}, Meta: responseMeta(c)})
}

func (h *authHandler) localLoginRequest(c *gin.Context) (contract.LocalLoginRequest, bool) {
	if !h.mutationOriginAllowed(c) {
		return contract.LocalLoginRequest{}, false
	}
	var request contract.LocalLoginRequest
	if err := c.ShouldBindJSON(&request); err != nil {
		respondError(c, http.StatusBadRequest, "INVALID_REQUEST", "Request body is invalid")
		return contract.LocalLoginRequest{}, false
	}
	return request, true
}

func (h *authHandler) ChangeLocalPassword(c *gin.Context, params contract.ChangeLocalPasswordParams) {
	if h.local == nil {
		respondError(c, http.StatusNotFound, "LOCAL_AUTH_UNAVAILABLE", "Local authentication is unavailable")
		return
	}
	_, user, ok := h.authenticateMutationForPasswordChange(c, params.XCSRFToken)
	if !ok {
		return
	}
	request, ok := passwordChangeRequest(c)
	if !ok {
		return
	}
	if err := h.local.ChangePassword(c.Request.Context(), user.ID, request.CurrentPassword, request.NewPassword); err != nil {
		respondError(c, http.StatusBadRequest, "PASSWORD_CHANGE_REJECTED", "Password change was rejected")
		return
	}
	h.clearSessionCookies(c)
	c.Status(http.StatusNoContent)
}

func passwordChangeRequest(c *gin.Context) (contract.ChangePasswordRequest, bool) {
	var request contract.ChangePasswordRequest
	if err := c.ShouldBindJSON(&request); err != nil {
		respondError(c, http.StatusBadRequest, "INVALID_REQUEST", "Request body is invalid")
		return contract.ChangePasswordRequest{}, false
	}
	return request, true
}
