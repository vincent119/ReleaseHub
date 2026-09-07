package httpserver_test

import (
	"context"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/google/uuid"

	authz "github.com/vincent119/ReleaseHub/Server/internal/authorization/domain"
	deployapp "github.com/vincent119/ReleaseHub/Server/internal/deployment/application"
	deploydomain "github.com/vincent119/ReleaseHub/Server/internal/deployment/domain"
	"github.com/vincent119/ReleaseHub/Server/internal/transport/httpserver"
)

func TestDeploymentRequestRoutesUseInjectedService(t *testing.T) {
	service := &fakeDeploymentRequestService{detail: deploymentRequestDetailFixture()}
	options := testAPIOptions(&fakeAuthFlow{})
	options.Deployment.Requests = service
	router := newTestHTTPRouter(t, options)
	value := service.detail.Summary
	path := "/api/v1/deployment-requests?organizationId=" + value.OrganizationID.String() + "&projectId=" + value.ProjectID.String() + "&environmentId=" + value.EnvironmentID.String()
	request := httptest.NewRequest(http.MethodGet, path, nil)
	request.AddCookie(&http.Cookie{Name: "releasehub_session", Value: "session-token"})
	response := httptest.NewRecorder()
	router.ServeHTTP(response, request)
	if response.Code != http.StatusOK || service.listCalls != 1 || !strings.Contains(response.Body.String(), `"title":"Release API"`) {
		t.Fatalf("list requests = %d %s", response.Code, response.Body.String())
	}

	metadata := httptest.NewRequest(http.MethodPatch, "/api/v1/deployment-requests/"+value.ID.String()+"/versions/"+service.detail.Version.ID.String(), strings.NewReader(`{"expectedVersion":1,"changeDescription":"release note"}`))
	metadata.AddCookie(&http.Cookie{Name: "releasehub_session", Value: "session-token"})
	metadata.Header.Set("Content-Type", "application/json")
	metadata.Header.Set("Origin", "https://releasehub.example")
	metadata.Header.Set("X-CSRF-Token", "csrf-token")
	updated := httptest.NewRecorder()
	router.ServeHTTP(updated, metadata)
	if updated.Code != http.StatusOK || service.metadata.Metadata.ChangeDescription != "release note" {
		t.Fatalf("update metadata = %d %s", updated.Code, updated.Body.String())
	}
}

type fakeDeploymentRequestService struct {
	detail    deploydomain.DeploymentRequestDetail
	listCalls int
	metadata  deployapp.MetadataVersionChange
}

func (s *fakeDeploymentRequestService) List(_ context.Context, _ deployapp.RequestPrincipal, _ authz.Scope) ([]deploydomain.DeploymentRequestSummary, error) {
	s.listCalls++
	return []deploydomain.DeploymentRequestSummary{s.detail.Summary}, nil
}

func (s *fakeDeploymentRequestService) Get(_ context.Context, _ deployapp.RequestPrincipal, _ uuid.UUID) (deploydomain.DeploymentRequestDetail, error) {
	return s.detail, nil
}

func (s *fakeDeploymentRequestService) UpdateMetadata(_ context.Context, _ deployapp.RequestPrincipal, input deployapp.MetadataVersionChange, _ string) (deploydomain.DeploymentRequestDetail, error) {
	s.metadata = input
	return s.detail, nil
}

func deploymentRequestDetailFixture() deploydomain.DeploymentRequestDetail {
	now := time.Date(2026, 9, 3, 10, 0, 0, 0, time.UTC)
	requestID, versionID := uuid.New(), uuid.New()
	return deploydomain.DeploymentRequestDetail{Summary: deploydomain.DeploymentRequestSummary{
		ID: requestID, OrganizationID: uuid.New(), ProjectID: uuid.New(), EnvironmentID: uuid.New(),
		Classification: "Standard", Status: deploydomain.DeploymentRequestCandidate, Title: "Release API", ActiveVersionNumber: 1, ApplicationCount: 1, UpdatedAt: now,
	}, Version: deploydomain.DeploymentRequestVersionSummary{
		ID: versionID, RequestID: requestID, VersionNumber: 1, Status: deploydomain.DeploymentRequestCandidate,
		Fingerprint: strings.Repeat("a", 64), WorkflowVersionID: uuid.New(), PlanVersionID: uuid.New(), Title: "Release API", LockVersion: 1, CreatedAt: now,
	}, Reviews: []deploydomain.DeploymentRequestReviewSnapshot{{
		ID: uuid.New(), StateKey: "review", StageNumber: 1, PolicyType: deploydomain.ReviewPolicyAny,
		RequiredApprovals: 1, Status: deploydomain.ReviewTaskPending,
	}}}
}

var _ httpserver.DeploymentHandlerOptions
