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

type planDefinitionService interface {
	List(context.Context, deployapp.PlanPrincipal, uuid.UUID) ([]deploydomain.DeploymentPlan, error)
	Create(context.Context, deployapp.PlanPrincipal, deployapp.CreatePlanInput) (deploydomain.DeploymentPlan, error)
	CreateVersion(context.Context, deployapp.PlanPrincipal, deployapp.CreatePlanVersionInput) (deploydomain.DeploymentPlanVersion, error)
	ChangeLifecycle(context.Context, deployapp.PlanPrincipal, deployapp.ChangePlanLifecycleInput) (deploydomain.DeploymentPlanVersion, error)
}

type deploymentBindingService interface {
	Get(context.Context, deployapp.PlanPrincipal, deployapp.BindingScopeInput) (*deploydomain.DeploymentBinding, error)
	Bind(context.Context, deployapp.PlanPrincipal, deployapp.BindDefinitionsInput) (deploydomain.DeploymentBinding, error)
}

type planHandler struct {
	authn       *authHandler
	definitions planDefinitionService
	bindings    deploymentBindingService
}

// GetDeploymentBinding returns the current Environment binding or an explicit null.
func (h *planHandler) GetDeploymentBinding(c *gin.Context, params contract.GetDeploymentBindingParams) {
	principal, ok := h.authenticateBinding(c, "")
	if !ok {
		return
	}
	value, err := h.bindings.Get(c.Request.Context(), principal, deployapp.BindingScopeInput{
		OrganizationID: params.OrganizationId, ProjectID: params.ProjectId, EnvironmentID: params.EnvironmentId,
	})
	if !respondBindingError(c, err) {
		return
	}
	c.JSON(http.StatusOK, bindingResponse(value, c))
}

// BindDeploymentDefinitions creates or switches an Environment binding.
func (h *planHandler) BindDeploymentDefinitions(c *gin.Context, params contract.BindDeploymentDefinitionsParams) {
	principal, ok := h.authenticateBinding(c, string(params.XCSRFToken))
	if !ok {
		return
	}
	input, err := bindDefinitionsInput(c)
	if err != nil {
		respondInvalidPlanRequest(c)
		return
	}
	value, err := h.bindings.Bind(c.Request.Context(), principal, input)
	if !respondBindingError(c, err) {
		return
	}
	c.JSON(http.StatusOK, bindingResponse(&value, c))
}

// ListDeploymentPlans returns platform and Project Plans visible to the caller.
func (h *planHandler) ListDeploymentPlans(c *gin.Context, params contract.ListDeploymentPlansParams) {
	principal, ok := h.authenticate(c)
	if !ok {
		return
	}
	values, err := h.definitions.List(c.Request.Context(), principal, params.ProjectId)
	if err != nil {
		respondPlanReadError(c, err)
		return
	}
	c.JSON(http.StatusOK, contract.DeploymentPlanListResponse{Data: planResponses(values), Meta: responseMeta(c)})
}

// CreateDeploymentPlan creates a Plan and its first Draft Version.
func (h *planHandler) CreateDeploymentPlan(c *gin.Context, params contract.CreateDeploymentPlanParams) {
	principal, ok := h.authenticateMutation(c, params.XCSRFToken)
	if !ok {
		return
	}
	input, err := createPlanInput(c)
	if err != nil {
		respondInvalidPlanRequest(c)
		return
	}
	value, err := h.definitions.Create(c.Request.Context(), principal, input)
	if !respondPlanMutationError(c, err, http.StatusConflict) {
		return
	}
	c.JSON(http.StatusCreated, contract.DeploymentPlanResponse{Data: planResponse(value), Meta: responseMeta(c)})
}

// CreateDeploymentPlanVersion appends one immutable Draft Version.
func (h *planHandler) CreateDeploymentPlanVersion(c *gin.Context, planID uuid.UUID, params contract.CreateDeploymentPlanVersionParams) {
	principal, ok := h.authenticateMutation(c, params.XCSRFToken)
	if !ok {
		return
	}
	input, err := createPlanVersionInput(c, planID)
	if err != nil {
		respondInvalidPlanRequest(c)
		return
	}
	value, err := h.definitions.CreateVersion(c.Request.Context(), principal, input)
	if !respondPlanMutationError(c, err, http.StatusConflict) {
		return
	}
	c.JSON(http.StatusCreated, contract.DeploymentPlanVersionResponse{Data: planVersionResponse(value), Meta: responseMeta(c)})
}

