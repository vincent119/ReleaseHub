package httpserver_test

import (
	"errors"
	"fmt"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strings"
	"testing"

	"github.com/stretchr/testify/require"

	deployapp "github.com/vincent119/ReleaseHub/Server/internal/deployment/application"
	deploydomain "github.com/vincent119/ReleaseHub/Server/internal/deployment/domain"
)

func TestDeploymentRequestListRejectsInvalidLimit(t *testing.T) {
	for _, value := range []string{"0", "-1", "101", "1.5", "", "invalid"} {
		t.Run(value, func(t *testing.T) {
			service := &fakeDeploymentRequestService{detail: deploymentRequestDetailFixture()}
			options := testAPIOptions(&fakeAuthFlow{})
			actual, _ := newRequestListHTTPService(t, service.detail, true)
			options.Deployment.Requests = actual
			router := newTestHTTPRouter(t, options)
			request := httptest.NewRequest(http.MethodGet, requestListPath(service)+"&limit="+value, nil)
			request.AddCookie(&http.Cookie{Name: "releasehub_session", Value: "session-token"})
			response := httptest.NewRecorder()
			router.ServeHTTP(response, request)
			require.Equal(t, http.StatusBadRequest, response.Code, response.Body.String())
			require.Contains(t, response.Body.String(), `"code":"INVALID_REQUEST"`)
		})
	}
}

func TestDeploymentRequestListRejectsInvalidFiltersAndCursor(t *testing.T) {
	queries := []string{"status=", "status=all", "status=succeeded", "status=Candidate&status=Failed",
		"limit=20&limit=50", "cursor=", "cursor=invalid", "cursor=" + strings.Repeat("x", 1025),
		"search=" + url.QueryEscape(strings.Repeat("界", 256))}
	for _, query := range queries {
		t.Run(query[:min(len(query), 45)], func(t *testing.T) {
			fixture := &fakeDeploymentRequestService{detail: deploymentRequestDetailFixture()}
			service, repo := newRequestListHTTPService(t, fixture.detail, true)
			options := testAPIOptions(&fakeAuthFlow{})
			options.Deployment.Requests = service
			response := serveRequestList(t, newTestHTTPRouter(t, options), requestListPath(fixture)+"&"+query, true)
			require.Equal(t, http.StatusBadRequest, response.Code, response.Body.String())
			require.Contains(t, response.Body.String(), `"code":"INVALID_REQUEST"`)
			require.Contains(t, response.Body.String(), `"category":"validation"`)
			require.Contains(t, response.Body.String(), `"retryable":false`)
			require.Zero(t, repo.calls)
		})
	}
}

func TestDeploymentRequestListForwardsQueryAndPageMeta(t *testing.T) {
	fixture := &fakeDeploymentRequestService{detail: deploymentRequestDetailFixture()}
	fixture.listPage = &deployapp.DeploymentRequestListPage{
		Items: []deploydomain.DeploymentRequestSummary{fixture.detail.Summary}, HasMore: true, NextCursor: "server-cursor",
	}
	options := testAPIOptions(&fakeAuthFlow{})
	options.Deployment.Requests = fixture
	path := requestListPath(fixture) + "&limit=50&search=api&status=Candidate&cursor=previous-cursor"
	response := serveRequestList(t, newTestHTTPRouter(t, options), path, true)
	require.Equal(t, http.StatusOK, response.Code, response.Body.String())
	require.Equal(t, 50, *fixture.listQuery.Limit)
	require.Equal(t, "api", *fixture.listQuery.Search)
	require.Equal(t, "Candidate", *fixture.listQuery.Status)
	require.Equal(t, "previous-cursor", *fixture.listQuery.Cursor)
	require.Contains(t, response.Body.String(), `"hasMore":true`)
	require.Contains(t, response.Body.String(), `"nextCursor":"server-cursor"`)
}

func TestDeploymentRequestListPreservesAuthenticationAndDeny(t *testing.T) {
	fixture := &fakeDeploymentRequestService{detail: deploymentRequestDetailFixture()}
	service, repo := newRequestListHTTPService(t, fixture.detail, false)
	options := testAPIOptions(&fakeAuthFlow{})
	options.Deployment.Requests = service
	router := newTestHTTPRouter(t, options)
	path := requestListPath(fixture) + "&cursor=invalid"
	require.Equal(t, http.StatusUnauthorized, serveRequestList(t, router, path, false).Code)
	denied := serveRequestList(t, router, path, true)
	require.Equal(t, http.StatusNotFound, denied.Code)
	require.Contains(t, denied.Body.String(), `"code":"DEPLOYMENT_REQUEST_NOT_FOUND"`)
	require.NotContains(t, denied.Body.String(), "cursor")
	require.Zero(t, repo.calls)
}

