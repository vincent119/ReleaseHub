package httpserver

import (
	"context"
	"errors"
	"net/http"

	"github.com/gin-gonic/gin"
	"github.com/google/uuid"

	argoinfra "github.com/vincent119/ReleaseHub/Server/internal/argocd/infrastructure"
	catalogapp "github.com/vincent119/ReleaseHub/Server/internal/catalog/application"
	catalog "github.com/vincent119/ReleaseHub/Server/internal/catalog/domain"
	contract "github.com/vincent119/ReleaseHub/Server/internal/transport/openapi"
)

type applicationStatusReader interface {
	Load(context.Context, uuid.UUID) (argoinfra.ApplicationStatus, error)
}

type catalogService interface {
	CreateOrganization(context.Context, catalogapp.Principal, catalogapp.Mutation, string) (catalog.Organization, error)
	CreateProject(context.Context, catalogapp.Principal, catalogapp.Mutation, uuid.UUID, string) (catalog.Project, error)
	CreateEnvironment(context.Context, catalogapp.Principal, catalogapp.Mutation, uuid.UUID, uuid.UUID, string, catalog.EnvironmentType) (catalog.Environment, error)
	CreateEnvironmentLabelMapping(context.Context, catalogapp.Principal, catalogapp.Mutation, uuid.UUID, uuid.UUID, uuid.UUID, string, string) (catalog.EnvironmentLabelMapping, error)
	FindApplication(context.Context, catalogapp.Principal, uuid.UUID) (catalog.Application, error)
	ListResourceTree(context.Context, catalogapp.Principal) (catalogapp.ResourceTree, error)
	ListVisibleApplications(context.Context, catalogapp.Principal) ([]catalog.Application, error)
	ListApplications(context.Context, catalogapp.Principal, uuid.UUID, uuid.UUID, uuid.UUID) ([]catalog.Application, error)
}

type catalogHandler struct {
	authn        *authHandler
	service      catalogService
	statusReader applicationStatusReader
}

// CreateCatalogOrganization creates a tenant boundary after CSRF and platform authorization checks.
func (h *catalogHandler) CreateCatalogOrganization(c *gin.Context, params contract.CreateCatalogOrganizationParams) {
	_, user, ok := h.authn.authenticateMutation(c, string(params.XCSRFToken))
	if !ok {
		return
	}
	var body contract.CreateNamedResourceRequest
	if err := c.ShouldBindJSON(&body); err != nil {
		respondError(c, http.StatusBadRequest, "INVALID_REQUEST", "Organization request is invalid")
		return
	}
	value, err := h.service.CreateOrganization(c.Request.Context(), catalogapp.Principal{UserID: user.ID, Disabled: user.Disabled}, catalogapp.Mutation{RequestID: c.GetHeader(requestIDHeader)}, body.Name)
	if !h.respondCatalogMutationError(c, err) {
		return
	}
	c.JSON(http.StatusCreated, contract.CatalogOrganizationResponse{Data: contract.CatalogOrganizationResource{Id: value.ID, Name: value.Name, Active: value.Active, Version: int64(value.Version)}, Meta: responseMeta(c)})
}

// CreateCatalogProject creates a Project under an existing Organization.
func (h *catalogHandler) CreateCatalogProject(c *gin.Context, organizationID uuid.UUID, params contract.CreateCatalogProjectParams) {
	_, user, ok := h.authn.authenticateMutation(c, string(params.XCSRFToken))
	if !ok {
		return
	}
	var body contract.CreateNamedResourceRequest
	if err := c.ShouldBindJSON(&body); err != nil {
		respondError(c, http.StatusBadRequest, "INVALID_REQUEST", "Project request is invalid")
		return
	}
	value, err := h.service.CreateProject(c.Request.Context(), catalogapp.Principal{UserID: user.ID, Disabled: user.Disabled}, catalogapp.Mutation{RequestID: c.GetHeader(requestIDHeader)}, organizationID, body.Name)
	if !h.respondCatalogMutationError(c, err) {
		return
	}
	c.JSON(http.StatusCreated, contract.CatalogProjectResponse{Data: contract.CatalogProjectResource{Id: value.ID, OrganizationId: value.OrganizationID, Name: value.Name, Active: value.Active, Version: int64(value.Version)}, Meta: responseMeta(c)})
}

// CreateCatalogEnvironment creates a custom-named Environment with a fixed type.
func (h *catalogHandler) CreateCatalogEnvironment(c *gin.Context, organizationID, projectID uuid.UUID, params contract.CreateCatalogEnvironmentParams) {
	_, user, ok := h.authn.authenticateMutation(c, string(params.XCSRFToken))
	if !ok {
		return
	}
	var body contract.CreateEnvironmentRequest
	if err := c.ShouldBindJSON(&body); err != nil {
		respondError(c, http.StatusBadRequest, "INVALID_REQUEST", "Environment request is invalid")
		return
	}
	value, err := h.service.CreateEnvironment(c.Request.Context(), catalogapp.Principal{UserID: user.ID, Disabled: user.Disabled}, catalogapp.Mutation{RequestID: c.GetHeader(requestIDHeader)}, organizationID, projectID, body.Name, catalog.EnvironmentType(body.Type))
	if !h.respondCatalogMutationError(c, err) {
		return
	}
	c.JSON(http.StatusCreated, contract.CatalogEnvironmentResponse{Data: contract.CatalogEnvironmentResource{Id: value.ID, OrganizationId: value.OrganizationID, ProjectId: value.ProjectID, Name: value.Name, Type: contract.CatalogEnvironmentResourceType(value.Type), Active: value.Active, Version: int64(value.Version)}, Meta: responseMeta(c)})
}

