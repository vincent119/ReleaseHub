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

type workflowDefinitionService interface {
	List(context.Context, deployapp.WorkflowPrincipal) ([]deploydomain.ReleaseWorkflow, error)
	ReviewOptions(context.Context, deployapp.WorkflowPrincipal) (deployapp.WorkflowReviewOptions, error)
	Create(context.Context, deployapp.WorkflowPrincipal, deployapp.CreateWorkflowInput) (deploydomain.ReleaseWorkflow, error)
	CreateVersion(context.Context, deployapp.WorkflowPrincipal, deployapp.CreateWorkflowVersionInput) (deploydomain.ReleaseWorkflowVersion, error)
	ChangeLifecycle(context.Context, deployapp.WorkflowPrincipal, deployapp.ChangeWorkflowLifecycleInput) (deploydomain.ReleaseWorkflowVersion, error)
	Delete(context.Context, deployapp.WorkflowPrincipal, deployapp.DeleteWorkflowInput) error
}

type workflowHandler struct {
	authn       *authHandler
	definitions workflowDefinitionService
}

// ListReleaseWorkflows returns centrally managed workflow definitions.
func (h *workflowHandler) ListReleaseWorkflows(c *gin.Context) {
	principal, ok := h.authenticate(c)
	if !ok {
		return
	}
	values, err := h.definitions.List(c.Request.Context(), principal)
	if err != nil {
		respondWorkflowReadError(c, err)
		return
	}
	c.JSON(http.StatusOK, contract.ReleaseWorkflowListResponse{Data: workflowResponses(values), Meta: responseMeta(c)})
}

// GetReleaseWorkflowReviewOptions returns minimal identities for Review policy assignment.
func (h *workflowHandler) GetReleaseWorkflowReviewOptions(c *gin.Context) {
	principal, ok := h.authenticate(c)
	if !ok {
		return
	}
	value, err := h.definitions.ReviewOptions(c.Request.Context(), principal)
	if err != nil {
		respondWorkflowReadError(c, err)
		return
	}
	c.JSON(http.StatusOK, contract.ReleaseWorkflowReviewOptionsResponse{
		Data: workflowReviewOptionsResponse(value), Meta: responseMeta(c),
	})
}

// CreateReleaseWorkflow creates a workflow with its first immutable draft.
func (h *workflowHandler) CreateReleaseWorkflow(c *gin.Context, params contract.CreateReleaseWorkflowParams) {
	principal, ok := h.authenticateMutation(c, params.XCSRFToken)
	if !ok {
		return
	}
	input, err := createWorkflowInput(c)
	if err != nil {
		respondInvalidWorkflowRequest(c)
		return
	}
	value, err := h.definitions.Create(c.Request.Context(), principal, input)
	if !respondWorkflowMutationError(c, err, http.StatusConflict) {
		return
	}
	c.JSON(http.StatusCreated, contract.ReleaseWorkflowResponse{Data: workflowResponse(value), Meta: responseMeta(c)})
}

// DeleteReleaseWorkflow removes an unused workflow whose versions never left Draft.
func (h *workflowHandler) DeleteReleaseWorkflow(c *gin.Context, workflowID uuid.UUID, params contract.DeleteReleaseWorkflowParams) {
	principal, ok := h.authenticateMutation(c, params.XCSRFToken)
	if !ok {
		return
	}
	input, err := deleteWorkflowInput(c, workflowID, params.ExpectedVersion)
	if err != nil {
		respondInvalidWorkflowRequest(c)
		return
	}
	if !respondWorkflowMutationError(c, h.definitions.Delete(c.Request.Context(), principal, input), http.StatusBadRequest) {
		return
	}
	c.Status(http.StatusNoContent)
}

// CreateReleaseWorkflowVersion appends one draft to an existing workflow.
func (h *workflowHandler) CreateReleaseWorkflowVersion(c *gin.Context, workflowID uuid.UUID, params contract.CreateReleaseWorkflowVersionParams) {
	principal, ok := h.authenticateMutation(c, params.XCSRFToken)
	if !ok {
		return
	}
	input, err := createWorkflowVersionInput(c, workflowID)
	if err != nil {
		respondInvalidWorkflowRequest(c)
		return
	}
	value, err := h.definitions.CreateVersion(c.Request.Context(), principal, input)
	if !respondWorkflowMutationError(c, err, http.StatusConflict) {
		return
	}
	c.JSON(http.StatusCreated, contract.ReleaseWorkflowVersionResponse{Data: workflowVersionResponse(value), Meta: responseMeta(c)})
}

