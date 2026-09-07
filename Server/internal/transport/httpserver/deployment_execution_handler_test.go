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
)

func TestDeploymentExecutionRoutesUseInjectedService(t *testing.T) {
	service := &fakeDeploymentExecutionService{value: executionFixture()}
	options := testAPIOptions(&fakeAuthFlow{})
	options.Deployment.Executions = service
	router := newTestHTTPRouter(t, options)
	requestID, versionID := uuid.New(), service.value.RequestVersionID

	get := workflowMutationRequest(http.MethodGet, "/api/v1/deployment-executions/"+service.value.ID.String(), "")
	get.Header.Del("X-CSRF-Token")
	assertExecutionResponse(t, router, get, http.StatusOK, `"status":"PartialFailed"`)

	retry := executionMutationRequest(http.MethodPost, executionCommandPath(requestID, versionID, "retry"), `{"applicationIds":["`+uuid.NewString()+`"],"expectedVersion":3}`)
	assertExecutionResponse(t, router, retry, http.StatusAccepted, `"attempt":2`)
	terminate := executionMutationRequest(http.MethodPost, executionCommandPath(requestID, versionID, "terminate"), `{"reason":"stop","expectedVersion":3}`)
	assertExecutionResponse(t, router, terminate, http.StatusAccepted, `"attempt":2`)
	unlock := executionMutationRequest(http.MethodPost, executionCommandPath(requestID, versionID, "unlock"), `{"reason":"verified","expectedVersion":3,"actualStates":[]}`)
	assertExecutionResponse(t, router, unlock, http.StatusAccepted, `"attempt":2`)

	if service.getCalls != 1 || service.retryCalls != 1 || service.terminateCalls != 1 || service.unlockCalls != 1 {
		t.Fatalf("execution service calls = %#v", service)
	}
}

func TestDeploymentExecutionConflictUsesStableError(t *testing.T) {
	service := &fakeDeploymentExecutionService{value: executionFixture(), commandError: deployapp.ErrExecutionConflict}
	options := testAPIOptions(&fakeAuthFlow{})
	options.Deployment.Executions = service
	router := newTestHTTPRouter(t, options)
	path := executionCommandPath(uuid.New(), service.value.RequestVersionID, "terminate")
	request := executionMutationRequest(http.MethodPost, path, `{"reason":"stop","expectedVersion":3}`)
	assertExecutionResponse(t, router, request, http.StatusConflict, `"code":"DEPLOYMENT_EXECUTION_CONFLICT"`)
}

func executionMutationRequest(method, path, body string) *http.Request {
	request := workflowMutationRequest(method, path, body)
	request.Header.Set("Idempotency-Key", "execution-command-1")
	return request
}

func executionCommandPath(requestID, versionID uuid.UUID, command string) string {
	return "/api/v1/deployment-requests/" + requestID.String() + "/versions/" + versionID.String() + "/" + command
}

func assertExecutionResponse(t *testing.T, router http.Handler, request *http.Request, status int, body string) {
	t.Helper()
	response := httptest.NewRecorder()
	router.ServeHTTP(response, request)
	if response.Code != status || !strings.Contains(response.Body.String(), body) {
		t.Fatalf("execution response = %d %s", response.Code, response.Body.String())
	}
}

type fakeDeploymentExecutionService struct {
	value                                             deployapp.DeploymentExecution
	commandError                                      error
	getCalls, retryCalls, terminateCalls, unlockCalls int
}

func (s *fakeDeploymentExecutionService) Get(context.Context, deployapp.RequestPrincipal, uuid.UUID) (deployapp.DeploymentExecution, error) {
	s.getCalls++
	return s.value, nil
}

func (s *fakeDeploymentExecutionService) Retry(context.Context, deployapp.RequestPrincipal, deployapp.RetryExecutionInput) (deployapp.DeploymentExecution, error) {
	s.retryCalls++
	return s.value, s.commandError
}

func (s *fakeDeploymentExecutionService) Terminate(context.Context, deployapp.RequestPrincipal, deployapp.TerminateExecutionInput) (deployapp.DeploymentExecution, error) {
	s.terminateCalls++
	return s.value, s.commandError
}

func (s *fakeDeploymentExecutionService) Unlock(context.Context, deployapp.RequestPrincipal, deployapp.UnlockExecutionInput) (deployapp.DeploymentExecution, error) {
	s.unlockCalls++
	return s.value, s.commandError
}

func executionFixture() deployapp.DeploymentExecution {
	return deployapp.DeploymentExecution{
		ID: uuid.New(), RequestVersionID: uuid.New(), PlanVersionID: uuid.New(), Attempt: 2,
		Status: "PartialFailed", TriggerKind: "Retry", LockVersion: 3,
		Nodes: []deployapp.DeploymentExecutionNode{}, CreatedAt: time.Date(2026, 9, 7, 12, 0, 0, 0, time.UTC),
	}
}
