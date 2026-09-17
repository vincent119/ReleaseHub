package httpserver_test

import (
	"context"
	"errors"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/google/uuid"

	auditapp "github.com/vincent119/ReleaseHub/Server/internal/audit/application"
	auditdomain "github.com/vincent119/ReleaseHub/Server/internal/audit/domain"
	authz "github.com/vincent119/ReleaseHub/Server/internal/authorization/domain"
)

func TestAuditRoutesUseInjectedServiceAndOmitListMetadata(t *testing.T) {
	organizationID, projectID := uuid.New(), uuid.New()
	scope, err := authz.NewProjectScope(organizationID, projectID)
	if err != nil {
		t.Fatal(err)
	}
	eventID := uuid.New()
	service := &fakeAuditQueryService{scope: scope, capabilities: auditapp.Capabilities{Visible: true, ScopeRoots: []auditapp.ScopeRoot{{Scope: scope, Label: "Project A"}}}, page: auditapp.QueryPage{
		HasMore: true, NextCursor: "next-audit", Items: []auditdomain.EventSummary{{
			ID: eventID, OccurredAt: time.Date(2026, 9, 17, 1, 0, 0, 0, time.UTC), Action: "workflow.published",
			ResourceType: "release_workflow", ResourceID: uuid.NewString(), RequestID: "request-audit", HasMetadata: true,
			Scope: auditdomain.ScopeAncestry{OrganizationID: &organizationID, ProjectID: &projectID, Resolution: auditdomain.ScopeResolved},
		}}}, detail: auditdomain.EventDetail{EventSummary: auditdomain.EventSummary{
		ID: eventID, OccurredAt: time.Date(2026, 9, 17, 1, 0, 0, 0, time.UTC), Action: "workflow.published",
		ResourceType: "release_workflow", ResourceID: uuid.NewString(), HasMetadata: true,
		Scope: auditdomain.ScopeAncestry{OrganizationID: &organizationID, ProjectID: &projectID, Resolution: auditdomain.ScopeResolved},
	}, Metadata: map[string]any{"safe": "metadata-log-canary"}}, options: []auditapp.FilterOption{{Value: "workflow.published", Label: "workflow.published"}},
	}
	options := testAPIOptions(&fakeAuthFlow{})
	options.Audit.Queries = service
	router, recorded, _, _, _ := newTestRouterFromOptionsWithState(t, options)

	capability := auditRequest(t, router, "/api/v1/audit/capabilities")
	if capability.Code != http.StatusOK || !strings.Contains(capability.Body.String(), `"visible":true`) || !strings.Contains(capability.Body.String(), `"label":"Project A"`) {
		t.Fatalf("capability response = %d %s", capability.Code, capability.Body.String())
	}
	list := auditRequest(t, router, "/api/v1/audit/events?scopeKind=project&scopeId="+projectID.String()+"&limit=1")
	if list.Code != http.StatusOK || !strings.Contains(list.Body.String(), `"nextCursor":"next-audit"`) || strings.Contains(list.Body.String(), `"metadata":`) {
		t.Fatalf("list response = %d %s", list.Code, list.Body.String())
	}
	if service.filter.Scope != scope || service.filter.Limit != 1 || service.resolvedID == nil || *service.resolvedID != projectID {
		t.Fatalf("resolved scope=%#v filter=%#v", service.resolvedID, service.filter)
	}
	filterOptions := auditRequest(t, router, "/api/v1/audit/filter-options?scopeKind=project&scopeId="+projectID.String()+"&field=action&search=workflow&limit=10")
	if filterOptions.Code != http.StatusOK || !strings.Contains(filterOptions.Body.String(), `"value":"workflow.published"`) {
		t.Fatalf("filter options response = %d %s", filterOptions.Code, filterOptions.Body.String())
	}
	if service.optionFilter.Scope != scope || service.optionFilter.Field != auditdomain.FilterOptionAction || service.optionFilter.Search != "workflow" || service.optionFilter.Limit != 10 {
		t.Fatalf("filter option request = %#v", service.optionFilter)
	}
	detail := auditRequest(t, router, "/api/v1/audit/events/"+eventID.String())
	if detail.Code != http.StatusOK || !strings.Contains(detail.Body.String(), `"metadata":{"safe":"metadata-log-canary"}`) {
		t.Fatalf("detail response = %d %s", detail.Code, detail.Body.String())
	}
	for _, entry := range recorded.All() {
		if strings.Contains(fmt.Sprint(entry.ContextMap()), "metadata-log-canary") {
			t.Fatalf("audit metadata leaked to request log: %#v", entry.ContextMap())
		}
	}
}

