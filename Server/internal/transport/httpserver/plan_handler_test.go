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
