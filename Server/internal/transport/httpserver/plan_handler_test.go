package httpserver_test

import (
	"context"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/google/uuid"

	deployapp "github.com/vincent119/ReleaseHub/Server/internal/deployment/application"
	deploydomain "github.com/vincent119/ReleaseHub/Server/internal/deployment/domain"
	identityapp "github.com/vincent119/ReleaseHub/Server/internal/identity/application"
)

func TestDeploymentPlanRoutesUseInjectedService(t *testing.T) {
	service := &fakePlanDefinitionService{}
	router := newTestRouterWithPlan(t, &fakeAuthFlow{}, service)
	request := workflowMutationRequest(http.MethodPost, "/api/v1/deployment-plans", planRequestBody(uuid.New()))
	response := httptest.NewRecorder()
	router.ServeHTTP(response, request)
	if response.Code != http.StatusCreated || service.created.Name != "Production" {
		t.Fatalf("create deployment plan = %d %s", response.Code, response.Body.String())
	}
	if !strings.Contains(response.Body.String(), `"applicationKey":"api"`) {
		t.Fatalf("deployment plan did not round trip: %s", response.Body.String())
	}
}

func TestDeploymentPlanLifecycleMapsCycleValidation(t *testing.T) {
	service := &fakePlanDefinitionService{changeError: deployapp.ErrPlanInvalid}
	router := newTestRouterWithPlan(t, &fakeAuthFlow{}, service)
	path := "/api/v1/deployment-plans/" + uuid.NewString() + "/versions/" + uuid.NewString() + "/lifecycle"
	request := workflowMutationRequest(http.MethodPost, path, `{"expectedVersion":1,"lifecycle":"Published"}`)
	response := httptest.NewRecorder()
	router.ServeHTTP(response, request)
	if response.Code != http.StatusUnprocessableEntity || !strings.Contains(response.Body.String(), `"code":"DEPLOYMENT_PLAN_INVALID"`) {
		t.Fatalf("deployment plan lifecycle error = %d %s", response.Code, response.Body.String())
	}
}

func TestDeploymentPlanCreateHidesForbiddenAsNotFound(t *testing.T) {
	service := &fakePlanDefinitionService{changeError: deployapp.ErrPlanForbidden}
	router := newTestRouterWithPlan(t, &fakeAuthFlow{}, service)
	request := workflowMutationRequest(http.MethodPost, "/api/v1/deployment-plans", planRequestBody(uuid.New()))
	response := httptest.NewRecorder()
	router.ServeHTTP(response, request)
	if response.Code != http.StatusNotFound || !strings.Contains(response.Body.String(), `"code":"DEPLOYMENT_PLAN_NOT_FOUND"`) {
		t.Fatalf("deployment plan forbidden error = %d %s", response.Code, response.Body.String())
	}
}

func TestDeploymentPlanUnavailableStillRequiresAuthentication(t *testing.T) {
	router := newTestHTTPRouter(t, testAPIOptions(nil))
	response := httptest.NewRecorder()
	router.ServeHTTP(response, httptest.NewRequest(http.MethodGet, "/api/v1/deployment-plans?projectId="+uuid.NewString(), nil))
	if response.Code != http.StatusUnauthorized {
		t.Fatalf("unavailable deployment plan without session = %d %s", response.Code, response.Body.String())
	}
}

