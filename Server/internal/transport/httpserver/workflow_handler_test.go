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
	identityapp "github.com/vincent119/ReleaseHub/Server/internal/identity/application"
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

func TestReleaseWorkflowReviewOptionsUseInjectedService(t *testing.T) {
	userID, roleID := uuid.New(), uuid.New()
	service := &fakeWorkflowDefinitionService{reviewOptions: deployapp.WorkflowReviewOptions{
		Users: []deployapp.WorkflowReviewUserOption{{ID: userID, Username: "reviewer", Assignable: true}},
		Roles: []deployapp.WorkflowReviewRoleOption{{ID: roleID, Name: "release_approver", OwnerKind: "platform", Assignable: true}},
	}}
	router := newTestRouterWithWorkflow(t, &fakeAuthFlow{}, service)
	request := workflowMutationRequest(http.MethodGet, "/api/v1/release-workflows/review-options", "")
	response := httptest.NewRecorder()
	router.ServeHTTP(response, request)
	if response.Code != http.StatusOK || !strings.Contains(response.Body.String(), `"username":"reviewer"`) || !strings.Contains(response.Body.String(), `"name":"release_approver"`) {
		t.Fatalf("review options = %d %s", response.Code, response.Body.String())
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

func TestCreateReleaseWorkflowReturnsNamedConflict(t *testing.T) {
	service := &fakeWorkflowDefinitionService{createError: deployapp.ErrWorkflowNameConflict}
	router := newTestRouterWithWorkflow(t, &fakeAuthFlow{}, service)
	request := workflowMutationRequest(http.MethodPost, "/api/v1/release-workflows", workflowRequestBody())
	response := httptest.NewRecorder()

	router.ServeHTTP(response, request)

	if response.Code != http.StatusConflict || !strings.Contains(response.Body.String(), `"code":"WORKFLOW_NAME_CONFLICT"`) {
		t.Fatalf("workflow name conflict = %d %s", response.Code, response.Body.String())
	}
}

func TestCreateReleaseWorkflowVersionReturnsSpecificConflicts(t *testing.T) {
	tests := []struct {
		name string
		err  error
		code string
	}{
		{name: "stale version", err: deployapp.ErrWorkflowVersionConflict, code: "WORKFLOW_VERSION_CONFLICT"},
		{name: "existing draft", err: deployapp.ErrWorkflowDraftExists, code: "WORKFLOW_DRAFT_EXISTS"},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			service := &fakeWorkflowDefinitionService{versionError: test.err}
			router := newTestRouterWithWorkflow(t, &fakeAuthFlow{}, service)
			path := "/api/v1/release-workflows/" + uuid.NewString() + "/versions"
			request := workflowMutationRequest(http.MethodPost, path, `{"expectedVersion":1,"document":{"initialState":"start","states":[{"key":"start","name":"Start","type":"Start"},{"key":"done","name":"Done","type":"Terminal"}],"transitions":[{"key":"finish","from":"start","to":"done","trigger":"Manual","permission":"deployment_request.update","conditions":[]}]}}`)
			response := httptest.NewRecorder()

			router.ServeHTTP(response, request)

			if response.Code != http.StatusConflict || !strings.Contains(response.Body.String(), `"code":"`+test.code+`"`) {
				t.Fatalf("workflow version conflict = %d %s", response.Code, response.Body.String())
			}
		})
	}
}

func TestReleaseWorkflowReadFailureKeepsResponseGenericAndLogsCause(t *testing.T) {
	service := &fakeWorkflowDefinitionService{listError: errors.New("database unavailable")}
	options := testAPIOptions(&fakeAuthFlow{})
	options.Workflow.Definitions = service
	router, recorded, _, _, _ := newTestRouterFromOptionsWithState(t, options)
	request := workflowMutationRequest(http.MethodGet, "/api/v1/release-workflows", "")
	request.Header.Set("Cookie", "releasehub_session=session-secret")
	response := httptest.NewRecorder()

	router.ServeHTTP(response, request)

	if response.Code != http.StatusInternalServerError || !strings.Contains(response.Body.String(), `"code":"WORKFLOW_READ_FAILED"`) || strings.Contains(response.Body.String(), "database unavailable") {
		t.Fatalf("workflow read failure = %d %s", response.Code, response.Body.String())
	}
	entry := recorded.FilterMessage("HTTP request failed").All()[0]
	loggedError, _ := entry.ContextMap()["error"].(string)
	if !strings.Contains(loggedError, "list release workflows: database unavailable") || strings.Contains(loggedError, "session-secret") {
		t.Fatalf("logged error = %q", loggedError)
	}
}

