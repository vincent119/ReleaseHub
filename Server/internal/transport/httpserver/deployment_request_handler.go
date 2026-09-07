package httpserver

import (
	"context"
	"errors"
	"net/http"

	"github.com/gin-gonic/gin"
	"github.com/google/uuid"

	authz "github.com/vincent119/ReleaseHub/Server/internal/authorization/domain"
	deployapp "github.com/vincent119/ReleaseHub/Server/internal/deployment/application"
	deploydomain "github.com/vincent119/ReleaseHub/Server/internal/deployment/domain"
	contract "github.com/vincent119/ReleaseHub/Server/internal/transport/openapi"
)

type deploymentRequestService interface {
	List(context.Context, deployapp.RequestPrincipal, authz.Scope) ([]deploydomain.DeploymentRequestSummary, error)
	Get(context.Context, deployapp.RequestPrincipal, uuid.UUID) (deploydomain.DeploymentRequestDetail, error)
	UpdateMetadata(context.Context, deployapp.RequestPrincipal, deployapp.MetadataVersionChange, string) (deploydomain.DeploymentRequestDetail, error)
}

// ListDeploymentRequests returns Requests visible in the requested Environment scope.
func (h *deploymentHandler) ListDeploymentRequests(c *gin.Context, params contract.ListDeploymentRequestsParams) {
	principal, ok := h.requestPrincipal(c, "")
	if !ok {
		return
	}
	scope, err := authz.NewEnvironmentScope(params.OrganizationId, params.ProjectId, params.EnvironmentId)
	if err != nil {
		respondInvalidDeploymentRequest(c)
		return
	}
	values, err := h.requests.List(c.Request.Context(), principal, scope)
	if !respondDeploymentRequestError(c, err, http.StatusInternalServerError) {
		return
	}
	meta := responseMeta(c)
	c.JSON(http.StatusOK, contract.DeploymentRequestListResponse{Data: deploymentRequestSummaries(values), Meta: contract.CursorPageMeta{RequestId: meta.RequestId, Timestamp: meta.Timestamp}})
}

// GetDeploymentRequest returns the latest immutable Version and its snapshots.
func (h *deploymentHandler) GetDeploymentRequest(c *gin.Context, requestID contract.RequestId) {
	principal, ok := h.requestPrincipal(c, "")
	if !ok {
		return
	}
	value, err := h.requests.Get(c.Request.Context(), principal, requestID)
	if !respondDeploymentRequestError(c, err, http.StatusInternalServerError) {
		return
	}
	c.JSON(http.StatusOK, contract.DeploymentRequestDetailResponse{Data: deploymentRequestDetail(value), Meta: responseMeta(c)})
}

// UpdateDeploymentRequestVersionMetadata creates a new immutable reviewed Version.
// codequality:allow-parameters Generated OpenAPI contract requires this signature.
func (h *deploymentHandler) UpdateDeploymentRequestVersionMetadata(c *gin.Context, requestID, versionID uuid.UUID, params contract.UpdateDeploymentRequestVersionMetadataParams) {
	principal, ok := h.requestPrincipal(c, string(params.XCSRFToken))
	if !ok {
		return
	}
	input, err := deploymentRequestMetadataInput(c, requestID, versionID)
	if err != nil {
		respondInvalidDeploymentRequest(c)
		return
	}
	value, err := h.requests.UpdateMetadata(c.Request.Context(), principal, input, c.GetHeader(requestIDHeader))
	if !respondDeploymentRequestError(c, err, http.StatusConflict) {
		return
	}
	c.JSON(http.StatusOK, contract.DeploymentRequestVersionResponse{Data: deploymentRequestDetail(value), Meta: responseMeta(c)})
}

func (h *deploymentHandler) requestPrincipal(c *gin.Context, csrf string) (deployapp.RequestPrincipal, bool) {
	if h.requests == nil {
		if csrf == "" {
			h.requireDeploymentRead(c)
		} else {
			h.requireDeploymentMutation(c, contract.CsrfToken(csrf))
		}
		return deployapp.RequestPrincipal{}, false
	}
	if csrf == "" {
		_, user, ok := h.authn.authenticate(c)
		return deployapp.RequestPrincipal{UserID: user.ID, Disabled: user.Disabled}, ok
	}
	_, user, ok := h.authn.authenticateMutation(c, csrf)
	return deployapp.RequestPrincipal{UserID: user.ID, Disabled: user.Disabled}, ok
}

func respondDeploymentRequestError(c *gin.Context, err error, conflictStatus int) bool {
	if err == nil {
		return true
	}
	if errors.Is(err, deployapp.ErrRequestForbidden) || errors.Is(err, deployapp.ErrRequestNotFound) {
		respondError(c, http.StatusNotFound, "DEPLOYMENT_REQUEST_NOT_FOUND", "Deployment Request was not found")
		return false
	}
	if errors.Is(err, deployapp.ErrRequestInvalid) {
		respondError(c, http.StatusUnprocessableEntity, "DEPLOYMENT_REQUEST_INVALID", "Deployment Request is invalid")
		return false
	}
	if errors.Is(err, deployapp.ErrRequestConflict) {
		respondError(c, conflictStatus, "DEPLOYMENT_REQUEST_CONFLICT", "Deployment Request changed concurrently")
		return false
	}
	respondError(c, http.StatusInternalServerError, "DEPLOYMENT_REQUEST_FAILED", "Unable to read Deployment Request")
	return false
}

func respondInvalidDeploymentRequest(c *gin.Context) {
	respondError(c, http.StatusBadRequest, "INVALID_REQUEST", "Deployment Request is invalid")
}