// ChangeReleaseWorkflowVersionLifecycle publishes or disables one version.
// codequality:allow-parameters Generated OpenAPI contract requires this signature.
func (h *workflowHandler) ChangeReleaseWorkflowVersionLifecycle(c *gin.Context, workflowID, versionID uuid.UUID, params contract.ChangeReleaseWorkflowVersionLifecycleParams) {
	principal, ok := h.authenticateMutation(c, params.XCSRFToken)
	if !ok {
		return
	}
	input, err := changeWorkflowLifecycleInput(c, workflowID, versionID)
	if err != nil {
		respondInvalidWorkflowRequest(c)
		return
	}
	value, err := h.definitions.ChangeLifecycle(c.Request.Context(), principal, input)
	if !respondWorkflowMutationError(c, err, http.StatusUnprocessableEntity) {
		return
	}
	c.JSON(http.StatusOK, contract.ReleaseWorkflowVersionResponse{Data: workflowVersionResponse(value), Meta: responseMeta(c)})
}

func (h *workflowHandler) authenticate(c *gin.Context) (deployapp.WorkflowPrincipal, bool) {
	_, user, ok := h.authn.authenticate(c)
	if !ok {
		return deployapp.WorkflowPrincipal{}, false
	}
	if h.definitions == nil {
		respondError(c, http.StatusServiceUnavailable, "WORKFLOW_UNAVAILABLE", "Release workflow is unavailable")
		return deployapp.WorkflowPrincipal{}, false
	}
	return deployapp.WorkflowPrincipal{UserID: user.ID, Disabled: user.Disabled}, true
}

func (h *workflowHandler) authenticateMutation(c *gin.Context, csrf string) (deployapp.WorkflowPrincipal, bool) {
	_, user, ok := h.authn.authenticateMutation(c, csrf)
	if !ok {
		return deployapp.WorkflowPrincipal{}, false
	}
	if h.definitions == nil {
		respondError(c, http.StatusServiceUnavailable, "WORKFLOW_UNAVAILABLE", "Release workflow is unavailable")
		return deployapp.WorkflowPrincipal{}, false
	}
	return deployapp.WorkflowPrincipal{UserID: user.ID, Disabled: user.Disabled}, true
}

func respondWorkflowReadError(c *gin.Context, err error) {
	if errors.Is(err, deployapp.ErrWorkflowForbidden) || errors.Is(err, deployapp.ErrWorkflowNotFound) {
		respondError(c, http.StatusNotFound, "WORKFLOW_NOT_FOUND", "Release workflow was not found")
		return
	}
	respondError(c, http.StatusInternalServerError, "WORKFLOW_READ_FAILED", "Unable to read release workflows")
}

func respondWorkflowMutationError(c *gin.Context, err error, invalidStatus int) bool {
	if err == nil {
		return true
	}
	if errors.Is(err, deployapp.ErrWorkflowForbidden) || errors.Is(err, deployapp.ErrWorkflowNotFound) {
		respondError(c, http.StatusNotFound, "WORKFLOW_NOT_FOUND", "Release workflow was not found")
		return false
	}
	if errors.Is(err, deployapp.ErrWorkflowInvalid) {
		respondError(c, invalidStatus, "WORKFLOW_INVALID", "Release workflow is invalid")
		return false
	}
	if errors.Is(err, deployapp.ErrWorkflowConflict) {
		respondError(c, http.StatusConflict, "WORKFLOW_CONFLICT", "Release workflow change was rejected")
		return false
	}
	respondError(c, http.StatusInternalServerError, "WORKFLOW_MUTATION_FAILED", "Unable to change release workflow")
	return false
}

func respondInvalidWorkflowRequest(c *gin.Context) {
	respondError(c, http.StatusBadRequest, "INVALID_REQUEST", "Release workflow request is invalid")
}