func TestDeploymentBindingRoutesUseInjectedService(t *testing.T) {
	binding := &fakeDeploymentBindingService{}
	options := testAPIOptions(&fakeAuthFlow{})
	options.Plan.Definitions = &fakePlanDefinitionService{}
	options.Plan.Bindings = binding
	router := newTestHTTPRouter(t, options)
	organizationID, projectID, environmentID := uuid.New(), uuid.New(), uuid.New()
	path := "/api/v1/deployment-bindings?organizationId=" + organizationID.String() + "&projectId=" + projectID.String() + "&environmentId=" + environmentID.String()
	read := workflowMutationRequest(http.MethodGet, path, "")
	read.Header.Del("X-CSRF-Token")
	readResponse := httptest.NewRecorder()
	router.ServeHTTP(readResponse, read)
	if readResponse.Code != http.StatusOK || !strings.Contains(readResponse.Body.String(), `"data":null`) {
		t.Fatalf("read unbound Environment = %d %s", readResponse.Code, readResponse.Body.String())
	}
	write := workflowMutationRequest(http.MethodPost, "/api/v1/deployment-bindings", bindingRequestBody(organizationID, projectID, environmentID))
	writeResponse := httptest.NewRecorder()
	router.ServeHTTP(writeResponse, write)
	if writeResponse.Code != http.StatusOK || binding.input.EnvironmentID != environmentID {
		t.Fatalf("bind Environment = %d %s", writeResponse.Code, writeResponse.Body.String())
	}
}

func TestDeploymentScheduleRoutesUseInjectedService(t *testing.T) {
	environmentID := uuid.New()
	schedule := &fakeDeploymentScheduleService{view: scheduleHandlerView(t, environmentID)}
	options := testAPIOptions(&fakeAuthFlow{})
	options.Plan.Schedules = schedule
	router := newTestHTTPRouter(t, options)

	read := workflowMutationRequest(http.MethodGet, "/api/v1/deployment-schedules/"+environmentID.String(), "")
	read.Header.Del("X-CSRF-Token")
	readResponse := httptest.NewRecorder()
	router.ServeHTTP(readResponse, read)
	if readResponse.Code != http.StatusOK || !strings.Contains(readResponse.Body.String(), `"canManage":true`) {
		t.Fatalf("read deployment schedule = %d %s", readResponse.Code, readResponse.Body.String())
	}

	write := workflowMutationRequest(http.MethodPut, "/api/v1/deployment-schedules/"+environmentID.String(), `{"enabled":true,"timeZone":"UTC","weeklyWindows":[{"dayOfWeek":1,"startMinute":60,"endMinute":120}],"blackouts":[],"expectedVersion":3}`)
	write.Header.Set("Idempotency-Key", "schedule-1")
	writeResponse := httptest.NewRecorder()
	router.ServeHTTP(writeResponse, write)
	if writeResponse.Code != http.StatusOK || schedule.input.IdempotencyKey != "schedule-1" || schedule.input.ExpectedVersion != 3 {
		t.Fatalf("update deployment schedule = %d %s", writeResponse.Code, writeResponse.Body.String())
	}
}

func TestDeploymentScheduleMapsStableErrors(t *testing.T) {
	tests := []struct {
		name   string
		err    error
		status int
		code   string
	}{
		{name: "not found", err: deployapp.ErrDeploymentScheduleNotFound, status: http.StatusNotFound, code: "DEPLOYMENT_SCHEDULE_NOT_FOUND"},
		{name: "invalid", err: deployapp.ErrDeploymentScheduleInvalid, status: http.StatusBadRequest, code: "DEPLOYMENT_SCHEDULE_INVALID"},
		{name: "conflict", err: deployapp.ErrDeploymentScheduleConflict, status: http.StatusConflict, code: "DEPLOYMENT_SCHEDULE_CONFLICT"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			options := testAPIOptions(&fakeAuthFlow{})
			options.Plan.Schedules = &fakeDeploymentScheduleService{err: tt.err}
			router := newTestHTTPRouter(t, options)
			request := workflowMutationRequest(http.MethodGet, "/api/v1/deployment-schedules/"+uuid.NewString(), "")
			request.Header.Del("X-CSRF-Token")
			response := httptest.NewRecorder()
			router.ServeHTTP(response, request)
			if response.Code != tt.status || !strings.Contains(response.Body.String(), `"code":"`+tt.code+`"`) {
				t.Fatalf("deployment schedule error = %d %s", response.Code, response.Body.String())
			}
		})
	}
}

