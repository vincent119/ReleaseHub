package httpserver

import (
	"context"
	"errors"
	"net/http"

	"github.com/gin-gonic/gin"
	"github.com/google/uuid"

	deployapp "github.com/vincent119/ReleaseHub/Server/internal/deployment/application"
	contract "github.com/vincent119/ReleaseHub/Server/internal/transport/openapi"
)

type deploymentExecutionService interface {
	Get(context.Context, deployapp.RequestPrincipal, uuid.UUID) (deployapp.DeploymentExecution, error)
	Retry(context.Context, deployapp.RequestPrincipal, deployapp.RetryExecutionInput) (deployapp.DeploymentExecution, error)
	Terminate(context.Context, deployapp.RequestPrincipal, deployapp.TerminateExecutionInput) (deployapp.DeploymentExecution, error)
	Unlock(context.Context, deployapp.RequestPrincipal, deployapp.UnlockExecutionInput) (deployapp.DeploymentExecution, error)
}

// GetDeploymentExecution returns one authorized execution projection.
func (h *deploymentHandler) GetDeploymentExecution(c *gin.Context, executionID contract.ExecutionId) {
	principal, ok := h.executionPrincipal(c, "")
	if !ok {
		return
	}
	value, err := h.executions.Get(c.Request.Context(), principal, executionID)
	if !respondExecutionError(c, err) {
		return
	}
	c.JSON(http.StatusOK, contract.DeploymentExecutionResponse{Data: deploymentExecutionResponse(value), Meta: responseMeta(c)})
}

// RetryDeploymentRequest queues one workflow-governed failed-only business retry.
// codequality:allow-parameters Generated OpenAPI contract requires this signature.
func (h *deploymentHandler) RetryDeploymentRequest(c *gin.Context, requestID, versionID uuid.UUID, params contract.RetryDeploymentRequestParams) {
	principal, ok := h.executionPrincipal(c, string(params.XCSRFToken))
	if !ok {
		return
	}
	input, err := retryExecutionInput(c, requestID, versionID, string(params.IdempotencyKey))
	if err != nil {
		respondInvalidExecution(c)
		return
	}
	value, err := h.executions.Retry(c.Request.Context(), principal, input)
	respondExecutionAccepted(c, value, err)
}

// TerminateDeploymentRequest stops an unconverged execution but retains its locks.
// codequality:allow-parameters Generated OpenAPI contract requires this signature.
func (h *deploymentHandler) TerminateDeploymentRequest(c *gin.Context, requestID, versionID uuid.UUID, params contract.TerminateDeploymentRequestParams) {
	principal, ok := h.executionPrincipal(c, string(params.XCSRFToken))
	if !ok {
		return
	}
	input, err := terminateExecutionInput(c, requestID, versionID, string(params.IdempotencyKey))
	if err != nil {
		respondInvalidExecution(c)
		return
	}
	value, err := h.executions.Terminate(c.Request.Context(), principal, input)
	respondExecutionAccepted(c, value, err)
}

// UnlockDeploymentRequest releases locks only after current-state confirmation.
// codequality:allow-parameters Generated OpenAPI contract requires this signature.
func (h *deploymentHandler) UnlockDeploymentRequest(c *gin.Context, requestID, versionID uuid.UUID, params contract.UnlockDeploymentRequestParams) {
	principal, ok := h.executionPrincipal(c, string(params.XCSRFToken))
	if !ok {
		return
	}
	input, err := unlockExecutionInput(c, requestID, versionID, string(params.IdempotencyKey))
	if err != nil {
		respondInvalidExecution(c)
		return
	}
	value, err := h.executions.Unlock(c.Request.Context(), principal, input)
	respondExecutionAccepted(c, value, err)
}

func (h *deploymentHandler) executionPrincipal(c *gin.Context, csrf string) (deployapp.RequestPrincipal, bool) {
	if h.executions == nil {
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

func respondExecutionAccepted(c *gin.Context, value deployapp.DeploymentExecution, err error) {
	if !respondExecutionError(c, err) {
		return
	}
	c.JSON(http.StatusAccepted, contract.DeploymentExecutionResponse{Data: deploymentExecutionResponse(value), Meta: responseMeta(c)})
}

func respondExecutionError(c *gin.Context, err error) bool {
	if err == nil {
		return true
	}
	switch {
	case errors.Is(err, deployapp.ErrExecutionNotFound), errors.Is(err, deployapp.ErrExecutionForbidden):
		respondError(c, http.StatusNotFound, "DEPLOYMENT_EXECUTION_NOT_FOUND", "Deployment execution was not found")
	case errors.Is(err, deployapp.ErrExecutionConflict):
		respondError(c, http.StatusConflict, "DEPLOYMENT_EXECUTION_CONFLICT", "Deployment execution changed concurrently")
	case errors.Is(err, deployapp.ErrExecutionInvalid):
		respondError(c, http.StatusUnprocessableEntity, "DEPLOYMENT_EXECUTION_INVALID", "Deployment execution command is invalid")
	default:
		respondError(c, http.StatusInternalServerError, "DEPLOYMENT_EXECUTION_FAILED", "Unable to process deployment execution")
	}
	return false
}

func respondInvalidExecution(c *gin.Context) {
	respondError(c, http.StatusBadRequest, "INVALID_REQUEST", "Deployment execution command is invalid")
}