// CreateCatalogEnvironmentLabelMapping maps existing Argo CD metadata to one Environment.
func (h *catalogHandler) CreateCatalogEnvironmentLabelMapping(c *gin.Context, organizationID, projectID uuid.UUID, params contract.CreateCatalogEnvironmentLabelMappingParams) {
	_, user, ok := h.authn.authenticateMutation(c, string(params.XCSRFToken))
	if !ok {
		return
	}
	var body contract.CreateEnvironmentLabelMappingRequest
	if err := c.ShouldBindJSON(&body); err != nil {
		respondError(c, http.StatusBadRequest, "INVALID_REQUEST", "Environment label mapping request is invalid")
		return
	}
	value, err := h.service.CreateEnvironmentLabelMapping(c.Request.Context(), catalogapp.Principal{UserID: user.ID, Disabled: user.Disabled}, catalogapp.Mutation{RequestID: c.GetHeader(requestIDHeader)}, organizationID, projectID, body.EnvironmentId, body.LabelKey, body.LabelValue)
	if !h.respondCatalogMutationError(c, err) {
		return
	}
	c.JSON(http.StatusCreated, contract.CatalogEnvironmentLabelMappingResponse{Data: contract.CatalogEnvironmentLabelMappingResource{Id: value.ID, OrganizationId: value.OrganizationID, ProjectId: value.ProjectID, EnvironmentId: value.EnvironmentID, LabelKey: value.LabelKey, LabelValue: value.LabelValue}, Meta: responseMeta(c)})
}

func (h *catalogHandler) respondCatalogMutationError(c *gin.Context, err error) bool {
	if err == nil {
		return true
	}
	if errors.Is(err, catalogapp.ErrResourceNotFound) {
		respondError(c, http.StatusNotFound, "RESOURCE_NOT_FOUND", "Resource was not found")
		return false
	}
	respondError(c, http.StatusConflict, "RESOURCE_MUTATION_REJECTED", "Resource change was rejected")
	return false
}

// GetCatalogResourceTree returns only resource ancestors visible to the current principal.
func (h *catalogHandler) GetCatalogResourceTree(c *gin.Context) {
	_, user, ok := h.authn.authenticate(c)
	if !ok {
		return
	}
	if h.service == nil {
		respondError(c, http.StatusServiceUnavailable, "CATALOG_UNAVAILABLE", "Resource catalog is unavailable")
		return
	}
	tree, err := h.service.ListResourceTree(c.Request.Context(), catalogapp.Principal{UserID: user.ID, Disabled: user.Disabled})
	if err != nil {
		respondError(c, http.StatusInternalServerError, "CATALOG_READ_FAILED", "Unable to read resource catalog")
		return
	}
	c.JSON(http.StatusOK, contract.CatalogResourceTreeResponse{Data: organizationNodes(tree.Organizations), CanCreateOrganization: tree.CanCreateOrganization, Meta: responseMeta(c)})
}

func organizationNodes(values []catalogapp.OrganizationNode) []contract.CatalogOrganizationNode {
	result := make([]contract.CatalogOrganizationNode, 0, len(values))
	for _, value := range values {
		result = append(result, contract.CatalogOrganizationNode{
			Id: value.Organization.ID, Name: value.Organization.Name,
			CanCreateProject: value.CanCreateProject, Projects: projectNodes(value.Projects),
		})
	}
	return result
}

func projectNodes(values []catalogapp.ProjectNode) []contract.CatalogProjectNode {
	result := make([]contract.CatalogProjectNode, 0, len(values))
	for _, value := range values {
		result = append(result, contract.CatalogProjectNode{
			Id: value.Project.ID, Name: value.Project.Name,
			CanManage: value.CanManage, Environments: environmentNodes(value.Environments),
		})
	}
	return result
}

func environmentNodes(values []catalogapp.EnvironmentNode) []contract.CatalogEnvironmentNode {
	result := make([]contract.CatalogEnvironmentNode, 0, len(values))
	for _, value := range values {
		result = append(result, contract.CatalogEnvironmentNode{
			Id: value.Environment.ID, Name: value.Environment.Name,
			Type:         contract.CatalogEnvironmentNodeType(value.Environment.Type),
			Applications: catalogApplicationsResponse(value.Applications),
		})
	}
	return result
}