func TestDeploymentScheduleMutationRejectsInvalidBoundaryRequests(t *testing.T) {
	environmentID := uuid.New()
	validBody := `{"enabled":true,"timeZone":"UTC","weeklyWindows":[{"dayOfWeek":1,"startMinute":60,"endMinute":120}],"blackouts":[],"expectedVersion":0}`
	tests := []struct {
		name       string
		flow       *fakeAuthFlow
		body       string
		mutate     func(*http.Request)
		serviceErr error
		wantStatus int
		wantCode   string
	}{
		{
			name: "missing idempotency key", flow: &fakeAuthFlow{}, body: validBody,
			mutate:     func(request *http.Request) { request.Header.Del("Idempotency-Key") },
			wantStatus: http.StatusBadRequest,
		},
		{
			name: "invalid csrf", flow: &fakeAuthFlow{mutationErr: identityapp.ErrCSRFInvalid}, body: validBody,
			mutate: func(*http.Request) {}, wantStatus: http.StatusForbidden, wantCode: "MUTATION_REJECTED",
		},
		{
			name: "invalid document", flow: &fakeAuthFlow{}, body: `{"enabled":true,"timeZone":"UTC","weeklyWindows":[],"blackouts":[],"expectedVersion":0}`,
			mutate: func(*http.Request) {}, serviceErr: deployapp.ErrDeploymentScheduleInvalid,
			wantStatus: http.StatusBadRequest, wantCode: "DEPLOYMENT_SCHEDULE_INVALID",
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			schedule := &fakeDeploymentScheduleService{view: scheduleHandlerView(t, environmentID), err: tt.serviceErr}
			options := testAPIOptions(tt.flow)
			options.Plan.Schedules = schedule
			router := newTestHTTPRouter(t, options)
			request := workflowMutationRequest(http.MethodPut, "/api/v1/deployment-schedules/"+environmentID.String(), tt.body)
			request.Header.Set("Idempotency-Key", "schedule-boundary")
			tt.mutate(request)
			response := httptest.NewRecorder()

			router.ServeHTTP(response, request)

			if response.Code != tt.wantStatus {
				t.Fatalf("deployment schedule boundary status = %d %s", response.Code, response.Body.String())
			}
			if tt.wantCode != "" && !strings.Contains(response.Body.String(), `"code":"`+tt.wantCode+`"`) {
				t.Fatalf("deployment schedule boundary body = %s, want code %s", response.Body.String(), tt.wantCode)
			}
		})
	}
}

func newTestRouterWithPlan(t *testing.T, flow *fakeAuthFlow, service *fakePlanDefinitionService) http.Handler {
	t.Helper()
	options := testAPIOptions(flow)
	options.Plan.Definitions = service
	return newTestHTTPRouter(t, options)
}

func planRequestBody(projectID uuid.UUID) string {
	return `{
  "ownerKind":"project",
  "ownerProjectId":"` + projectID.String() + `",
  "name":"Production",
  "document":{
    "nodes":[{
      "key":"api",
      "applicationKey":"api",
      "order":0,
      "successCondition":{"syncStatuses":["Synced"],"healthStatuses":["Healthy"],"stabilizationSeconds":30}
    }],
    "edges":[],
    "maxParallel":2
  }
}`
}

type fakePlanDefinitionService struct {
	created     deployapp.CreatePlanInput
	changeError error
}

type fakeDeploymentBindingService struct {
	input deployapp.BindDefinitionsInput
}

type fakeDeploymentScheduleService struct {
	view  deployapp.DeploymentScheduleView
	input deployapp.UpdateDeploymentScheduleInput
	err   error
}

func (s *fakeDeploymentScheduleService) Get(context.Context, deployapp.PlanPrincipal, uuid.UUID) (deployapp.DeploymentScheduleView, error) {
	return s.view, s.err
}

func (s *fakeDeploymentScheduleService) Put(_ context.Context, _ deployapp.PlanPrincipal, input deployapp.UpdateDeploymentScheduleInput) (deployapp.DeploymentScheduleView, error) {
	s.input = input
	return s.view, s.err
}