func TestReleaseWorkflowMutationAuthenticationErrorMapping(t *testing.T) {
	tests := []struct {
		name             string
		err              error
		wantStatus       int
		wantCode         string
		wantClearedCount int
	}{
		{name: "invalid session", err: identityapp.ErrSessionInvalid, wantStatus: http.StatusUnauthorized, wantCode: "SESSION_INVALID", wantClearedCount: 2},
		{name: "invalid CSRF", err: identityapp.ErrCSRFInvalid, wantStatus: http.StatusForbidden, wantCode: "MUTATION_REJECTED"},
		{name: "infrastructure failure", err: errors.New("database unavailable"), wantStatus: http.StatusServiceUnavailable, wantCode: "SESSION_STATE_UNAVAILABLE"},
		{name: "deadline exceeded", err: context.DeadlineExceeded, wantStatus: http.StatusServiceUnavailable, wantCode: "SESSION_STATE_UNAVAILABLE"},
		{name: "client canceled request", err: context.Canceled, wantStatus: 499},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			router := newTestRouterWithWorkflow(t, &fakeAuthFlow{mutationErr: test.err}, &fakeWorkflowDefinitionService{})
			response := httptest.NewRecorder()
			router.ServeHTTP(response, workflowMutationRequest(http.MethodPost, "/api/v1/release-workflows", workflowRequestBody()))
			if response.Code != test.wantStatus {
				t.Fatalf("mutation authentication = %d %s", response.Code, response.Body.String())
			}
			if test.wantCode == "" {
				if response.Body.Len() != 0 {
					t.Fatalf("canceled mutation body = %q, want empty", response.Body.String())
				}
			} else if !strings.Contains(response.Body.String(), `"code":"`+test.wantCode+`"`) {
				t.Fatalf("mutation authentication body = %s, want code %s", response.Body.String(), test.wantCode)
			}
			clearedCount := 0
			for _, cookie := range response.Result().Cookies() {
				if cookie.MaxAge == -1 {
					clearedCount++
				}
			}
			if clearedCount != test.wantClearedCount {
				t.Fatalf("cleared cookies = %d, want %d", clearedCount, test.wantClearedCount)
			}
		})
	}
}

func TestDeleteReleaseWorkflowReturnsNoContent(t *testing.T) {
	workflowID := uuid.New()
	service := &fakeWorkflowDefinitionService{}
	router := newTestRouterWithWorkflow(t, &fakeAuthFlow{}, service)
	request := workflowMutationRequest(http.MethodDelete, "/api/v1/release-workflows/"+workflowID.String()+"?expectedVersion=2", "")
	response := httptest.NewRecorder()
	router.ServeHTTP(response, request)
	if response.Code != http.StatusNoContent {
		t.Fatalf("delete workflow = %d %s", response.Code, response.Body.String())
	}
	if service.deleted.WorkflowID != workflowID || service.deleted.ExpectedVersion != 2 {
		t.Fatalf("delete input = %#v", service.deleted)
	}
}

func TestDeleteReleaseWorkflowMapsConflictAndInvalidQuery(t *testing.T) {
	workflowID := uuid.NewString()
	service := &fakeWorkflowDefinitionService{deleteError: deployapp.ErrWorkflowConflict}
	router := newTestRouterWithWorkflow(t, &fakeAuthFlow{}, service)

	conflict := httptest.NewRecorder()
	router.ServeHTTP(conflict, workflowMutationRequest(http.MethodDelete, "/api/v1/release-workflows/"+workflowID+"?expectedVersion=1", ""))
	if conflict.Code != http.StatusConflict || !strings.Contains(conflict.Body.String(), `"code":"WORKFLOW_CONFLICT"`) {
		t.Fatalf("delete conflict = %d %s", conflict.Code, conflict.Body.String())
	}

	invalid := httptest.NewRecorder()
	router.ServeHTTP(invalid, workflowMutationRequest(http.MethodDelete, "/api/v1/release-workflows/"+workflowID+"?expectedVersion=0", ""))
	if invalid.Code != http.StatusBadRequest {
		t.Fatalf("invalid expected version = %d %s", invalid.Code, invalid.Body.String())
	}
}

