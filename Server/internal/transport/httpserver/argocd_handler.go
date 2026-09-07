package httpserver

import (
	"context"
	"errors"
	"net/http"

	"github.com/gin-gonic/gin"
	"github.com/google/uuid"

	argoapp "github.com/vincent119/ReleaseHub/Server/internal/argocd/application"
	argodomain "github.com/vincent119/ReleaseHub/Server/internal/argocd/domain"
	catalog "github.com/vincent119/ReleaseHub/Server/internal/catalog/domain"
	identity "github.com/vincent119/ReleaseHub/Server/internal/identity/domain"
	contract "github.com/vincent119/ReleaseHub/Server/internal/transport/openapi"
)

type onboardingService interface {
	DryRun(context.Context, argoapp.OnboardingPrincipal, argoapp.OnboardingMutation, uuid.UUID) (argodomain.Onboarding, error)
	ConfirmIdempotent(context.Context, argoapp.OnboardingPrincipal, argoapp.OnboardingMutation, uuid.UUID, uint64, string) (argodomain.Onboarding, error)
}

type candidateService interface {
	List(context.Context, argoapp.OnboardingPrincipal) ([]argoapp.Candidate, error)
	ListAssignmentScopes(context.Context, argoapp.OnboardingPrincipal) ([]argoapp.AssignmentScope, error)
	Assign(context.Context, argoapp.OnboardingPrincipal, argoapp.AssignCandidateInput) (catalog.Application, error)
}

type argoCDHandler struct {
	authn      *authHandler
	onboarding onboardingService
	candidates candidateService
}

func (h *argoCDHandler) ListArgoCDCandidates(c *gin.Context) {
	_, user, ok := h.authn.authenticate(c)
	if !ok || !h.candidateServiceAvailable(c) {
		return
	}
	values, err := h.candidates.List(c.Request.Context(), onboardingPrincipal(user))
	if err != nil {
		respondCandidateReadError(c, err, "CANDIDATE_QUEUE_NOT_FOUND", "Argo CD candidate queue was not found")
		return
	}
	c.JSON(http.StatusOK, contract.ArgoCDCandidateListResponse{
		Data: candidateResponses(values), Meta: responseMeta(c),
	})
}

func (h *argoCDHandler) ListArgoCDCandidateAssignmentScopes(c *gin.Context) {
	_, user, ok := h.authn.authenticate(c)
	if !ok || !h.candidateServiceAvailable(c) {
		return
	}
	values, err := h.candidates.ListAssignmentScopes(c.Request.Context(), onboardingPrincipal(user))
	if err != nil {
		respondCandidateReadError(c, err, "CANDIDATE_ASSIGNMENT_SCOPES_NOT_FOUND", "Argo CD candidate assignment scopes were not found")
		return
	}
	c.JSON(http.StatusOK, contract.ArgoCDCandidateAssignmentScopeListResponse{
		Data: assignmentScopeResponses(values), Meta: responseMeta(c),
	})
}

func (h *argoCDHandler) AssignArgoCDCandidate(c *gin.Context, candidateID contract.CandidateId, params contract.AssignArgoCDCandidateParams) {
	_, user, ok := h.authn.authenticateMutation(c, string(params.XCSRFToken))
	if !ok || !h.candidateServiceAvailable(c) {
		return
	}
	body, ok := candidateAssignmentBody(c)
	if !ok {
		return
	}
	value, err := h.candidates.Assign(
		c.Request.Context(), onboardingPrincipal(user),
		candidateAssignmentInput(c, candidateID, body),
	)
	if err != nil {
		respondCandidateAssignmentError(c, err)
		return
	}
	c.JSON(http.StatusCreated, contract.CatalogApplicationResponse{Data: catalogApplicationResponse(value), Meta: responseMeta(c)})
}

