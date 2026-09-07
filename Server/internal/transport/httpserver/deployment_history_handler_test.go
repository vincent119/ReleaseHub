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

func TestDeploymentHistoryRouteUsesInjectedService(t *testing.T) {
	service := &fakeDeploymentHistoryService{page: deploymentHistoryPageFixture()}
	options := testAPIOptions(&fakeAuthFlow{})
	options.Deployment.History = service
	router := newTestHTTPRouter(t, options)
	projectID, environmentID := uuid.New(), uuid.New()
	path := "/api/v1/deployment-history?projectId=" + projectID.String() + "&environmentId=" + environmentID.String() + "&limit=1"
	request := httptest.NewRequest(http.MethodGet, path, nil)
	request.AddCookie(&http.Cookie{Name: "releasehub_session", Value: "session-token"})
	response := httptest.NewRecorder()
	router.ServeHTTP(response, request)
	if response.Code != http.StatusOK || !strings.Contains(response.Body.String(), `"classification":"ForwardRollback"`) {
		t.Fatalf("history response = %d %s", response.Code, response.Body.String())
	}
	if service.query.ProjectID != projectID || service.query.EnvironmentID != environmentID || service.query.Limit != 1 {
		t.Fatalf("history query = %#v", service.query)
	}
}

type fakeDeploymentHistoryService struct {
	page  deployapp.DeploymentHistoryPage
	query deployapp.DeploymentHistoryQuery
}

func (s *fakeDeploymentHistoryService) List(_ context.Context, _ deployapp.RequestPrincipal, query deployapp.DeploymentHistoryQuery) (deployapp.DeploymentHistoryPage, error) {
	s.query = query
	return s.page, nil
}

func deploymentHistoryPageFixture() deployapp.DeploymentHistoryPage {
	return deployapp.DeploymentHistoryPage{HasMore: true, NextCursor: "next", Items: []deploydomain.DeploymentHistoryItem{{
		RequestID: uuid.New(), RequestVersionID: uuid.New(),
		Classification: deploydomain.DeploymentClassificationForwardRollback,
		CompletedAt:    time.Date(2026, 9, 7, 10, 0, 0, 0, time.UTC),
		Applications: []deploydomain.DeploymentRequestApplicationSnapshot{{
			ID: uuid.New(), ApplicationID: uuid.New(), ApplicationKey: "api",
			TargetRevision: "revision-a", TargetRevisions: []string{"revision-a"},
			ManifestHash: strings.Repeat("a", 64), DiffHash: strings.Repeat("b", 64),
			DiffSnapshot: []byte(`{}`), Images: []deploydomain.DeploymentRequestImageSnapshot{{
				Registry: "registry.example", Repository: "api", Tag: "v2.0.0", Digest: "sha256:old",
			}},
		}},
	}}}
}