func TestAuditRouteMapsInvalidCursorToUnifiedError(t *testing.T) {
	service := &fakeAuditQueryService{scope: authz.NewPlatformScope(), listErr: auditdomain.ErrInvalidCursor}
	options := testAPIOptions(&fakeAuthFlow{})
	options.Audit.Queries = service
	response := auditRequest(t, newTestHTTPRouter(t, options), "/api/v1/audit/events?scopeKind=platform&cursor=invalid")
	if response.Code != http.StatusBadRequest || !strings.Contains(response.Body.String(), `"code":"AUDIT_INVALID_QUERY"`) || !strings.Contains(response.Body.String(), `"category":"validation"`) {
		t.Fatalf("invalid cursor response = %d %s", response.Code, response.Body.String())
	}
}

func TestAuditRouteMapsMaskedDependencyAndCancellationErrors(t *testing.T) {
	tests := []struct {
		name       string
		err        error
		status     int
		bodyMarker string
	}{
		{name: "masked", err: auditapp.ErrForbidden, status: http.StatusNotFound, bodyMarker: `"code":"AUDIT_NOT_FOUND"`},
		{name: "dependency", err: errors.New("database unavailable"), status: http.StatusServiceUnavailable, bodyMarker: `"retryable":true`},
		{name: "canceled", err: context.Canceled, status: 499},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			service := &fakeAuditQueryService{scope: authz.NewPlatformScope(), listErr: test.err}
			options := testAPIOptions(&fakeAuthFlow{})
			options.Audit.Queries = service
			response := auditRequest(t, newTestHTTPRouter(t, options), "/api/v1/audit/events?scopeKind=platform")
			if response.Code != test.status || (test.bodyMarker != "" && !strings.Contains(response.Body.String(), test.bodyMarker)) {
				t.Fatalf("response = %d %s", response.Code, response.Body.String())
			}
		})
	}
}

func auditRequest(t *testing.T, router http.Handler, path string) *httptest.ResponseRecorder {
	t.Helper()
	request := httptest.NewRequest(http.MethodGet, path, nil)
	request.AddCookie(&http.Cookie{Name: "releasehub_session", Value: "session-token"})
	response := httptest.NewRecorder()
	router.ServeHTTP(response, request)
	return response
}

type fakeAuditQueryService struct {
	capabilities auditapp.Capabilities
	scope        authz.Scope
	page         auditapp.QueryPage
	detail       auditdomain.EventDetail
	options      []auditapp.FilterOption
	listErr      error
	filter       auditdomain.QueryFilter
	optionFilter auditdomain.FilterOptionFilter
	resolvedID   *uuid.UUID
}

func (s *fakeAuditQueryService) Capabilities(context.Context, auditapp.Principal) (auditapp.Capabilities, error) {
	return s.capabilities, nil
}

func (s *fakeAuditQueryService) ResolveScope(_ context.Context, _ authz.ScopeKind, id *uuid.UUID) (authz.Scope, error) {
	s.resolvedID = id
	if s.scope.Kind == "" {
		return authz.Scope{}, auditapp.ErrNotFound
	}
	return s.scope, nil
}

func (s *fakeAuditQueryService) List(_ context.Context, _ auditapp.Principal, filter auditdomain.QueryFilter, _ string) (auditapp.QueryPage, error) {
	s.filter = filter
	return s.page, s.listErr
}

func (s *fakeAuditQueryService) FilterOptions(_ context.Context, _ auditapp.Principal, filter auditdomain.FilterOptionFilter) ([]auditapp.FilterOption, error) {
	s.optionFilter = filter
	return s.options, s.listErr
}

func (s *fakeAuditQueryService) Detail(_ context.Context, _ auditapp.Principal, eventID uuid.UUID) (auditdomain.EventDetail, error) {
	if s.detail.ID != eventID {
		return auditdomain.EventDetail{}, errors.New("unexpected audit event ID")
	}
	return s.detail, nil
}