func TestDeleteReleaseWorkflowPreservesAuthenticationAndHiddenAuthorization(t *testing.T) {
	workflowID := uuid.NewString()
	service := &fakeWorkflowDefinitionService{deleteError: deployapp.ErrWorkflowForbidden}
	router := newTestRouterWithWorkflow(t, &fakeAuthFlow{}, service)

	forbidden := httptest.NewRecorder()
	router.ServeHTTP(forbidden, workflowMutationRequest(http.MethodDelete, "/api/v1/release-workflows/"+workflowID+"?expectedVersion=1", ""))
	if forbidden.Code != http.StatusNotFound || !strings.Contains(forbidden.Body.String(), `"code":"WORKFLOW_NOT_FOUND"`) {
		t.Fatalf("hidden delete authorization = %d %s", forbidden.Code, forbidden.Body.String())
	}

	unauthenticatedRequest := httptest.NewRequest(http.MethodDelete, "/api/v1/release-workflows/"+workflowID+"?expectedVersion=1", nil)
	unauthenticatedRequest.Header.Set("X-CSRF-Token", "csrf-token")
	unauthenticatedRequest.Header.Set("Origin", "https://releasehub.example")
	unauthenticated := httptest.NewRecorder()
	router.ServeHTTP(unauthenticated, unauthenticatedRequest)
	if unauthenticated.Code != http.StatusUnauthorized {
		t.Fatalf("unauthenticated delete = %d %s", unauthenticated.Code, unauthenticated.Body.String())
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
	created       deployapp.CreateWorkflowInput
	deleted       deployapp.DeleteWorkflowInput
	listError     error
	createError   error
	versionError  error
	changeError   error
	deleteError   error
	reviewOptions deployapp.WorkflowReviewOptions
}

func (s *fakeWorkflowDefinitionService) List(context.Context, deployapp.WorkflowPrincipal) ([]deploydomain.ReleaseWorkflow, error) {
	return []deploydomain.ReleaseWorkflow{}, s.listError
}

func (s *fakeWorkflowDefinitionService) ReviewOptions(context.Context, deployapp.WorkflowPrincipal) (deployapp.WorkflowReviewOptions, error) {
	return s.reviewOptions, nil
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

func (s *fakeWorkflowDefinitionService) CreateVersion(_ context.Context, _ deployapp.WorkflowPrincipal, input deployapp.CreateWorkflowVersionInput) (deploydomain.ReleaseWorkflowVersion, error) {
	if s.versionError != nil {
		return deploydomain.ReleaseWorkflowVersion{}, s.versionError
	}
	return deploydomain.NewReleaseWorkflowVersion(deploydomain.WorkflowVersionDraft{
		WorkflowID: input.WorkflowID, VersionNumber: input.ExpectedVersion + 1,
		ActorID: uuid.New(), Document: input.Document, CreatedAt: workflowHandlerTestTime(),
	})
}

func (s *fakeWorkflowDefinitionService) ChangeLifecycle(_ context.Context, _ deployapp.WorkflowPrincipal, _ deployapp.ChangeWorkflowLifecycleInput) (deploydomain.ReleaseWorkflowVersion, error) {
	return deploydomain.ReleaseWorkflowVersion{}, s.changeError
}

func (s *fakeWorkflowDefinitionService) Delete(_ context.Context, _ deployapp.WorkflowPrincipal, input deployapp.DeleteWorkflowInput) error {
	s.deleted = input
	return s.deleteError
}

func workflowHandlerTestTime() time.Time {
	return time.Date(2026, 9, 3, 3, 4, 5, 0, time.UTC)
}
