package httpserver_test

import (
	"context"
	"errors"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/prometheus/client_golang/prometheus"
	"go.opentelemetry.io/otel/sdk/trace"
	"go.uber.org/zap"
	"go.uber.org/zap/zaptest/observer"

	deployapp "github.com/vincent119/ReleaseHub/Server/internal/deployment/application"
	deploydomain "github.com/vincent119/ReleaseHub/Server/internal/deployment/domain"
	"github.com/vincent119/ReleaseHub/Server/internal/observability"
	"github.com/vincent119/ReleaseHub/Server/internal/transport/httpserver"
)

func TestReleaseWorkflowRoutesUseInjectedService(t *testing.T) {
	service := &fakeWorkflowDefinitionService{}
	router := newTestRouterWithWorkflow(t, &fakeAuthFlow{}, service)
	request := workflowMutationRequest(http.MethodPost, "/api/v1/release-workflows", workflowRequestBody())
	response := httptest.NewRecorder()
	router.ServeHTTP(response, request)
	if response.Code != http.StatusCreated || service.created.Name != "Production approval" {
		t.Fatalf("create workflow = %d %s", response.Code, response.Body.String())
	}
	if !strings.Contains(response.Body.String(), `"initialState":"start"`) {
		t.Fatalf("workflow response did not round trip: %s", response.Body.String())
	}
}

func TestReleaseWorkflowLifecycleMapsDomainValidation(t *testing.T) {
	service := &fakeWorkflowDefinitionService{changeError: deployapp.ErrWorkflowInvalid}
	router := newTestRouterWithWorkflow(t, &fakeAuthFlow{}, service)
	path := "/api/v1/release-workflows/" + uuid.NewString() + "/versions/" + uuid.NewString() + "/lifecycle"
	request := workflowMutationRequest(http.MethodPost, path, `{"expectedVersion":1,"lifecycle":"Published"}`)
	response := httptest.NewRecorder()
	router.ServeHTTP(response, request)
	if response.Code != http.StatusUnprocessableEntity || !strings.Contains(response.Body.String(), `"code":"WORKFLOW_INVALID"`) {
		t.Fatalf("lifecycle error = %d %s", response.Code, response.Body.String())
	}
}

func TestReleaseWorkflowUnavailableStillRequiresAuthentication(t *testing.T) {
	router := newTestHTTPRouter(t, testAPIOptions(nil))
	response := httptest.NewRecorder()
	router.ServeHTTP(response, httptest.NewRequest(http.MethodGet, "/api/v1/release-workflows", nil))
	if response.Code != http.StatusUnauthorized {
		t.Fatalf("unavailable workflow without session = %d %s", response.Code, response.Body.String())
	}
}

func TestReleaseWorkflowUnexpectedMutationFailureIsInternalError(t *testing.T) {
	service := &fakeWorkflowDefinitionService{createError: errors.New("database unavailable")}
	router := newTestRouterWithWorkflow(t, &fakeAuthFlow{}, service)
	request := workflowMutationRequest(http.MethodPost, "/api/v1/release-workflows", workflowRequestBody())
	response := httptest.NewRecorder()
	router.ServeHTTP(response, request)
	if response.Code != http.StatusInternalServerError || !strings.Contains(response.Body.String(), `"code":"WORKFLOW_MUTATION_FAILED"`) {
		t.Fatalf("unexpected workflow failure = %d %s", response.Code, response.Body.String())
	}
}

func newTestRouterWithWorkflow(t *testing.T, flow *fakeAuthFlow, service *fakeWorkflowDefinitionService) http.Handler {
	t.Helper()
	options := testAPIOptions(flow)
	options.Workflow.Definitions = service
	return newTestHTTPRouter(t, options)
}

func newTestHTTPRouter(t *testing.T, options httpserver.APIOptions) http.Handler {
	t.Helper()
	core, _ := observer.New(zap.InfoLevel)
	logger, err := observability.WithComponent(zap.New(core), observability.ComponentAPI)
	if err != nil {
		t.Fatalf("create test logger: %v", err)
	}
	registry := prometheus.NewRegistry()
	metrics, err := observability.NewHTTPMetrics(registry)
	if err != nil {
		t.Fatalf("create test metrics: %v", err)
	}
	provider := trace.NewTracerProvider()
	t.Cleanup(func() { _ = provider.Shutdown(t.Context()) })
	return httpserver.NewRouter(httpserver.RouterOptions{
		Logger: logger, Registry: registry, HTTPMetrics: metrics,
		TracerProvider: provider, Readiness: httpserver.NewReadiness(), MetricsPath: "/metrics",
	}, httpserver.NewAPIHandler(options))
}

func workflowMutationRequest(method, path, body string) *http.Request {
	request := httptest.NewRequest(method, path, strings.NewReader(body))
	request.AddCookie(&http.Cookie{Name: "releasehub_session", Value: "session-token"})
	request.Header.Set("Content-Type", "application/json")
	request.Header.Set("Origin", "https://releasehub.example")
	request.Header.Set("X-CSRF-Token", "csrf-token")
	return request
}

func workflowRequestBody() string {
	return `{
  "name":"Production approval",
  "document":{
    "initialState":"start",
    "states":[
      {"key":"start","name":"Start","type":"Start"},
      {"key":"done","name":"Done","type":"Terminal"}
    ],
    "transitions":[
      {"key":"finish","from":"start","to":"done","trigger":"Manual","permission":"deployment_request.update","conditions":[]}
    ]
  }
}`
}

type fakeWorkflowDefinitionService struct {
	created     deployapp.CreateWorkflowInput
	createError error
	changeError error
}

func (s *fakeWorkflowDefinitionService) List(context.Context, deployapp.WorkflowPrincipal) ([]deploydomain.ReleaseWorkflow, error) {
	return []deploydomain.ReleaseWorkflow{}, nil
}

func (s *fakeWorkflowDefinitionService) Create(_ context.Context, principal deployapp.WorkflowPrincipal, input deployapp.CreateWorkflowInput) (deploydomain.ReleaseWorkflow, error) {
	s.created = input
	if s.createError != nil {
		return deploydomain.ReleaseWorkflow{}, s.createError
	}
	return deploydomain.NewReleaseWorkflow(deploydomain.ReleaseWorkflow{
		ID: uuid.New(), Name: input.Name, Description: input.Description,
		CreatedBy: principal.UserID, CreatedAt: workflowHandlerTestTime(),
	}, input.Document)
}

func (*fakeWorkflowDefinitionService) CreateVersion(_ context.Context, _ deployapp.WorkflowPrincipal, input deployapp.CreateWorkflowVersionInput) (deploydomain.ReleaseWorkflowVersion, error) {
	return deploydomain.NewReleaseWorkflowVersion(deploydomain.WorkflowVersionDraft{
		WorkflowID: input.WorkflowID, VersionNumber: input.ExpectedVersion + 1,
		ActorID: uuid.New(), Document: input.Document, CreatedAt: workflowHandlerTestTime(),
	})
}

func (s *fakeWorkflowDefinitionService) ChangeLifecycle(_ context.Context, _ deployapp.WorkflowPrincipal, _ deployapp.ChangeWorkflowLifecycleInput) (deploydomain.ReleaseWorkflowVersion, error) {
	return deploydomain.ReleaseWorkflowVersion{}, s.changeError
}

func workflowHandlerTestTime() time.Time {
	return time.Date(2026, 9, 3, 3, 4, 5, 0, time.UTC)
}
