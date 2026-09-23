package httpserver

import (
	"net/http"
	"time"

	"github.com/gin-gonic/gin"

	contract "github.com/vincent119/ReleaseHub/Server/internal/transport/openapi"
)

// SystemHandlerOptions configures the system contract handler.
type SystemHandlerOptions struct {
	Version     string
	TenancyMode string
	OIDCEnabled bool
}

// AuthHandlerOptions configures authentication delivery behavior.
type AuthHandlerOptions struct {
	Flow           authFlow
	Local          localAuth
	WebRedirectURL string
	CookieSecure   bool
	LoginStateTTL  time.Duration
	SessionTTL     time.Duration
}

// CatalogHandlerOptions supplies catalog application ports.
type CatalogHandlerOptions struct {
	Service      catalogService
	StatusReader applicationStatusReader
	Runtime      runtimeService
}

// ArgoCDHandlerOptions supplies Argo CD onboarding application ports.
type ArgoCDHandlerOptions struct {
	Onboarding onboardingService
	Candidates candidateService
}

// AccessHandlerOptions supplies authorization management ports.
type AccessHandlerOptions struct {
	Service    accessManagementService
	LocalUsers localUserCreator
}

// WorkflowHandlerOptions supplies release workflow application ports.
type WorkflowHandlerOptions struct {
	Definitions workflowDefinitionService
}

// PlanHandlerOptions supplies Deployment Plan application ports.
type PlanHandlerOptions struct {
	Definitions planDefinitionService
	Bindings    deploymentBindingService
	Schedules   deploymentScheduleService
}

// DeploymentHandlerOptions supplies Deployment Request application ports.
type DeploymentHandlerOptions struct {
	Requests      deploymentRequestService
	Workflows     deploymentWorkflowService
	Executions    deploymentExecutionService
	History       deploymentHistoryService
	Notifications deploymentNotificationService
}

// AuditHandlerOptions supplies Audit Trail query ports.
type AuditHandlerOptions struct {
	Queries auditQueryService
}

// APIOptions groups feature dependencies for the generated OpenAPI contract.
type APIOptions struct {
	System     SystemHandlerOptions
	Auth       AuthHandlerOptions
	Catalog    CatalogHandlerOptions
	ArgoCD     ArgoCDHandlerOptions
	Access     AccessHandlerOptions
	Workflow   WorkflowHandlerOptions
	Plan       PlanHandlerOptions
	Deployment DeploymentHandlerOptions
	Audit      AuditHandlerOptions
}

type apiHandler struct {
	*authHandler
	*catalogHandler
	*argoCDHandler
	*accessHandler
	*workflowHandler
	*planHandler
	*deploymentHandler
	*auditHandler
	*systemHandler
}

type systemHandler struct {
	version     string
	tenancyMode string
	oidcEnabled bool
}

// NewAPIHandler composes bounded-context handlers behind the generated contract.
func NewAPIHandler(options APIOptions) contract.ServerInterface {
	authn := newAuthHandler(options.Auth)
	return &apiHandler{
		authHandler:     authn,
		catalogHandler:  &catalogHandler{authn: authn, service: options.Catalog.Service, statusReader: options.Catalog.StatusReader, runtime: options.Catalog.Runtime},
		argoCDHandler:   &argoCDHandler{authn: authn, onboarding: options.ArgoCD.Onboarding, candidates: options.ArgoCD.Candidates},
		accessHandler:   &accessHandler{authn: authn, service: options.Access.Service, localUsers: options.Access.LocalUsers},
		workflowHandler: &workflowHandler{authn: authn, definitions: options.Workflow.Definitions},
		planHandler:     &planHandler{authn: authn, definitions: options.Plan.Definitions, bindings: options.Plan.Bindings, schedules: options.Plan.Schedules},
		deploymentHandler: &deploymentHandler{authn: authn, requests: options.Deployment.Requests,
			workflows: options.Deployment.Workflows, executions: options.Deployment.Executions,
			history: options.Deployment.History, notifications: options.Deployment.Notifications},
		auditHandler: &auditHandler{authn: authn, queries: options.Audit.Queries},
		systemHandler: &systemHandler{
			version: options.System.Version, tenancyMode: options.System.TenancyMode,
			oidcEnabled: options.System.OIDCEnabled,
		},
	}
}

func newAuthHandler(options AuthHandlerOptions) *authHandler {
	return &authHandler{
		flow: options.Flow, local: options.Local, webRedirectURL: options.WebRedirectURL,
		cookieSecure: options.CookieSecure, loginStateTTL: options.LoginStateTTL,
		sessionTTL: options.SessionTTL,
	}
}

func (h *systemHandler) GetSystemStatus(c *gin.Context) {
	c.JSON(http.StatusOK, contract.SystemStatusResponse{
		Data: contract.SystemStatus{
			Name: "ReleaseHub", Version: h.version, OidcEnabled: h.oidcEnabled,
			TenancyMode: contract.SystemStatusTenancyMode(h.normalizedTenancyMode()),
		},
		Meta: responseMeta(c),
	})
}

func (h *systemHandler) normalizedTenancyMode() string {
	if h.tenancyMode == "" {
		return "single"
	}
	return h.tenancyMode
}