func (h *argoCDHandler) DryRunApplicationOnboarding(c *gin.Context, _ contract.ApplicationId, params contract.DryRunApplicationOnboardingParams) {
	_, user, ok := h.authn.authenticateMutation(c, string(params.XCSRFToken))
	if !ok || !h.onboardingAvailable(c) {
		return
	}
	applicationID, _ := uuid.Parse(c.Param("applicationId"))
	state, err := h.onboarding.DryRun(
		c.Request.Context(), onboardingPrincipal(user),
		argoapp.OnboardingMutation{RequestID: c.GetHeader(requestIDHeader)}, applicationID,
	)
	if err != nil {
		respondError(c, http.StatusConflict, "ONBOARDING_DRY_RUN_REJECTED", "Application onboarding validation failed")
		return
	}
	c.JSON(http.StatusOK, onboardingResponse(state, c))
}

func (h *argoCDHandler) ConfirmApplicationOnboarding(c *gin.Context, _ contract.ApplicationId, params contract.ConfirmApplicationOnboardingParams) {
	_, user, ok := h.authn.authenticateMutation(c, string(params.XCSRFToken))
	if !ok || !h.onboardingAvailable(c) {
		return
	}
	expectedVersion, ok := onboardingExpectedVersion(c)
	if !ok {
		return
	}
	state, err := h.confirmOnboarding(c, user, expectedVersion, params)
	if respondOnboardingConfirmation(c, state, err) {
		return
	}
	c.JSON(http.StatusOK, onboardingResponse(state, c))
}

func (h *argoCDHandler) candidateServiceAvailable(c *gin.Context) bool {
	if h.candidates != nil {
		return true
	}
	respondError(c, http.StatusServiceUnavailable, "CANDIDATE_QUEUE_UNAVAILABLE", "Argo CD candidate queue is unavailable")
	return false
}

func (h *argoCDHandler) onboardingAvailable(c *gin.Context) bool {
	if h.onboarding != nil {
		return true
	}
	respondError(c, http.StatusServiceUnavailable, "ONBOARDING_UNAVAILABLE", "Application onboarding is unavailable")
	return false
}

func onboardingPrincipal(user identity.User) argoapp.OnboardingPrincipal {
	return argoapp.OnboardingPrincipal{UserID: user.ID, Disabled: user.Disabled}
}

func respondCandidateReadError(c *gin.Context, err error, notFoundCode, notFoundMessage string) {
	if errors.Is(err, argoapp.ErrCandidateQueueNotFound) {
		respondError(c, http.StatusNotFound, notFoundCode, notFoundMessage)
		return
	}
	respondError(c, http.StatusInternalServerError, "CANDIDATE_QUEUE_READ_FAILED", "Unable to read Argo CD candidate queue")
}

func candidateResponses(values []argoapp.Candidate) []contract.ArgoCDCandidate {
	result := make([]contract.ArgoCDCandidate, 0, len(values))
	for _, value := range values {
		result = append(result, candidateResponse(value))
	}
	return result
}

func candidateResponse(value argoapp.Candidate) contract.ArgoCDCandidate {
	result := contract.ArgoCDCandidate{
		Id: value.ID, ArgocdNamespace: value.ArgoCDNamespace, ArgocdApplicationName: value.ArgoCDApplicationName,
		ArgocdProject: value.ArgoCDProject, DestinationServer: value.DestinationServer, DestinationName: value.DestinationName,
		DestinationNamespace: value.DestinationNamespace, AutomatedSync: value.AutomatedSync, SyncStatus: value.SyncStatus,
		HealthStatus: value.HealthStatus, OperationPhase: value.OperationPhase, ResolvedRevision: value.ResolvedRevision,
		ResourceVersion: value.ResourceVersion, FirstSeenAt: value.FirstSeenAt, LastSeenAt: value.LastSeenAt,
		Version: int64(value.Version),
	}
	for _, source := range value.Sources {
		result.Sources = append(result.Sources, candidateSourceResponse(source))
	}
	return result
}

func candidateSourceResponse(source argodomain.Source) struct {
	Path           string `json:"path"`
	RepositoryUrl  string `json:"repositoryUrl"`
	TargetRevision string `json:"targetRevision"`
} {
	return struct {
		Path           string `json:"path"`
		RepositoryUrl  string `json:"repositoryUrl"`
		TargetRevision string `json:"targetRevision"`
	}{Path: source.Path, RepositoryUrl: source.RepositoryURL, TargetRevision: source.TargetRevision}
}