func scheduleHandlerView(t *testing.T, environmentID uuid.UUID) deployapp.DeploymentScheduleView {
	t.Helper()
	policy, err := deploydomain.NewDeploymentSchedulePolicy(deploydomain.DeploymentSchedulePolicyDraft{
		EnvironmentID: environmentID, Enabled: true, TimeZone: "UTC", Version: 4,
		WeeklyWindows: []deploydomain.DeploymentScheduleWeeklyWindow{{DayOfWeek: time.Monday, StartMinute: 60, EndMinute: 120}},
	})
	if err != nil {
		t.Fatalf("create schedule policy: %v", err)
	}
	return deployapp.DeploymentScheduleView{Policy: policy, CanManage: true}
}

func (*fakeDeploymentBindingService) Get(context.Context, deployapp.PlanPrincipal, deployapp.BindingScopeInput) (*deploydomain.DeploymentBinding, error) {
	return nil, nil
}

func (s *fakeDeploymentBindingService) Bind(_ context.Context, principal deployapp.PlanPrincipal, input deployapp.BindDefinitionsInput) (deploydomain.DeploymentBinding, error) {
	s.input = input
	return deploydomain.NewDeploymentBinding(deploydomain.DeploymentBinding{
		ID: uuid.New(), OrganizationID: input.OrganizationID, ProjectID: input.ProjectID,
		EnvironmentID: input.EnvironmentID, WorkflowVersionID: input.WorkflowVersionID,
		PlanVersionID: input.PlanVersionID, CreatedBy: principal.UserID,
		CreatedAt: time.Date(2026, 9, 7, 11, 12, 13, 0, time.UTC),
	})
}

func bindingRequestBody(organizationID, projectID, environmentID uuid.UUID) string {
	return `{"organizationId":"` + organizationID.String() + `","projectId":"` + projectID.String() +
		`","environmentId":"` + environmentID.String() + `","workflowVersionId":"` + uuid.NewString() +
		`","planVersionId":"` + uuid.NewString() + `","expectedVersion":0}`
}

func (*fakePlanDefinitionService) List(context.Context, deployapp.PlanPrincipal, uuid.UUID) ([]deploydomain.DeploymentPlan, error) {
	return []deploydomain.DeploymentPlan{}, nil
}

func (s *fakePlanDefinitionService) Create(_ context.Context, principal deployapp.PlanPrincipal, input deployapp.CreatePlanInput) (deploydomain.DeploymentPlan, error) {
	s.created = input
	if s.changeError != nil {
		return deploydomain.DeploymentPlan{}, s.changeError
	}
	return deploydomain.NewDeploymentPlan(deploydomain.DeploymentPlan{
		ID: uuid.New(), OwnerKind: input.OwnerKind, OwnerProjectID: input.OwnerProjectID,
		Name: input.Name, Description: input.Description, CreatedBy: principal.UserID,
		CreatedAt: time.Date(2026, 9, 3, 10, 11, 12, 0, time.UTC),
	}, input.Document)
}

func (*fakePlanDefinitionService) CreateVersion(_ context.Context, principal deployapp.PlanPrincipal, input deployapp.CreatePlanVersionInput) (deploydomain.DeploymentPlanVersion, error) {
	return deploydomain.NewDeploymentPlanVersion(deploydomain.DeploymentPlanVersionDraft{
		PlanID: input.PlanID, VersionNumber: input.ExpectedVersion + 1,
		ActorID: principal.UserID, Document: input.Document,
		CreatedAt: time.Date(2026, 9, 3, 10, 11, 12, 0, time.UTC),
	})
}

func (s *fakePlanDefinitionService) ChangeLifecycle(context.Context, deployapp.PlanPrincipal, deployapp.ChangePlanLifecycleInput) (deploydomain.DeploymentPlanVersion, error) {
	return deploydomain.DeploymentPlanVersion{}, s.changeError
}
