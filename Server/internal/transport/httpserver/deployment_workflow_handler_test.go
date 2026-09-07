package httpserver_test

import (
	"context"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/google/uuid"

	deployapp "github.com/vincent119/ReleaseHub/Server/internal/deployment/application"
	deploydomain "github.com/vincent119/ReleaseHub/Server/internal/deployment/domain"
)

func TestDeploymentWorkflowRoutesUseInjectedRuntime(t *testing.T) {
	requests := &fakeDeploymentRequestService{detail: deploymentRequestDetailFixture()}
	workflows := &fakeDeploymentWorkflowService{}
	options := testAPIOptions(&fakeAuthFlow{})
	options.Deployment.Requests = requests
	options.Deployment.Workflows = workflows
	router := newTestHTTPRouter(t, options)

	reviewID := uuid.New()
	path := deploymentVersionPath(requests.detail) + "/reviews/" + reviewID.String() + "/decisions"
	review := deploymentWorkflowRequest(http.MethodPost, path, `{"decision":"Approve","expectedVersion":1}`, "review-1")
	reviewResponse := httptest.NewRecorder()
	router.ServeHTTP(reviewResponse, review)
	if reviewResponse.Code != http.StatusOK || workflows.review.ReviewTaskID != reviewID || !strings.Contains(reviewResponse.Body.String(), `"policyType":"AnyApprover"`) {
		t.Fatalf("review workflow = %d %s", reviewResponse.Code, reviewResponse.Body.String())
	}

	reassignPath := deploymentVersionPath(requests.detail) + "/reviews/" + reviewID.String() + "/reassignments"
	reassign := deploymentWorkflowRequest(http.MethodPost, reassignPath, `{"userIds":["`+uuid.NewString()+`"],"roleIds":[],"reason":"reviewer unavailable","expectedVersion":1}`, "reassign-1")
	reassignResponse := httptest.NewRecorder()
	router.ServeHTTP(reassignResponse, reassign)
	if reassignResponse.Code != http.StatusOK || workflows.reassignment.ReviewTaskID != reviewID {
		t.Fatalf("reassign workflow = %d %s", reassignResponse.Code, reassignResponse.Body.String())
	}

	transition := deploymentWorkflowRequest(http.MethodPost, deploymentVersionPath(requests.detail)+"/transitions", `{"transitionKey":"deploy","expectedVersion":1}`, "transition-1")
	transitionResponse := httptest.NewRecorder()
	router.ServeHTTP(transitionResponse, transition)
	if transitionResponse.Code != http.StatusAccepted || workflows.transition.Trigger != deploydomain.WorkflowTriggerManual {
		t.Fatalf("transition workflow = %d %s", transitionResponse.Code, transitionResponse.Body.String())
	}
}

func TestDeploymentWorkflowRejectsMismatchedRequestVersion(t *testing.T) {
	requests := &fakeDeploymentRequestService{detail: deploymentRequestDetailFixture()}
	workflows := &fakeDeploymentWorkflowService{}
	options := testAPIOptions(&fakeAuthFlow{})
	options.Deployment.Requests = requests
	options.Deployment.Workflows = workflows
	router := newTestHTTPRouter(t, options)
	path := "/api/v1/deployment-requests/" + requests.detail.Summary.ID.String() + "/versions/" + uuid.NewString() + "/transitions"
	request := deploymentWorkflowRequest(http.MethodPost, path, `{"transitionKey":"deploy","expectedVersion":1}`, "transition-2")
	response := httptest.NewRecorder()
	router.ServeHTTP(response, request)
	if response.Code != http.StatusNotFound || workflows.transition.RequestVersionID != uuid.Nil {
		t.Fatalf("mismatched workflow version = %d %s", response.Code, response.Body.String())
	}
}

func deploymentVersionPath(value deploydomain.DeploymentRequestDetail) string {
	return "/api/v1/deployment-requests/" + value.Summary.ID.String() + "/versions/" + value.Version.ID.String()
}

func deploymentWorkflowRequest(method, path, body, key string) *http.Request {
	request := httptest.NewRequest(method, path, strings.NewReader(body))
	request.AddCookie(&http.Cookie{Name: "releasehub_session", Value: "session-token"})
	request.Header.Set("Content-Type", "application/json")
	request.Header.Set("Origin", "https://releasehub.example")
	request.Header.Set("X-CSRF-Token", "csrf-token")
	request.Header.Set("Idempotency-Key", key)
	return request
}

type fakeDeploymentWorkflowService struct {
	review       deployapp.WorkflowReviewInput
	reassignment deployapp.WorkflowReviewReassignmentInput
	transition   deployapp.WorkflowTransitionInput
}

func (s *fakeDeploymentWorkflowService) Transition(_ context.Context, _ deployapp.WorkflowPrincipal, input deployapp.WorkflowTransitionInput) (deploydomain.WorkflowResult, error) {
	s.transition = input
	return deploydomain.WorkflowResult{}, nil
}

func (s *fakeDeploymentWorkflowService) DecideReview(_ context.Context, _ deployapp.WorkflowPrincipal, input deployapp.WorkflowReviewInput) (deploydomain.ReviewTask, error) {
	s.review = input
	return deploydomain.ReviewTask{}, nil
}

func (s *fakeDeploymentWorkflowService) ReassignReview(_ context.Context, _ deployapp.WorkflowPrincipal, input deployapp.WorkflowReviewReassignmentInput) (deploydomain.ReviewTask, error) {
	s.reassignment = input
	return deploydomain.ReviewTask{}, nil
}