func assignmentScopeResponses(values []argoapp.AssignmentScope) []contract.ArgoCDCandidateAssignmentScope {
	result := make([]contract.ArgoCDCandidateAssignmentScope, 0, len(values))
	for _, value := range values {
		result = append(result, contract.ArgoCDCandidateAssignmentScope{
			OrganizationId: value.OrganizationID, OrganizationName: value.OrganizationName,
			ProjectId: value.ProjectID, ProjectName: value.ProjectName,
			EnvironmentId: value.EnvironmentID, EnvironmentName: value.EnvironmentName,
		})
	}
	return result
}

func candidateAssignmentBody(c *gin.Context) (contract.ArgoCDCandidateAssignmentRequest, bool) {
	var body contract.ArgoCDCandidateAssignmentRequest
	if err := c.ShouldBindJSON(&body); err != nil || body.ExpectedVersion < 1 {
		respondError(c, http.StatusBadRequest, "INVALID_REQUEST", "Candidate assignment request is invalid")
		return body, false
	}
	return body, true
}

func candidateAssignmentInput(c *gin.Context, candidateID uuid.UUID, body contract.ArgoCDCandidateAssignmentRequest) argoapp.AssignCandidateInput {
	return argoapp.AssignCandidateInput{
		CandidateID: candidateID, ExpectedVersion: uint64(body.ExpectedVersion),
		OrganizationID: body.OrganizationId, ProjectID: body.ProjectId, EnvironmentID: body.EnvironmentId,
		ApplicationName: body.ApplicationName, RequestID: c.GetHeader(requestIDHeader),
		Source: catalog.GitOpsSource{
			RepositoryURL: body.Source.RepositoryUrl, TargetRevision: body.Source.TargetRevision, Path: body.Source.Path,
		},
	}
}

func respondCandidateAssignmentError(c *gin.Context, err error) {
	if errors.Is(err, argoapp.ErrCandidateQueueNotFound) {
		respondError(c, http.StatusNotFound, "CANDIDATE_QUEUE_NOT_FOUND", "Argo CD candidate queue was not found")
		return
	}
	respondError(c, http.StatusConflict, "CANDIDATE_ASSIGNMENT_CONFLICT", "Argo CD candidate assignment was rejected")
}

func onboardingExpectedVersion(c *gin.Context) (uint64, bool) {
	var body contract.OnboardingConfirmRequest
	if err := c.ShouldBindJSON(&body); err != nil || body.ExpectedVersion < 1 {
		respondError(c, http.StatusBadRequest, "INVALID_REQUEST", "Expected onboarding version is required")
		return 0, false
	}
	return uint64(body.ExpectedVersion), true
}

func (h *argoCDHandler) confirmOnboarding(c *gin.Context, user identity.User, expectedVersion uint64, params contract.ConfirmApplicationOnboardingParams) (argodomain.Onboarding, error) {
	applicationID, _ := uuid.Parse(c.Param("applicationId"))
	return h.onboarding.ConfirmIdempotent(
		c.Request.Context(), onboardingPrincipal(user),
		argoapp.OnboardingMutation{RequestID: c.GetHeader(requestIDHeader)},
		applicationID, expectedVersion, string(params.IdempotencyKey),
	)
}

func respondOnboardingConfirmation(c *gin.Context, state argodomain.Onboarding, err error) bool {
	if errors.Is(err, argoapp.ErrCommandInProgress) {
		c.Status(http.StatusAccepted)
		return true
	}
	if err != nil {
		respondError(c, http.StatusConflict, "ONBOARDING_CONFIRM_REJECTED", "Application onboarding confirmation was rejected")
		return true
	}
	return false
}

func onboardingResponse(state argodomain.Onboarding, c *gin.Context) contract.OnboardingResponse {
	var result contract.OnboardingResponse
	result.Data.ApplicationId = state.ApplicationID
	result.Data.Status = string(state.Status)
	result.Data.Version = int64(state.Version)
	result.Meta = responseMeta(c)
	return result
}