func TestDeploymentRequestListDefaultsAndEmptyPage(t *testing.T) {
	fixture := &fakeDeploymentRequestService{detail: deploymentRequestDetailFixture()}
	service, repo := newRequestListHTTPService(t, fixture.detail, true)
	options := testAPIOptions(&fakeAuthFlow{})
	options.Deployment.Requests = service
	response := serveRequestList(t, newTestHTTPRouter(t, options), requestListPath(fixture), true)
	require.Equal(t, http.StatusOK, response.Code)
	require.Equal(t, 20, repo.filter.Limit)
	require.Contains(t, response.Body.String(), `"data":[]`)
	require.Contains(t, response.Body.String(), `"hasMore":false`)
	require.NotContains(t, response.Body.String(), "nextCursor")
}

func TestDeploymentRequestListErrorContract(t *testing.T) {
	tests := []struct {
		name   string
		err    error
		status int
		code   string
	}{
		{"query", deployapp.ErrRequestQueryInvalid, 400, "INVALID_REQUEST"},
		{"forbidden", deployapp.ErrRequestForbidden, 404, "DEPLOYMENT_REQUEST_NOT_FOUND"},
		{"not found", deployapp.ErrRequestNotFound, 404, "DEPLOYMENT_REQUEST_NOT_FOUND"},
		{"metadata", deployapp.ErrRequestInvalid, 422, "DEPLOYMENT_REQUEST_INVALID"},
		{"repository", errors.New("private SQL error"), 500, "DEPLOYMENT_REQUEST_FAILED"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			fixture := &fakeDeploymentRequestService{detail: deploymentRequestDetailFixture(), listError: tt.err}
			options := testAPIOptions(&fakeAuthFlow{})
			options.Deployment.Requests = fixture
			response := serveRequestList(t, newTestHTTPRouter(t, options), requestListPath(fixture), true)
			require.Equal(t, tt.status, response.Code)
			require.Contains(t, response.Body.String(), `"code":"`+tt.code+`"`)
			require.NotContains(t, response.Body.String(), "private SQL error")
			require.NotContains(t, response.Body.String(), `"data":[]`)
		})
	}
}

func TestDeploymentRequestListAcceptsBoundedQueries(t *testing.T) {
	for _, limit := range []string{"1", "20", "50", "100"} {
		t.Run(limit, func(t *testing.T) {
			fixture := &fakeDeploymentRequestService{detail: deploymentRequestDetailFixture()}
			service, repo := newRequestListHTTPService(t, fixture.detail, true)
			options := testAPIOptions(&fakeAuthFlow{})
			options.Deployment.Requests = service
			search := "  " + strings.Repeat("界", 255) + "  "
			path := requestListPath(fixture) + "&limit=" + limit + "&search=" + url.QueryEscape(search) + "&status=Succeeded"
			response := serveRequestList(t, newTestHTTPRouter(t, options), path, true)
			require.Equal(t, http.StatusOK, response.Code, response.Body.String())
			require.Equal(t, strings.TrimSpace(search), repo.filter.Search)
			require.Equal(t, deploydomain.DeploymentRequestSucceeded, repo.filter.Status)
			require.Equal(t, limit, fmt.Sprint(repo.filter.Limit))
		})
	}
}

func serveRequestList(t *testing.T, router http.Handler, path string, authenticated bool) *httptest.ResponseRecorder {
	t.Helper()
	request := httptest.NewRequest(http.MethodGet, path, nil)
	if authenticated {
		request.AddCookie(&http.Cookie{Name: "releasehub_session", Value: "session-token"})
	}
	response := httptest.NewRecorder()
	router.ServeHTTP(response, request)
	return response
}

func requestListPath(service *fakeDeploymentRequestService) string {
	summary := service.detail.Summary
	return "/api/v1/deployment-requests?organizationId=" + summary.OrganizationID.String() +
		"&projectId=" + summary.ProjectID.String() + "&environmentId=" + summary.EnvironmentID.String()
}