func (h *catalogHandler) ListCatalogApplications(c *gin.Context, params contract.ListCatalogApplicationsParams) {
	_, user, ok := h.authn.authenticate(c)
	if !ok {
		return
	}
	if h.service == nil {
		respondError(c, http.StatusServiceUnavailable, "CATALOG_UNAVAILABLE", "Catalog is unavailable")
		return
	}
	applications, err := h.service.ListApplications(c.Request.Context(), catalogapp.Principal{UserID: user.ID, Disabled: user.Disabled}, params.OrganizationId, params.ProjectId, params.EnvironmentId)
	if err != nil {
		respondError(c, http.StatusInternalServerError, "CATALOG_READ_FAILED", "Unable to read catalog Applications")
		return
	}
	respondCatalogApplications(c, applications)
}

func (h *catalogHandler) ListVisibleCatalogApplications(c *gin.Context) {
	_, user, ok := h.authn.authenticate(c)
	if !ok {
		return
	}
	if h.service == nil {
		respondError(c, http.StatusServiceUnavailable, "CATALOG_UNAVAILABLE", "Catalog is unavailable")
		return
	}
	applications, err := h.service.ListVisibleApplications(c.Request.Context(), catalogapp.Principal{UserID: user.ID, Disabled: user.Disabled})
	if err != nil {
		respondError(c, http.StatusInternalServerError, "CATALOG_READ_FAILED", "Unable to read visible catalog Applications")
		return
	}
	respondCatalogApplications(c, applications)
}

func respondCatalogApplications(c *gin.Context, values []catalog.Application) {
	c.JSON(http.StatusOK, contract.CatalogApplicationListResponse{Data: catalogApplicationsResponse(values), Meta: responseMeta(c)})
}

func catalogApplicationsResponse(values []catalog.Application) []contract.CatalogApplication {
	result := make([]contract.CatalogApplication, 0, len(values))
	for _, value := range values {
		result = append(result, catalogApplicationResponse(value))
	}
	return result
}

func (h *catalogHandler) GetCatalogApplication(c *gin.Context, applicationID contract.ApplicationId) {
	_, user, ok := h.authn.authenticate(c)
	if !ok {
		return
	}
	if h.service == nil {
		respondError(c, http.StatusServiceUnavailable, "CATALOG_UNAVAILABLE", "Catalog is unavailable")
		return
	}
	application, err := h.service.FindApplication(c.Request.Context(), catalogapp.Principal{UserID: user.ID, Disabled: user.Disabled}, applicationID)
	if err != nil {
		respondError(c, http.StatusNotFound, "CATALOG_APPLICATION_NOT_FOUND", "Catalog Application was not found")
		return
	}
	c.JSON(http.StatusOK, contract.CatalogApplicationResponse{Data: catalogApplicationResponse(application), Meta: responseMeta(c)})
}

func (h *catalogHandler) GetCatalogApplicationStatus(c *gin.Context, applicationID contract.ApplicationId) {
	_, user, ok := h.authn.authenticate(c)
	if !ok {
		return
	}
	if h.service == nil || h.statusReader == nil {
		respondError(c, http.StatusServiceUnavailable, "APPLICATION_STATUS_UNAVAILABLE", "Application status is unavailable")
		return
	}
	if _, err := h.service.FindApplication(c.Request.Context(), catalogapp.Principal{UserID: user.ID, Disabled: user.Disabled}, applicationID); err != nil {
		respondError(c, http.StatusNotFound, "CATALOG_APPLICATION_NOT_FOUND", "Catalog Application was not found")
		return
	}
	status, err := h.statusReader.Load(c.Request.Context(), applicationID)
	if err != nil {
		respondError(c, http.StatusNotFound, "CATALOG_APPLICATION_NOT_FOUND", "Catalog Application was not found")
		return
	}
	c.JSON(http.StatusOK, contract.CatalogApplicationStatusResponse{Data: catalogApplicationStatusResponse(status), Meta: responseMeta(c)})
}

func catalogApplicationStatusResponse(status argoinfra.ApplicationStatus) contract.CatalogApplicationStatus {
	return contract.CatalogApplicationStatus{
		CandidatePresent: status.CandidatePresent, AutomatedSync: status.AutomatedSync,
		SyncStatus: status.SyncStatus, HealthStatus: status.HealthStatus,
		OperationPhase: status.OperationPhase, ResolvedRevision: status.ResolvedRevision,
		OnboardingStatus: status.OnboardingStatus, OnboardingVersion: int64(status.OnboardingVersion),
		DriftReasons: status.DriftReasons,
	}
}

func catalogApplicationResponse(application catalog.Application) contract.CatalogApplication {
	return contract.CatalogApplication{
		Id: application.ID, OrganizationId: application.OrganizationID, ProjectId: application.ProjectID, EnvironmentId: application.EnvironmentID,
		Name: application.Name, ArgocdNamespace: application.Argo.Namespace, ArgocdApplicationName: application.Argo.Name,
		ArgocdProject: application.ArgoProject, DestinationServer: application.DestinationServer, DestinationNamespace: application.DestinationNamespace,
		SourceRepositoryUrl: application.Source.RepositoryURL, SourceTargetRevision: application.Source.TargetRevision, SourcePath: application.Source.Path,
		Active: application.Active, Version: int64(application.Version),
	}
}