// ChangeDeploymentPlanVersionLifecycle publishes or disables one Plan Version.
// codequality:allow-parameters Generated OpenAPI contract requires this signature.
func (h *planHandler) ChangeDeploymentPlanVersionLifecycle(c *gin.Context, planID, versionID uuid.UUID, params contract.ChangeDeploymentPlanVersionLifecycleParams) {
	principal, ok := h.authenticateMutation(c, params.XCSRFToken)
	if !ok {
		return
	}
	input, err := changePlanLifecycleInput(c, planID, versionID)
	if err != nil {
		respondInvalidPlanRequest(c)
		return
	}
	value, err := h.definitions.ChangeLifecycle(c.Request.Context(), principal, input)
	if !respondPlanMutationError(c, err, http.StatusUnprocessableEntity) {
		return
	}
	c.JSON(http.StatusOK, contract.DeploymentPlanVersionResponse{Data: planVersionResponse(value), Meta: responseMeta(c)})
}

func (h *planHandler) authenticate(c *gin.Context) (deployapp.PlanPrincipal, bool) {
	_, user, ok := h.authn.authenticate(c)
	if !ok {
		return deployapp.PlanPrincipal{}, false
	}
	if h.definitions == nil {
		respondError(c, http.StatusServiceUnavailable, "DEPLOYMENT_PLAN_UNAVAILABLE", "Deployment Plan is unavailable")
		return deployapp.PlanPrincipal{}, false
	}
	return deployapp.PlanPrincipal{UserID: user.ID, Disabled: user.Disabled}, true
}

func (h *planHandler) authenticateMutation(c *gin.Context, csrf string) (deployapp.PlanPrincipal, bool) {
	_, user, ok := h.authn.authenticateMutation(c, csrf)
	if !ok {
		return deployapp.PlanPrincipal{}, false
	}
	if h.definitions == nil {
		respondError(c, http.StatusServiceUnavailable, "DEPLOYMENT_PLAN_UNAVAILABLE", "Deployment Plan is unavailable")
		return deployapp.PlanPrincipal{}, false
	}
	return deployapp.PlanPrincipal{UserID: user.ID, Disabled: user.Disabled}, true
}

func (h *planHandler) authenticateBinding(c *gin.Context, csrf string) (deployapp.PlanPrincipal, bool) {
	var userPrincipal deployapp.PlanPrincipal
	var ok bool
	if csrf == "" {
		userPrincipal, ok = h.authenticate(c)
	} else {
		userPrincipal, ok = h.authenticateMutation(c, csrf)
	}
	if !ok {
		return deployapp.PlanPrincipal{}, false
	}
	if h.bindings == nil {
		respondError(c, http.StatusServiceUnavailable, "DEPLOYMENT_BINDING_UNAVAILABLE", "Deployment binding is unavailable")
		return deployapp.PlanPrincipal{}, false
	}
	return userPrincipal, true
}

func respondPlanReadError(c *gin.Context, err error) {
	if errors.Is(err, deployapp.ErrPlanForbidden) || errors.Is(err, deployapp.ErrPlanNotFound) {
		respondError(c, http.StatusNotFound, "DEPLOYMENT_PLAN_NOT_FOUND", "Deployment Plan was not found")
		return
	}
	respondError(c, http.StatusInternalServerError, "DEPLOYMENT_PLAN_READ_FAILED", "Unable to read Deployment Plans")
}

func respondPlanMutationError(c *gin.Context, err error, invalidStatus int) bool {
	if err == nil {
		return true
	}
	if errors.Is(err, deployapp.ErrPlanForbidden) || errors.Is(err, deployapp.ErrPlanNotFound) {
		respondError(c, http.StatusNotFound, "DEPLOYMENT_PLAN_NOT_FOUND", "Deployment Plan was not found")
		return false
	}
	if errors.Is(err, deployapp.ErrPlanInvalid) {
		respondError(c, invalidStatus, "DEPLOYMENT_PLAN_INVALID", "Deployment Plan is invalid")
		return false
	}
	if errors.Is(err, deployapp.ErrPlanConflict) {
		respondError(c, http.StatusConflict, "DEPLOYMENT_PLAN_CONFLICT", "Deployment Plan change was rejected")
		return false
	}
	respondError(c, http.StatusInternalServerError, "DEPLOYMENT_PLAN_MUTATION_FAILED", "Unable to change Deployment Plan")
	return false
}

func respondInvalidPlanRequest(c *gin.Context) {
	respondError(c, http.StatusBadRequest, "INVALID_REQUEST", "Deployment Plan request is invalid")
}

func respondBindingError(c *gin.Context, err error) bool {
	if err == nil {
		return true
	}
	if errors.Is(err, deployapp.ErrBindingForbidden) || errors.Is(err, deployapp.ErrBindingNotFound) {
		respondError(c, http.StatusNotFound, "DEPLOYMENT_BINDING_NOT_FOUND", "Deployment binding scope was not found")
		return false
	}
	if errors.Is(err, deployapp.ErrBindingConflict) {
		respondError(c, http.StatusConflict, "DEPLOYMENT_BINDING_CONFLICT", "Deployment binding change was rejected")
		return false
	}
	respondError(c, http.StatusInternalServerError, "DEPLOYMENT_BINDING_FAILED", "Unable to read or change deployment binding")
	return false
}
