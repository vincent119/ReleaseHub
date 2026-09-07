package httpserver

import (
	"net/http"

	"github.com/gin-gonic/gin"

	contract "github.com/vincent119/ReleaseHub/Server/internal/transport/openapi"
)

type deploymentHandler struct {
	authn         *authHandler
	requests      deploymentRequestService
	workflows     deploymentWorkflowService
	executions    deploymentExecutionService
	history       deploymentHistoryService
	notifications deploymentNotificationService
}

func (h *deploymentHandler) requireDeploymentRead(c *gin.Context) bool {
	_, _, ok := h.authn.authenticate(c)
	if !ok {
		return false
	}
	respondError(c, http.StatusNotImplemented, "DEPLOYMENT_FEATURE_UNAVAILABLE", "Deployment feature is not available")
	return false
}

func (h *deploymentHandler) requireDeploymentMutation(c *gin.Context, csrfToken contract.CsrfToken) bool {
	_, _, ok := h.authn.authenticateMutation(c, string(csrfToken))
	if !ok {
		return false
	}
	respondError(c, http.StatusNotImplemented, "DEPLOYMENT_FEATURE_UNAVAILABLE", "Deployment feature is not available")
	return false
}
