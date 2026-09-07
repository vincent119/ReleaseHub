package httpserver

import (
	"context"
	"errors"
	"net/http"

	"github.com/gin-gonic/gin"
	"github.com/google/uuid"

	deployapp "github.com/vincent119/ReleaseHub/Server/internal/deployment/application"
	deploydomain "github.com/vincent119/ReleaseHub/Server/internal/deployment/domain"
	contract "github.com/vincent119/ReleaseHub/Server/internal/transport/openapi"
)

type deploymentWorkflowService interface {
	Transition(context.Context, deployapp.WorkflowPrincipal, deployapp.WorkflowTransitionInput) (deploydomain.WorkflowResult, error)
	DecideReview(context.Context, deployapp.WorkflowPrincipal, deployapp.WorkflowReviewInput) (deploydomain.ReviewTask, error)
	ReassignReview(context.Context, deployapp.WorkflowPrincipal, deployapp.WorkflowReviewReassignmentInput) (deploydomain.ReviewTask, error)
}

type deploymentWorkflowContext struct {
	workflow deployapp.WorkflowPrincipal
	request  deployapp.RequestPrincipal
}

// DecideDeploymentReview records one authorized decision against the pinned workflow.
// codequality:allow-parameters Generated OpenAPI contract requires this signature.
func (h *deploymentHandler) DecideDeploymentReview(c *gin.Context, requestID, versionID, taskID uuid.UUID, params contract.DecideDeploymentReviewParams) {
	principal, ok := h.prepareWorkflowMutation(c, requestID, versionID, params.XCSRFToken)
	if !ok {
		return
	}
	input, err := deploymentReviewInput(c, versionID, taskID, string(params.IdempotencyKey))
	if err != nil {
		respondInvalidDeploymentRequest(c)
		return
	}
	_, err = h.workflows.DecideReview(c.Request.Context(), principal.workflow, input)
	if respondDeploymentWorkflowError(c, err) {
		h.respondWorkflowRequest(c, principal.request, requestID, http.StatusOK)
	}
}

// ReassignDeploymentReview replaces eligible reviewers after management authorization.
// codequality:allow-parameters Generated OpenAPI contract requires this signature.
func (h *deploymentHandler) ReassignDeploymentReview(c *gin.Context, requestID, versionID, taskID uuid.UUID, params contract.ReassignDeploymentReviewParams) {
	principal, ok := h.prepareWorkflowMutation(c, requestID, versionID, params.XCSRFToken)
	if !ok {
		return
	}
	input, err := deploymentReviewReassignmentInput(c, versionID, taskID, string(params.IdempotencyKey))
	if err != nil {
		respondInvalidDeploymentRequest(c)
		return
	}
	_, err = h.workflows.ReassignReview(c.Request.Context(), principal.workflow, input)
	if respondDeploymentWorkflowError(c, err) {
		h.respondWorkflowRequest(c, principal.request, requestID, http.StatusOK)
	}
}

// TransitionDeploymentRequest executes one user-requested manual workflow edge.
func (h *deploymentHandler) TransitionDeploymentRequest(c *gin.Context, requestID, versionID uuid.UUID, params contract.TransitionDeploymentRequestParams) {
	principal, ok := h.prepareWorkflowMutation(c, requestID, versionID, params.XCSRFToken)
	if !ok {
		return
	}
	input, err := deploymentTransitionInput(c, versionID, string(params.IdempotencyKey))
	if err != nil {
		respondInvalidDeploymentRequest(c)
		return
	}
	_, err = h.workflows.Transition(c.Request.Context(), principal.workflow, input)
	if respondDeploymentWorkflowError(c, err) {
		h.respondWorkflowRequest(c, principal.request, requestID, http.StatusAccepted)
	}
}

func (h *deploymentHandler) prepareWorkflowMutation(c *gin.Context, requestID, versionID uuid.UUID, csrf contract.CsrfToken) (deploymentWorkflowContext, bool) {
	if h.workflows == nil || h.requests == nil {
		h.requireDeploymentMutation(c, csrf)
		return deploymentWorkflowContext{}, false
	}
	_, user, ok := h.authn.authenticateMutation(c, string(csrf))
	if !ok {
		return deploymentWorkflowContext{}, false
	}
	principal := deploymentWorkflowContext{workflow: deployapp.WorkflowPrincipal{UserID: user.ID, Disabled: user.Disabled}, request: deployapp.RequestPrincipal{UserID: user.ID, Disabled: user.Disabled}}
	value, err := h.requests.Get(c.Request.Context(), principal.request, requestID)
	if !respondDeploymentRequestError(c, err, http.StatusConflict) || value.Version.ID != versionID {
		if err == nil && value.Version.ID != versionID {
			respondError(c, http.StatusNotFound, "DEPLOYMENT_REQUEST_NOT_FOUND", "Deployment Request was not found")
		}
		return deploymentWorkflowContext{}, false
	}
	return principal, true
}

func (h *deploymentHandler) respondWorkflowRequest(c *gin.Context, principal deployapp.RequestPrincipal, requestID uuid.UUID, status int) {
	value, err := h.requests.Get(c.Request.Context(), principal, requestID)
	if !respondDeploymentRequestError(c, err, http.StatusConflict) {
		return
	}
	c.JSON(status, contract.DeploymentRequestVersionResponse{Data: deploymentRequestDetail(value), Meta: responseMeta(c)})
}

func respondDeploymentWorkflowError(c *gin.Context, err error) bool {
	if err == nil {
		return true
	}
	if errors.Is(err, deployapp.ErrWorkflowForbidden) || errors.Is(err, deployapp.ErrWorkflowRuntimeNotFound) {
		respondError(c, http.StatusNotFound, "DEPLOYMENT_WORKFLOW_NOT_FOUND", "Deployment workflow was not found")
		return false
	}
	if errors.Is(err, deployapp.ErrWorkflowRuntimeConflict) {
		respondError(c, http.StatusConflict, "DEPLOYMENT_WORKFLOW_CONFLICT", "Deployment workflow changed concurrently")
		return false
	}
	if errors.Is(err, deployapp.ErrWorkflowRuntimeInvalid) {
		respondError(c, http.StatusUnprocessableEntity, "DEPLOYMENT_WORKFLOW_INVALID", "Deployment workflow transition is invalid")
		return false
	}
	respondError(c, http.StatusInternalServerError, "DEPLOYMENT_WORKFLOW_FAILED", "Unable to update deployment workflow")
	return false
}
