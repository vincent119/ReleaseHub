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
	"go.opentelemetry.io/otel/sdk/trace/tracetest"
	"go.uber.org/zap"
	"go.uber.org/zap/zapcore"
	"go.uber.org/zap/zaptest/observer"

	argoapp "github.com/vincent119/ReleaseHub/Server/internal/argocd/application"
	argodomain "github.com/vincent119/ReleaseHub/Server/internal/argocd/domain"
	authzapp "github.com/vincent119/ReleaseHub/Server/internal/authorization/application"
	catalogapp "github.com/vincent119/ReleaseHub/Server/internal/catalog/application"
	catalog "github.com/vincent119/ReleaseHub/Server/internal/catalog/domain"
	identityapp "github.com/vincent119/ReleaseHub/Server/internal/identity/application"
	identity "github.com/vincent119/ReleaseHub/Server/internal/identity/domain"
	"github.com/vincent119/ReleaseHub/Server/internal/observability"
	"github.com/vincent119/ReleaseHub/Server/internal/transport/httpserver"
)

func TestAuthDefaultDenyWithoutSession(t *testing.T) {
	router, _, _, _, _ := newTestRouter(t)
	response := httptest.NewRecorder()
	router.ServeHTTP(response, httptest.NewRequest(http.MethodGet, "/api/v1/auth/session", nil))
	if response.Code != http.StatusUnauthorized {
		t.Fatalf("expected 401, got %d", response.Code)
	}
	if !strings.Contains(response.Body.String(), `"code":"SESSION_REQUIRED"`) {
		t.Fatalf("unexpected response: %s", response.Body.String())
	}
}

func TestAuthSessionDistinguishesInvalidSessionFromInfrastructureFailure(t *testing.T) {
	tests := []struct {
		name             string
		authenticateErr  error
		wantStatus       int
		wantCode         string
		wantClearedCount int
		wantLogLevel     zapcore.Level
		wantLoggedError  string
	}{
		{
			name:             "terminal invalid session",
			authenticateErr:  identityapp.ErrSessionInvalid,
			wantStatus:       http.StatusUnauthorized,
			wantCode:         "SESSION_INVALID",
			wantClearedCount: 2,
			wantLogLevel:     zap.WarnLevel,
		},
		{
			name:            "authentication infrastructure failure",
			authenticateErr: errors.New("identity database unavailable"),
			wantStatus:      http.StatusServiceUnavailable,
			wantCode:        "SESSION_STATE_UNAVAILABLE",
			wantLogLevel:    zap.ErrorLevel,
			wantLoggedError: "authenticate session: identity database unavailable",
		},
		{
			name:            "authentication deadline exceeded",
			authenticateErr: context.DeadlineExceeded,
			wantStatus:      http.StatusServiceUnavailable,
			wantCode:        "SESSION_STATE_UNAVAILABLE",
			wantLogLevel:    zap.ErrorLevel,
			wantLoggedError: "authenticate session: context deadline exceeded",
		},
		{
			name:            "client canceled authentication",
			authenticateErr: context.Canceled,
			wantStatus:      499,
			wantLogLevel:    zap.InfoLevel,
		},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			router, recorded, _, _, _ := newTestRouterFromOptionsWithState(t, testAPIOptions(&fakeAuthFlow{authenticateErr: test.authenticateErr}))
			request := httptest.NewRequest(http.MethodGet, "/api/v1/auth/session", nil)
			request.AddCookie(&http.Cookie{Name: "releasehub_session", Value: "session-token"})
			response := httptest.NewRecorder()

			router.ServeHTTP(response, request)

			if response.Code != test.wantStatus {
				t.Fatalf("auth session = %d %s", response.Code, response.Body.String())
			}
			if test.wantCode == "" {
				if response.Body.Len() != 0 {
					t.Fatalf("canceled auth session body = %q, want empty", response.Body.String())
				}
			} else if !strings.Contains(response.Body.String(), `"code":"`+test.wantCode+`"`) {
				t.Fatalf("auth session body = %s, want code %s", response.Body.String(), test.wantCode)
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
			entries := recorded.All()
			if len(entries) != 1 || entries[0].Level != test.wantLogLevel {
				t.Fatalf("request logs = %#v, want level %s", entries, test.wantLogLevel)
			}
			loggedError, _ := entries[0].ContextMap()["error"].(string)
			if loggedError != test.wantLoggedError || strings.Contains(loggedError, "session-token") {
				t.Fatalf("logged error = %q, want %q", loggedError, test.wantLoggedError)
			}
		})
	}
}

func TestSystemStatusReportsOIDCCapability(t *testing.T) {
	options := testAPIOptions(&fakeAuthFlow{})
	options.System.OIDCEnabled = true
	router := newTestRouterFromOptions(t, options)
	response := httptest.NewRecorder()

	router.ServeHTTP(response, httptest.NewRequest(http.MethodGet, "/api/v1/system/status", nil))

	if response.Code != http.StatusOK || !strings.Contains(response.Body.String(), `"oidcEnabled":true`) {
		t.Fatalf("system status = %d %s", response.Code, response.Body.String())
	}
}

func TestLocalLoginSetsSessionCookiesAndRequiresPasswordChange(t *testing.T) {
	userID := uuid.New()
	local := &fakeLocalAuth{user: identity.User{ID: userID, Username: "admin"}, mustChange: true}
	options := testAPIOptions(&fakeAuthFlow{})
	options.Auth.Local = local
	router := newTestRouterFromOptions(t, options)
	request := httptest.NewRequest(http.MethodPost, "/api/v1/auth/local/login", strings.NewReader(`{"username":"admin","password":"admin"}`))
	request.Header.Set("Content-Type", "application/json")
	request.Header.Set("Origin", "https://releasehub.example")
	response := httptest.NewRecorder()

	router.ServeHTTP(response, request)

	if response.Code != http.StatusOK || !strings.Contains(response.Body.String(), `"mustChangePassword":true`) || !strings.Contains(response.Body.String(), `"passwordChangeAvailable":true`) || !strings.Contains(response.Body.String(), `"idleExpiresAt":"2026-09-14T08:30:00Z"`) || !strings.Contains(response.Body.String(), `"absoluteExpiresAt":"2026-09-14T20:00:00Z"`) {
		t.Fatalf("local login = %d %s", response.Code, response.Body.String())
	}
	if local.loginCalls != 1 || len(response.Result().Cookies()) != 2 {
		t.Fatalf("local login calls=%d cookies=%d", local.loginCalls, len(response.Result().Cookies()))
	}
}

func TestAuthSessionReportsPasswordChangeCapabilityFromAuthenticationMethod(t *testing.T) {
	userID := uuid.New()
	tests := []struct {
		name                        string
		authenticationMethod        string
		local                       *fakeLocalAuth
		wantMustChange              bool
		wantPasswordChangeAvailable bool
	}{
		{
			name:                        "local session",
			authenticationMethod:        "local",
			local:                       &fakeLocalAuth{mustChange: true},
			wantMustChange:              true,
			wantPasswordChangeAvailable: true,
		},
		{
			name:                        "OIDC session",
			authenticationMethod:        "oidc",
			local:                       &fakeLocalAuth{mustChange: true},
			wantMustChange:              false,
			wantPasswordChangeAvailable: false,
		},
		{
			name:                        "local session without local authentication service",
			authenticationMethod:        "local",
			wantMustChange:              false,
			wantPasswordChangeAvailable: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			idleExpiresAt := time.Date(2026, time.September, 14, 8, 30, 0, 0, time.UTC)
			absoluteExpiresAt := time.Date(2026, time.September, 14, 20, 0, 0, 0, time.UTC)
			flow := &fakeAuthFlow{
				session: identity.Session{ID: uuid.New(), UserID: userID, AuthenticationMethod: tt.authenticationMethod, IdleExpiresAt: idleExpiresAt, AbsoluteExpiresAt: absoluteExpiresAt},
				user:    identity.User{ID: userID, Username: "vincent"},
			}
			options := testAPIOptions(flow)
			if tt.local != nil {
				options.Auth.Local = tt.local
			}
			router := newTestRouterFromOptions(t, options)
			request := httptest.NewRequest(http.MethodGet, "/api/v1/auth/session", nil)
			request.AddCookie(&http.Cookie{Name: "releasehub_session", Value: "session-token"})
			response := httptest.NewRecorder()

			router.ServeHTTP(response, request)

			wantMustChange := `"mustChangePassword":false`
			if tt.wantMustChange {
				wantMustChange = `"mustChangePassword":true`
			}
			wantAvailable := `"passwordChangeAvailable":false`
			if tt.wantPasswordChangeAvailable {
				wantAvailable = `"passwordChangeAvailable":true`
			}
			if response.Code != http.StatusOK || !strings.Contains(response.Body.String(), wantMustChange) || !strings.Contains(response.Body.String(), wantAvailable) || !strings.Contains(response.Body.String(), `"idleExpiresAt":"2026-09-14T08:30:00Z"`) || !strings.Contains(response.Body.String(), `"absoluteExpiresAt":"2026-09-14T20:00:00Z"`) {
				t.Fatalf("auth session = %d %s", response.Code, response.Body.String())
			}
		})
	}
}

func TestLocalInitialPasswordBlocksProtectedRoutesUntilChanged(t *testing.T) {
	userID := uuid.New()
	flow := &fakeAuthFlow{session: identity.Session{ID: uuid.New(), UserID: userID, AuthenticationMethod: "local"}, user: identity.User{ID: userID, Username: "admin"}}
	local := &fakeLocalAuth{user: flow.user, mustChange: true}
	options := testAPIOptions(flow)
	options.Auth.Local = local
	router := newTestRouterFromOptions(t, options)
	request := httptest.NewRequest(http.MethodGet, "/api/v1/catalog/visible-applications", nil)
	request.AddCookie(&http.Cookie{Name: "releasehub_session", Value: "session-token"})
	response := httptest.NewRecorder()

	router.ServeHTTP(response, request)

	if response.Code != http.StatusForbidden || !strings.Contains(response.Body.String(), `"code":"PASSWORD_CHANGE_REQUIRED"`) {
		t.Fatalf("forced password change = %d %s", response.Code, response.Body.String())
	}
}

func TestChangeLocalPasswordAllowsInitialSessionAndClearsCookies(t *testing.T) {
	userID := uuid.New()
	flow := &fakeAuthFlow{session: identity.Session{ID: uuid.New(), UserID: userID, AuthenticationMethod: "local"}, user: identity.User{ID: userID, Username: "admin"}}
	local := &fakeLocalAuth{user: flow.user, mustChange: true}
	options := testAPIOptions(flow)
	options.Auth.Local = local
	router := newTestRouterFromOptions(t, options)
	request := httptest.NewRequest(http.MethodPost, "/api/v1/auth/password", strings.NewReader(`{"currentPassword":"admin","newPassword":"changed-password"}`))
	request.AddCookie(&http.Cookie{Name: "releasehub_session", Value: "session-token"})
	request.Header.Set("Content-Type", "application/json")
	request.Header.Set("Origin", "https://releasehub.example")
	request.Header.Set("X-CSRF-Token", "csrf-token")
	response := httptest.NewRecorder()

	router.ServeHTTP(response, request)

	if response.Code != http.StatusNoContent || local.changeCalls != 1 {
		t.Fatalf("password change = %d %s calls=%d", response.Code, response.Body.String(), local.changeCalls)
	}
	for _, cookie := range response.Result().Cookies() {
		if cookie.MaxAge != -1 {
			t.Fatalf("password change cookie %s was not cleared", cookie.Name)
		}
	}
}

func TestDeploymentContractRoutesFailClosedUntilUseCasesAreWired(t *testing.T) {
	router, _, _, _, _ := newTestRouterWithAuth(t, &fakeAuthFlow{})
	organizationID, projectID, environmentID := uuid.New(), uuid.New(), uuid.New()
	listPath := "/api/v1/deployment-requests?organizationId=" + organizationID.String() + "&projectId=" + projectID.String() + "&environmentId=" + environmentID.String()

	unauthenticated := httptest.NewRecorder()
	router.ServeHTTP(unauthenticated, httptest.NewRequest(http.MethodGet, listPath, nil))
	if unauthenticated.Code != http.StatusUnauthorized {
		t.Fatalf("unauthenticated deployment request list status = %d", unauthenticated.Code)
	}

	authorizedRequest := httptest.NewRequest(http.MethodGet, listPath, nil)
	authorizedRequest.AddCookie(&http.Cookie{Name: "releasehub_session", Value: "session-token"})
	authorized := httptest.NewRecorder()
	router.ServeHTTP(authorized, authorizedRequest)
	if authorized.Code != http.StatusNotImplemented || !strings.Contains(authorized.Body.String(), `"code":"DEPLOYMENT_FEATURE_UNAVAILABLE"`) {
		t.Fatalf("unwired deployment request list = %d %s", authorized.Code, authorized.Body.String())
	}

	requestID, versionID := uuid.New(), uuid.New()
	mutation := httptest.NewRequest(http.MethodPost, "/api/v1/deployment-requests/"+requestID.String()+"/versions/"+versionID.String()+"/transitions", strings.NewReader(`{"transitionKey":"deploy","expectedVersion":1}`))
	mutation.AddCookie(&http.Cookie{Name: "releasehub_session", Value: "session-token"})
	mutation.Header.Set("Content-Type", "application/json")
	mutation.Header.Set("Origin", "https://releasehub.example")
	mutation.Header.Set("X-CSRF-Token", "csrf-token")
	mutation.Header.Set("Idempotency-Key", "transition-1")
	mutationResponse := httptest.NewRecorder()
	router.ServeHTTP(mutationResponse, mutation)
	if mutationResponse.Code != http.StatusNotImplemented || !strings.Contains(mutationResponse.Body.String(), `"code":"DEPLOYMENT_FEATURE_UNAVAILABLE"`) {
		t.Fatalf("unwired deployment transition = %d %s", mutationResponse.Code, mutationResponse.Body.String())
	}
}

func TestCatalogApplicationEndpointsRequireSessionAndPreserveEmptyUnauthorizedList(t *testing.T) {
	service := &fakeCatalogService{}
	router, _, _, _, _ := newTestRouterWithCatalog(t, &fakeAuthFlow{}, service)
	organizationID, projectID, environmentID := uuid.New(), uuid.New(), uuid.New()
	unauthenticated := httptest.NewRecorder()
	router.ServeHTTP(unauthenticated, httptest.NewRequest(http.MethodGet, "/api/v1/catalog/applications?organizationId="+organizationID.String()+"&projectId="+projectID.String()+"&environmentId="+environmentID.String(), nil))
	if unauthenticated.Code != http.StatusUnauthorized {
		t.Fatalf("unauthenticated list status = %d", unauthenticated.Code)
	}

	response := httptest.NewRecorder()
	request := httptest.NewRequest(http.MethodGet, "/api/v1/catalog/applications?organizationId="+organizationID.String()+"&projectId="+projectID.String()+"&environmentId="+environmentID.String(), nil)
	request.AddCookie(&http.Cookie{Name: "releasehub_session", Value: "session-token"})
	router.ServeHTTP(response, request)
	if response.Code != http.StatusOK || !strings.Contains(response.Body.String(), `"data":[]`) || service.listCalls != 1 {
		t.Fatalf("authorized catalog list = %d %s calls=%d", response.Code, response.Body.String(), service.listCalls)
	}
}

func TestVisibleCatalogApplicationsRequireSession(t *testing.T) {
	service := &fakeCatalogService{}
	router, _, _, _, _ := newTestRouterWithCatalog(t, &fakeAuthFlow{}, service)
	response := httptest.NewRecorder()
	request := httptest.NewRequest(http.MethodGet, "/api/v1/catalog/visible-applications", nil)
	request.AddCookie(&http.Cookie{Name: "releasehub_session", Value: "session-token"})
	router.ServeHTTP(response, request)
	if response.Code != http.StatusOK || !strings.Contains(response.Body.String(), `"data":[]`) || service.visibleCalls != 1 {
		t.Fatalf("visible catalog list = %d %s calls=%d", response.Code, response.Body.String(), service.visibleCalls)
	}
}

func TestCatalogResourceTreeRequiresSessionAndPreservesAuthorizedAncestors(t *testing.T) {
	organizationID, projectID, environmentID := uuid.New(), uuid.New(), uuid.New()
	service := &fakeCatalogService{tree: catalogapp.ResourceTree{Organizations: []catalogapp.OrganizationNode{{
		Organization: catalog.Organization{ID: organizationID, Name: "Tenant A", Version: 2}, IsDefault: false, CanRename: true, CanDelete: true,
		Projects: []catalogapp.ProjectNode{{Project: catalog.Project{ID: projectID, OrganizationID: organizationID, Name: "Payment"}, Environments: []catalogapp.EnvironmentNode{{
			Environment:  catalog.Environment{ID: environmentID, OrganizationID: organizationID, ProjectID: projectID, Name: "production", Type: catalog.EnvironmentProduction},
			Applications: []catalog.Application{},
		}}}},
	}}}}
	router, _, _, _, _ := newTestRouterWithCatalog(t, &fakeAuthFlow{}, service)

	response := httptest.NewRecorder()
	request := httptest.NewRequest(http.MethodGet, "/api/v1/catalog/resource-tree", nil)
	request.AddCookie(&http.Cookie{Name: "releasehub_session", Value: "session-token"})
	router.ServeHTTP(response, request)
	body := response.Body.String()
	if response.Code != http.StatusOK || service.treeCalls != 1 || !strings.Contains(body, `"name":"production"`) || !strings.Contains(body, `"version":2`) || !strings.Contains(body, `"isDefault":false`) || !strings.Contains(body, `"canRename":true`) || !strings.Contains(body, `"canDelete":true`) {
		t.Fatalf("resource tree = %d %s calls=%d", response.Code, response.Body.String(), service.treeCalls)
	}
}

func TestCatalogResourceTreeFailureKeepsResponseGenericAndLogsCause(t *testing.T) {
	service := &fakeCatalogService{treeError: errors.New("database unavailable")}
	router, recorded, _, _, _ := newTestRouterWithCatalog(t, &fakeAuthFlow{}, service)
	request := httptest.NewRequest(http.MethodGet, "/api/v1/catalog/resource-tree", nil)
	request.AddCookie(&http.Cookie{Name: "releasehub_session", Value: "session-secret"})
	response := httptest.NewRecorder()

	router.ServeHTTP(response, request)

	if response.Code != http.StatusInternalServerError || !strings.Contains(response.Body.String(), `"code":"CATALOG_READ_FAILED"`) || strings.Contains(response.Body.String(), "database unavailable") {
		t.Fatalf("resource tree failure = %d %s", response.Code, response.Body.String())
	}
	entry := recorded.FilterMessage("HTTP request failed").All()[0]
	loggedError, _ := entry.ContextMap()["error"].(string)
	if !strings.Contains(loggedError, "list catalog resource tree: database unavailable") || strings.Contains(loggedError, "session-secret") {
		t.Fatalf("logged error = %q", loggedError)
	}
}

func TestCatalogEnvironmentCreationUsesMutationValidation(t *testing.T) {
	service := &fakeCatalogService{}
	router, _, _, _, _ := newTestRouterWithCatalog(t, &fakeAuthFlow{}, service)
	organizationID, projectID := uuid.New(), uuid.New()
	request := httptest.NewRequest(http.MethodPost, "/api/v1/catalog/organizations/"+organizationID.String()+"/projects/"+projectID.String()+"/environments", strings.NewReader(`{"name":"uat-tw","type":"Testing"}`))
	request.AddCookie(&http.Cookie{Name: "releasehub_session", Value: "session-token"})
	request.Header.Set("Content-Type", "application/json")
	request.Header.Set("Origin", "https://releasehub.example")
	request.Header.Set("X-CSRF-Token", "csrf-token")
	response := httptest.NewRecorder()
	router.ServeHTTP(response, request)
	if response.Code != http.StatusCreated || service.environmentCreates != 1 || !strings.Contains(response.Body.String(), `"name":"uat-tw"`) {
		t.Fatalf("create Environment = %d %s calls=%d", response.Code, response.Body.String(), service.environmentCreates)
	}
}

func TestCatalogOrganizationRenameUsesVersionedMutation(t *testing.T) {
	organizationID := uuid.New()
	service := &fakeCatalogService{}
	router, _, _, _, _ := newTestRouterWithCatalog(t, &fakeAuthFlow{}, service)
	request := httptest.NewRequest(http.MethodPatch, "/api/v1/catalog/organizations/"+organizationID.String(), strings.NewReader(`{"name":"Platform","version":2}`))
	request.AddCookie(&http.Cookie{Name: "releasehub_session", Value: "session-token"})
	request.Header.Set("Content-Type", "application/json")
	request.Header.Set("Origin", "https://releasehub.example")
	request.Header.Set("X-CSRF-Token", "csrf-token")
	response := httptest.NewRecorder()

	router.ServeHTTP(response, request)

	if response.Code != http.StatusOK || service.organizationRenames != 1 || service.renameID != organizationID || service.renameName != "Platform" || service.renameVersion != 2 {
		t.Fatalf("rename Organization = %d %s calls=%d input=%s/%q/%d", response.Code, response.Body.String(), service.organizationRenames, service.renameID, service.renameName, service.renameVersion)
	}
	if !strings.Contains(response.Body.String(), `"name":"Platform"`) || !strings.Contains(response.Body.String(), `"version":3`) {
		t.Fatalf("rename Organization response = %s", response.Body.String())
	}
}

func TestCatalogOrganizationRenameMapsValidationAndConflictErrors(t *testing.T) {
	organizationID := uuid.New()
	tests := []struct {
		name       string
		body       string
		serviceErr error
		status     int
		code       string
	}{
		{name: "invalid version", body: `{"name":"Platform","version":0}`, status: http.StatusBadRequest, code: "INVALID_REQUEST"},
		{name: "invalid name", body: `{"name":" ","version":1}`, serviceErr: catalogapp.ErrInvalidResource, status: http.StatusBadRequest, code: "INVALID_REQUEST"},
		{name: "hidden resource", body: `{"name":"Platform","version":1}`, serviceErr: catalogapp.ErrResourceNotFound, status: http.StatusNotFound, code: "RESOURCE_NOT_FOUND"},
		{name: "conflict", body: `{"name":"Platform","version":1}`, serviceErr: catalogapp.ErrResourceConflict, status: http.StatusConflict, code: "RESOURCE_MUTATION_CONFLICT"},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			service := &fakeCatalogService{renameError: test.serviceErr}
			router, _, _, _, _ := newTestRouterWithCatalog(t, &fakeAuthFlow{}, service)
			request := httptest.NewRequest(http.MethodPatch, "/api/v1/catalog/organizations/"+organizationID.String(), strings.NewReader(test.body))
			request.AddCookie(&http.Cookie{Name: "releasehub_session", Value: "session-token"})
			request.Header.Set("Content-Type", "application/json")
			request.Header.Set("Origin", "https://releasehub.example")
			request.Header.Set("X-CSRF-Token", "csrf-token")
			response := httptest.NewRecorder()
			router.ServeHTTP(response, request)
			if response.Code != test.status || !strings.Contains(response.Body.String(), `"code":"`+test.code+`"`) {
				t.Fatalf("rename error = %d %s", response.Code, response.Body.String())
			}
		})
	}
}

func TestCatalogOrganizationDeleteUsesVersionedMutation(t *testing.T) {
	organizationID := uuid.New()
	service := &fakeCatalogService{}
	router, _, _, _, _ := newTestRouterWithCatalog(t, &fakeAuthFlow{}, service)
	request := httptest.NewRequest(http.MethodDelete, "/api/v1/catalog/organizations/"+organizationID.String()+"?expectedVersion=2", nil)
	request.AddCookie(&http.Cookie{Name: "releasehub_session", Value: "session-token"})
	request.Header.Set("Origin", "https://releasehub.example")
	request.Header.Set("X-CSRF-Token", "csrf-token")
	response := httptest.NewRecorder()

	router.ServeHTTP(response, request)

	if response.Code != http.StatusNoContent || service.organizationDeletes != 1 || service.deleteID != organizationID || service.deleteVersion != 2 {
		t.Fatalf("delete Organization = %d %s calls=%d input=%s/%d", response.Code, response.Body.String(), service.organizationDeletes, service.deleteID, service.deleteVersion)
	}
}

func TestCatalogOrganizationDeleteMapsValidationAndConflictErrors(t *testing.T) {
	organizationID := uuid.New()
	tests := []struct {
		name       string
		query      string
		serviceErr error
		status     int
		code       string
	}{
		{name: "invalid version", query: "?expectedVersion=0", status: http.StatusBadRequest, code: "INVALID_REQUEST"},
		{name: "hidden resource", query: "?expectedVersion=1", serviceErr: catalogapp.ErrResourceNotFound, status: http.StatusNotFound, code: "RESOURCE_NOT_FOUND"},
		{name: "conflict", query: "?expectedVersion=1", serviceErr: catalogapp.ErrResourceConflict, status: http.StatusConflict, code: "RESOURCE_MUTATION_CONFLICT"},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			service := &fakeCatalogService{deleteError: test.serviceErr}
			router, _, _, _, _ := newTestRouterWithCatalog(t, &fakeAuthFlow{}, service)
			request := httptest.NewRequest(http.MethodDelete, "/api/v1/catalog/organizations/"+organizationID.String()+test.query, nil)
			request.AddCookie(&http.Cookie{Name: "releasehub_session", Value: "session-token"})
			request.Header.Set("Origin", "https://releasehub.example")
			request.Header.Set("X-CSRF-Token", "csrf-token")
			response := httptest.NewRecorder()
			router.ServeHTTP(response, request)
			if response.Code != test.status || !strings.Contains(response.Body.String(), `"code":"`+test.code+`"`) {
				t.Fatalf("delete error = %d %s", response.Code, response.Body.String())
			}
		})
	}
}

func TestCandidateAssignmentScopesRequireSessionAndReturnProductionTargets(t *testing.T) {
	service := &fakeCandidateService{scopes: []argoapp.AssignmentScope{{OrganizationID: uuid.New(), OrganizationName: "Tenant A", ProjectID: uuid.New(), ProjectName: "Payment", EnvironmentID: uuid.New(), EnvironmentName: "Production"}}}
	router := newTestRouterWithCandidates(t, &fakeAuthFlow{}, service)

	unauthenticated := httptest.NewRecorder()
	router.ServeHTTP(unauthenticated, httptest.NewRequest(http.MethodGet, "/api/v1/argocd/candidate-assignment-scopes", nil))
	if unauthenticated.Code != http.StatusUnauthorized || service.scopeCalls != 0 {
		t.Fatalf("unauthenticated scopes = %d, calls = %d", unauthenticated.Code, service.scopeCalls)
	}

	response := httptest.NewRecorder()
	request := httptest.NewRequest(http.MethodGet, "/api/v1/argocd/candidate-assignment-scopes", nil)
	request.AddCookie(&http.Cookie{Name: "releasehub_session", Value: "session-token"})
	router.ServeHTTP(response, request)
	if response.Code != http.StatusOK || service.scopeCalls != 1 || !strings.Contains(response.Body.String(), `"environmentName":"Production"`) {
		t.Fatalf("assignment scopes = %d %s calls=%d", response.Code, response.Body.String(), service.scopeCalls)
	}
}

func TestAccessManagementSnapshotRequiresSessionAndReturnsFilteredRecords(t *testing.T) {
	service := &fakeAccessManagementService{value: authzapp.AccessSnapshot{Users: []authzapp.AccessUser{{ID: uuid.New(), Username: "vincent"}}, Groups: []authzapp.AccessGroup{}, Roles: []authzapp.AccessRole{}, Memberships: []authzapp.AccessMembership{}, Bindings: []authzapp.AccessBinding{}, Denies: []authzapp.AccessDeny{}}}
	router := newTestRouterWithAccess(t, &fakeAuthFlow{}, service)

	response := httptest.NewRecorder()
	request := httptest.NewRequest(http.MethodGet, "/api/v1/access/snapshot", nil)
	request.AddCookie(&http.Cookie{Name: "releasehub_session", Value: "session-token"})
	router.ServeHTTP(response, request)
	if response.Code != http.StatusOK || service.calls != 1 || !strings.Contains(response.Body.String(), `"username":"vincent"`) {
		t.Fatalf("access snapshot = %d %s calls=%d", response.Code, response.Body.String(), service.calls)
	}
}

func TestMembershipCandidatesAllowEmptySearchAndReturnCursorMetadata(t *testing.T) {
	groupID := uuid.New()
	service := &fakeAccessManagementService{candidatePage: authzapp.AccessListPage[authzapp.AccessUser]{
		Items:      []authzapp.AccessUser{{ID: uuid.New(), Username: "amy"}},
		NextCursor: "next-cursor",
		HasMore:    true,
	}}
	router := newTestRouterWithAccess(t, &fakeAuthFlow{}, service)
	request := httptest.NewRequest(http.MethodGet, "/api/v1/access/groups/"+groupID.String()+"/membership-candidates", nil)
	request.AddCookie(&http.Cookie{Name: "releasehub_session", Value: "session-token"})
	response := httptest.NewRecorder()

	router.ServeHTTP(response, request)

	if response.Code != http.StatusOK || service.candidateQuery != "" || service.candidateCursor != "" {
		t.Fatalf("membership candidates = %d %s query=%q cursor=%q", response.Code, response.Body.String(), service.candidateQuery, service.candidateCursor)
	}
	if !strings.Contains(response.Body.String(), `"username":"amy"`) || !strings.Contains(response.Body.String(), `"nextCursor":"next-cursor"`) || !strings.Contains(response.Body.String(), `"hasMore":true`) {
		t.Fatalf("membership candidate response = %s", response.Body.String())
	}
}

func TestAccessManagementMutationRequiresCSRFAndReturnsCreatedRecord(t *testing.T) {
	service := &fakeAccessManagementService{}
	router := newTestRouterWithAccess(t, &fakeAuthFlow{}, service)
	body := `{"ownerKind":"platform","name":"operators","oidcViewerOnly":false}`

	rejected := httptest.NewRequest(http.MethodPost, "/api/v1/access/groups", strings.NewReader(body))
	rejected.AddCookie(&http.Cookie{Name: "releasehub_session", Value: "session-token"})
	rejected.Header.Set("Content-Type", "application/json")
	rejected.Header.Set("Origin", "https://attacker.example")
	rejected.Header.Set("X-CSRF-Token", "csrf-token")
	rejectedResponse := httptest.NewRecorder()
	router.ServeHTTP(rejectedResponse, rejected)
	if rejectedResponse.Code != http.StatusForbidden || service.mutationCalls != 0 {
		t.Fatalf("cross-origin mutation = %d calls=%d", rejectedResponse.Code, service.mutationCalls)
	}

	request := httptest.NewRequest(http.MethodPost, "/api/v1/access/groups", strings.NewReader(body))
	request.AddCookie(&http.Cookie{Name: "releasehub_session", Value: "session-token"})
	request.Header.Set("Content-Type", "application/json")
	request.Header.Set("Origin", "https://releasehub.example")
	request.Header.Set("X-CSRF-Token", "csrf-token")
	response := httptest.NewRecorder()
	router.ServeHTTP(response, request)
	if response.Code != http.StatusCreated || service.mutationCalls != 1 || !strings.Contains(response.Body.String(), `"name":"operators"`) {
		t.Fatalf("create Group = %d %s calls=%d", response.Code, response.Body.String(), service.mutationCalls)
	}
}

func TestCreateAccessUserMapsStableValidationAndConflictErrors(t *testing.T) {
	tests := []struct {
		name       string
		serviceErr error
		status     int
		code       string
	}{
		{name: "invalid", serviceErr: identityapp.ErrInvalidLocalUser, status: http.StatusBadRequest, code: "INVALID_REQUEST"},
		{name: "conflict", serviceErr: identityapp.ErrLocalUsernameConflict, status: http.StatusConflict, code: "ACCESS_USERNAME_CONFLICT"},
		{name: "internal", serviceErr: context.DeadlineExceeded, status: http.StatusInternalServerError, code: "ACCESS_USER_CREATE_FAILED"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			access := &fakeAccessManagementService{capabilities: authzapp.AccessCapabilities{Collections: []authzapp.AccessCollectionCapability{{Key: "users", Visible: true, CanCreate: true}}}}
			local := &fakeLocalUserCreator{err: tt.serviceErr}
			router := newTestRouterWithAccessAndLocal(t, &fakeAuthFlow{}, access, local)
			request := httptest.NewRequest(http.MethodPost, "/api/v1/access/users", strings.NewReader(`{"username":"operator","initialPassword":"initial-password"}`))
			request.AddCookie(&http.Cookie{Name: "releasehub_session", Value: "session-token"})
			request.Header.Set("Content-Type", "application/json")
			request.Header.Set("Origin", "https://releasehub.example")
			request.Header.Set("X-CSRF-Token", "csrf-token")
			response := httptest.NewRecorder()

			router.ServeHTTP(response, request)

			if response.Code != tt.status || !strings.Contains(response.Body.String(), `"code":"`+tt.code+`"`) {
				t.Fatalf("create local user = %d %s", response.Code, response.Body.String())
			}
		})
	}
}

func TestApplicationOnboardingEndpointsUseMutationValidation(t *testing.T) {
	service := &fakeOnboardingService{state: argodomain.Onboarding{ApplicationID: uuid.New(), Status: argodomain.OnboardingAwaitingConfirmation, Version: 3}}
	flow := &fakeAuthFlow{}
	router := newTestRouterWithOnboarding(t, flow, service)
	dryRun := httptest.NewRequest(http.MethodPost, "/api/v1/catalog/applications/"+service.state.ApplicationID.String()+"/onboarding/dry-run", nil)
	dryRun.AddCookie(&http.Cookie{Name: "releasehub_session", Value: "session-token"})
	dryRun.Header.Set("Origin", "https://releasehub.example")
	dryRun.Header.Set("X-CSRF-Token", "csrf-token")
	dryRunResponse := httptest.NewRecorder()
	router.ServeHTTP(dryRunResponse, dryRun)
	if dryRunResponse.Code != http.StatusOK || service.dryRuns != 1 {
		t.Fatalf("dry-run = %d %s", dryRunResponse.Code, dryRunResponse.Body.String())
	}

	confirm := httptest.NewRequest(http.MethodPost, "/api/v1/catalog/applications/"+service.state.ApplicationID.String()+"/onboarding/confirm", strings.NewReader(`{"expectedVersion":3}`))
	confirm.AddCookie(&http.Cookie{Name: "releasehub_session", Value: "session-token"})
	confirm.Header.Set("Content-Type", "application/json")
	confirm.Header.Set("Origin", "https://releasehub.example")
	confirm.Header.Set("X-CSRF-Token", "csrf-token")
	confirm.Header.Set("Idempotency-Key", "confirm-key")
	confirmResponse := httptest.NewRecorder()
	router.ServeHTTP(confirmResponse, confirm)
	if confirmResponse.Code != http.StatusOK || service.confirms != 1 {
		t.Fatalf("confirm = %d %s", confirmResponse.Code, confirmResponse.Body.String())
	}
}

func TestApplicationOnboardingConfirmReturnsAcceptedWhileCommandIsInProgress(t *testing.T) {
	service := &fakeOnboardingService{state: argodomain.Onboarding{ApplicationID: uuid.New(), Version: 3}, confirmError: argoapp.ErrCommandInProgress}
	router := newTestRouterWithOnboarding(t, &fakeAuthFlow{}, service)
	request := httptest.NewRequest(http.MethodPost, "/api/v1/catalog/applications/"+service.state.ApplicationID.String()+"/onboarding/confirm", strings.NewReader(`{"expectedVersion":3}`))
	request.AddCookie(&http.Cookie{Name: "releasehub_session", Value: "session-token"})
	request.Header.Set("Content-Type", "application/json")
	request.Header.Set("Origin", "https://releasehub.example")
	request.Header.Set("X-CSRF-Token", "csrf-token")
	request.Header.Set("Idempotency-Key", "confirm-key")
	response := httptest.NewRecorder()
	router.ServeHTTP(response, request)
	if response.Code != http.StatusAccepted || service.confirms != 1 {
		t.Fatalf("in-progress confirm = %d %s", response.Code, response.Body.String())
	}
}

func TestApplicationOnboardingRejectsCrossOriginMutation(t *testing.T) {
	service := &fakeOnboardingService{state: argodomain.Onboarding{ApplicationID: uuid.New(), Version: 3}}
	router := newTestRouterWithOnboarding(t, &fakeAuthFlow{}, service)
	request := httptest.NewRequest(http.MethodPost, "/api/v1/catalog/applications/"+service.state.ApplicationID.String()+"/onboarding/dry-run", nil)
	request.AddCookie(&http.Cookie{Name: "releasehub_session", Value: "session-token"})
	request.Header.Set("Origin", "https://attacker.example")
	request.Header.Set("X-CSRF-Token", "csrf-token")
	response := httptest.NewRecorder()
	router.ServeHTTP(response, request)
	if response.Code != http.StatusForbidden || service.dryRuns != 0 {
		t.Fatalf("cross-origin dry-run = %d %s", response.Code, response.Body.String())
	}
}

func TestApplicationOnboardingConfirmReturnsConflictForRejectedCommand(t *testing.T) {
	service := &fakeOnboardingService{state: argodomain.Onboarding{ApplicationID: uuid.New(), Version: 3}, confirmError: argoapp.ErrIdempotencyKeyReuse}
	router := newTestRouterWithOnboarding(t, &fakeAuthFlow{}, service)
	request := httptest.NewRequest(http.MethodPost, "/api/v1/catalog/applications/"+service.state.ApplicationID.String()+"/onboarding/confirm", strings.NewReader(`{"expectedVersion":3}`))
	request.AddCookie(&http.Cookie{Name: "releasehub_session", Value: "session-token"})
	request.Header.Set("Content-Type", "application/json")
	request.Header.Set("Origin", "https://releasehub.example")
	request.Header.Set("X-CSRF-Token", "csrf-token")
	request.Header.Set("Idempotency-Key", "reused-key")
	response := httptest.NewRecorder()
	router.ServeHTTP(response, request)
	if response.Code != http.StatusConflict || service.confirms != 1 {
		t.Fatalf("rejected confirm = %d %s", response.Code, response.Body.String())
	}
}

func TestLoginAndCallbackUseSecureBFFCookies(t *testing.T) {
	flow := &fakeAuthFlow{}
	router, _, _, _, _ := newTestRouterWithAuth(t, flow)
	login := httptest.NewRecorder()
	router.ServeHTTP(login, httptest.NewRequest(http.MethodGet, "/api/v1/auth/login", nil))
	if login.Code != http.StatusFound || login.Header().Get("Location") != "https://issuer.example/authorize" {
		t.Fatalf("unexpected login response: %d %s", login.Code, login.Header().Get("Location"))
	}
	if cookie := login.Header().Get("Set-Cookie"); !strings.Contains(cookie, "releasehub_login=login-state") || !strings.Contains(cookie, "HttpOnly") || !strings.Contains(cookie, "Secure") {
		t.Fatalf("login cookie is not secure: %s", cookie)
	}

	callback := httptest.NewRecorder()
	request := httptest.NewRequest(http.MethodGet, "/api/v1/auth/callback?code=code&state=state", nil)
	request.AddCookie(&http.Cookie{Name: "releasehub_login", Value: "login-state"})
	router.ServeHTTP(callback, request)
	if callback.Code != http.StatusFound || callback.Header().Get("Location") != "https://releasehub.example/" {
		t.Fatalf("unexpected callback response: %d %s", callback.Code, callback.Header().Get("Location"))
	}
	cookies := callback.Header().Values("Set-Cookie")
	joined := strings.Join(cookies, "\n")
	if !strings.Contains(joined, "releasehub_session=session-token") || !strings.Contains(joined, "HttpOnly") {
		t.Fatalf("session cookie is missing: %s", joined)
	}
	if !strings.Contains(joined, "releasehub_csrf=csrf-token") {
		t.Fatalf("CSRF cookie is missing: %s", joined)
	}
}

func TestLogoutRequiresSameOriginAndCSRFThenRevokesSession(t *testing.T) {
	flow := &fakeAuthFlow{}
	router, _, _, _, _ := newTestRouterWithAuth(t, flow)
	rejected := httptest.NewRecorder()
	rejectedRequest := httptest.NewRequest(http.MethodPost, "/api/v1/auth/logout", nil)
	rejectedRequest.AddCookie(&http.Cookie{Name: "releasehub_session", Value: "session-token"})
	rejectedRequest.Header.Set("X-CSRF-Token", "csrf-token")
	rejectedRequest.Header.Set("Origin", "https://attacker.example")
	router.ServeHTTP(rejected, rejectedRequest)
	if rejected.Code != http.StatusForbidden {
		t.Fatalf("expected cross-origin request to fail, got %d", rejected.Code)
	}

	response := httptest.NewRecorder()
	request := httptest.NewRequest(http.MethodPost, "/api/v1/auth/logout", nil)
	request.AddCookie(&http.Cookie{Name: "releasehub_session", Value: "session-token"})
	request.Header.Set("X-CSRF-Token", "csrf-token")
	request.Header.Set("Origin", "https://releasehub.example")
	router.ServeHTTP(response, request)
	if response.Code != http.StatusOK || !flow.loggedOut {
		t.Fatalf("logout failed: %d %s", response.Code, response.Body.String())
	}
	if !strings.Contains(response.Body.String(), "https://issuer.example/logout") {
		t.Fatalf("provider logout URL is missing: %s", response.Body.String())
	}
}

func TestBackchannelLogoutRejectsMissingTokenAndRevokesValidSubject(t *testing.T) {
	flow := &fakeAuthFlow{}
	router, _, _, _, _ := newTestRouterWithAuth(t, flow)
	missing := httptest.NewRecorder()
	router.ServeHTTP(missing, httptest.NewRequest(http.MethodPost, "/api/v1/auth/backchannel-logout", nil))
	if missing.Code != http.StatusBadRequest {
		t.Fatalf("expected missing token to fail, got %d", missing.Code)
	}

	response := httptest.NewRecorder()
	request := httptest.NewRequest(http.MethodPost, "/api/v1/auth/backchannel-logout", strings.NewReader("logout_token=signed-token"))
	request.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	router.ServeHTTP(response, request)
	if response.Code != http.StatusNoContent || !flow.backchannelLoggedOut {
		t.Fatalf("back-channel logout failed: %d", response.Code)
	}
}

func TestSuccessfulProbeDoesNotCreateAccessLogTraceOrRequestMetric(t *testing.T) {
	router, recorded, registry, spans, _ := newTestRouter(t)

	for _, path := range []string{"/healthz", "/readyz", "/metrics"} {
		response := httptest.NewRecorder()
		router.ServeHTTP(response, httptest.NewRequest(http.MethodGet, path, nil))
		if response.Code != http.StatusOK {
			t.Fatalf("%s should succeed, got status %d", path, response.Code)
		}
	}

	if recorded.Len() != 0 {
		t.Fatalf("successful probe should not create access logs, got %d", recorded.Len())
	}
	if len(spans.Ended()) != 0 {
		t.Fatalf("successful probe should not create traces, got %d", len(spans.Ended()))
	}
	if got := requestMetricCount(t, registry); got != 0 {
		t.Fatalf("successful probe should not create request metrics, got %v", got)
	}
}

func TestFailedReadinessCreatesAccessLogTraceAndRequestMetric(t *testing.T) {
	router, recorded, registry, spans, readiness := newTestRouter(t)
	readiness.BeginShutdown()

	response := httptest.NewRecorder()
	router.ServeHTTP(response, httptest.NewRequest(http.MethodGet, "/readyz", nil))

	if response.Code != http.StatusServiceUnavailable {
		t.Fatalf("readiness should be 503 during shutdown, got %d", response.Code)
	}
	if recorded.Len() != 1 {
		t.Fatalf("failed probe should create one access log, got %d", recorded.Len())
	}
	if len(spans.Ended()) != 1 {
		t.Fatalf("failed probe should create one trace, got %d", len(spans.Ended()))
	}
	if got := requestMetricCount(t, registry); got != 1 {
		t.Fatalf("failed probe should create one request metric, got %v", got)
	}
}

func newTestRouter(t *testing.T) (
	http.Handler,
	*observer.ObservedLogs,
	*prometheus.Registry,
	*tracetest.SpanRecorder,
	*httpserver.Readiness,
) {
	return newTestRouterWithAuth(t, nil)
}

func newTestRouterWithAuth(t *testing.T, flow *fakeAuthFlow) (
	http.Handler,
	*observer.ObservedLogs,
	*prometheus.Registry,
	*tracetest.SpanRecorder,
	*httpserver.Readiness,
) {
	t.Helper()
	return newTestRouterFromOptionsWithState(t, testAPIOptions(flow))
}

func newTestRouterFromOptions(t *testing.T, apiOptions httpserver.APIOptions) http.Handler {
	t.Helper()
	router, _, _, _, _ := newTestRouterFromOptionsWithState(t, apiOptions)
	return router
}

func newTestRouterFromOptionsWithState(t *testing.T, apiOptions httpserver.APIOptions) (
	http.Handler,
	*observer.ObservedLogs,
	*prometheus.Registry,
	*tracetest.SpanRecorder,
	*httpserver.Readiness,
) {
	t.Helper()
	core, recorded := observer.New(zap.InfoLevel)
	logger, err := observability.WithComponent(zap.New(core), observability.ComponentAPI)
	if err != nil {
		t.Fatalf("create test logger: %v", err)
	}
	registry := prometheus.NewRegistry()
	metrics, err := observability.NewHTTPMetrics(registry)
	if err != nil {
		t.Fatalf("create test metrics: %v", err)
	}
	spans := tracetest.NewSpanRecorder()
	provider := trace.NewTracerProvider(trace.WithSpanProcessor(spans))
	t.Cleanup(func() { _ = provider.Shutdown(t.Context()) })

	readiness := httpserver.NewReadiness()
	options := httpserver.RouterOptions{
		Logger:         logger,
		Registry:       registry,
		HTTPMetrics:    metrics,
		TracerProvider: provider,
		Readiness:      readiness,
		MetricsPath:    "/metrics",
	}
	router := httpserver.NewRouter(options, httpserver.NewAPIHandler(apiOptions))
	return router, recorded, registry, spans, readiness
}

func newTestRouterWithCatalog(t *testing.T, flow *fakeAuthFlow, catalogService *fakeCatalogService) (
	http.Handler,
	*observer.ObservedLogs,
	*prometheus.Registry,
	*tracetest.SpanRecorder,
	*httpserver.Readiness,
) {
	t.Helper()
	core, recorded := observer.New(zap.InfoLevel)
	logger, err := observability.WithComponent(zap.New(core), observability.ComponentAPI)
	if err != nil {
		t.Fatalf("create test logger: %v", err)
	}
	registry := prometheus.NewRegistry()
	metrics, err := observability.NewHTTPMetrics(registry)
	if err != nil {
		t.Fatalf("create test metrics: %v", err)
	}
	spans := tracetest.NewSpanRecorder()
	provider := trace.NewTracerProvider(trace.WithSpanProcessor(spans))
	t.Cleanup(func() { _ = provider.Shutdown(t.Context()) })
	readiness := httpserver.NewReadiness()
	return httpserver.NewRouter(httpserver.RouterOptions{
		Logger: logger, Registry: registry, HTTPMetrics: metrics, TracerProvider: provider, Readiness: readiness,
		MetricsPath: "/metrics",
	}, httpserver.NewAPIHandler(httpserver.APIOptions{
		System: httpserver.SystemHandlerOptions{Version: "test"},
		Auth:   testAuthOptions(flow),
		Catalog: httpserver.CatalogHandlerOptions{
			Service: catalogService,
		},
	})), recorded, registry, spans, readiness
}

func newTestRouterWithOnboarding(t *testing.T, flow *fakeAuthFlow, service *fakeOnboardingService) http.Handler {
	t.Helper()
	core, _ := observer.New(zap.InfoLevel)
	logger, err := observability.WithComponent(zap.New(core), observability.ComponentAPI)
	if err != nil {
		t.Fatalf("create test logger: %v", err)
	}
	registry := prometheus.NewRegistry()
	metrics, err := observability.NewHTTPMetrics(registry)
	if err != nil {
		t.Fatalf("create metrics: %v", err)
	}
	provider := trace.NewTracerProvider()
	t.Cleanup(func() { _ = provider.Shutdown(t.Context()) })
	options := testAPIOptions(flow)
	options.ArgoCD.Onboarding = service
	return httpserver.NewRouter(httpserver.RouterOptions{Logger: logger, Registry: registry, HTTPMetrics: metrics, TracerProvider: provider, Readiness: httpserver.NewReadiness(), MetricsPath: "/metrics"}, httpserver.NewAPIHandler(options))
}

func newTestRouterWithCandidates(t *testing.T, flow *fakeAuthFlow, service *fakeCandidateService) http.Handler {
	t.Helper()
	core, _ := observer.New(zap.InfoLevel)
	logger, err := observability.WithComponent(zap.New(core), observability.ComponentAPI)
	if err != nil {
		t.Fatalf("create test logger: %v", err)
	}
	registry := prometheus.NewRegistry()
	metrics, err := observability.NewHTTPMetrics(registry)
	if err != nil {
		t.Fatalf("create metrics: %v", err)
	}
	provider := trace.NewTracerProvider()
	t.Cleanup(func() { _ = provider.Shutdown(t.Context()) })
	options := testAPIOptions(flow)
	options.ArgoCD.Candidates = service
	return httpserver.NewRouter(httpserver.RouterOptions{Logger: logger, Registry: registry, HTTPMetrics: metrics, TracerProvider: provider, Readiness: httpserver.NewReadiness(), MetricsPath: "/metrics"}, httpserver.NewAPIHandler(options))
}

func newTestRouterWithAccess(t *testing.T, flow *fakeAuthFlow, service *fakeAccessManagementService) http.Handler {
	return newTestRouterWithAccessAndLocal(t, flow, service, nil)
}

func newTestRouterWithAccessAndLocal(t *testing.T, flow *fakeAuthFlow, service *fakeAccessManagementService, local *fakeLocalUserCreator) http.Handler {
	t.Helper()
	core, _ := observer.New(zap.InfoLevel)
	logger, err := observability.WithComponent(zap.New(core), observability.ComponentAPI)
	if err != nil {
		t.Fatalf("create test logger: %v", err)
	}
	registry := prometheus.NewRegistry()
	metrics, err := observability.NewHTTPMetrics(registry)
	if err != nil {
		t.Fatalf("create metrics: %v", err)
	}
	provider := trace.NewTracerProvider()
	t.Cleanup(func() { _ = provider.Shutdown(t.Context()) })
	options := testAPIOptions(flow)
	options.Access.Service = service
	options.Access.LocalUsers = local
	return httpserver.NewRouter(httpserver.RouterOptions{Logger: logger, Registry: registry, HTTPMetrics: metrics, TracerProvider: provider, Readiness: httpserver.NewReadiness(), MetricsPath: "/metrics"}, httpserver.NewAPIHandler(options))
}

func testAPIOptions(flow *fakeAuthFlow) httpserver.APIOptions {
	return httpserver.APIOptions{System: httpserver.SystemHandlerOptions{Version: "test"}, Auth: testAuthOptions(flow)}
}

func testAuthOptions(flow *fakeAuthFlow) httpserver.AuthHandlerOptions {
	return httpserver.AuthHandlerOptions{Flow: flow, WebRedirectURL: "https://releasehub.example/", CookieSecure: true, LoginStateTTL: time.Minute, SessionTTL: time.Hour}
}

type fakeAuthFlow struct {
	loggedOut, backchannelLoggedOut bool
	session                         identity.Session
	user                            identity.User
	authenticateErr                 error
	mutationErr                     error
}

func (*fakeAuthFlow) Begin() (string, string, error) {
	return "login-state", "https://issuer.example/authorize", nil
}
func (*fakeAuthFlow) Complete(context.Context, string, string, string) (identity.User, string, string, error) {
	return identity.User{ID: uuid.New(), Username: "vincent"}, "session-token", "csrf-token", nil
}
func (f *fakeAuthFlow) Authenticate(context.Context, string) (identity.Session, identity.User, error) {
	if f.authenticateErr != nil {
		return identity.Session{}, identity.User{}, f.authenticateErr
	}
	if f.session.ID != uuid.Nil {
		return f.session, f.user, nil
	}
	return identity.Session{ID: uuid.New(), UserID: uuid.New(), AuthenticationMethod: "oidc"}, identity.User{ID: uuid.New(), Username: "vincent"}, nil
}

func (f *fakeAuthFlow) ValidateMutation(ctx context.Context, token, _ string) (identity.Session, identity.User, error) {
	if f.mutationErr != nil {
		return identity.Session{}, identity.User{}, f.mutationErr
	}
	return f.Authenticate(ctx, token)
}
func (f *fakeAuthFlow) Logout(context.Context, string, string) (string, error) {
	f.loggedOut = true
	return "https://issuer.example/logout", nil
}
func (f *fakeAuthFlow) BackchannelLogout(context.Context, string) error {
	f.backchannelLoggedOut = true
	return nil
}

type fakeLocalAuth struct {
	user                    identity.User
	mustChange              bool
	loginCalls, changeCalls int
}

func (f *fakeLocalAuth) Login(context.Context, string, string) (identity.User, bool, identity.Session, string, string, error) {
	f.loginCalls++
	return f.user, f.mustChange, identity.Session{
		IdleExpiresAt:     time.Date(2026, time.September, 14, 8, 30, 0, 0, time.UTC),
		AbsoluteExpiresAt: time.Date(2026, time.September, 14, 20, 0, 0, 0, time.UTC),
	}, "local-session-token", "local-csrf-token", nil
}

func (f *fakeLocalAuth) ChangePassword(context.Context, uuid.UUID, string, string) error {
	f.changeCalls++
	f.mustChange = false
	return nil
}

func (f *fakeLocalAuth) MustChangePassword(context.Context, uuid.UUID) (bool, error) {
	return f.mustChange, nil
}

type fakeCatalogService struct {
	listCalls, visibleCalls, treeCalls, environmentCreates, organizationRenames, organizationDeletes int
	tree                                                                                             catalogapp.ResourceTree
	renameID                                                                                         uuid.UUID
	renameName                                                                                       string
	renameVersion                                                                                    uint64
	renameError                                                                                      error
	deleteID                                                                                         uuid.UUID
	deleteVersion                                                                                    uint64
	deleteError                                                                                      error
	treeError                                                                                        error
}

func (*fakeCatalogService) CreateOrganization(_ context.Context, _ catalogapp.Principal, _ catalogapp.Mutation, name string) (catalog.Organization, error) {
	return catalog.Organization{ID: uuid.New(), Name: name, Active: true, Version: 1}, nil
}

func (s *fakeCatalogService) RenameOrganization(_ context.Context, _ catalogapp.Principal, _ catalogapp.Mutation, organizationID uuid.UUID, name string, expectedVersion uint64) (catalog.Organization, error) {
	s.organizationRenames++
	s.renameID, s.renameName, s.renameVersion = organizationID, name, expectedVersion
	if s.renameError != nil {
		return catalog.Organization{}, s.renameError
	}
	return catalog.Organization{ID: organizationID, Name: name, Active: true, Version: expectedVersion + 1}, nil
}

func (s *fakeCatalogService) DeleteOrganization(_ context.Context, _ catalogapp.Principal, _ catalogapp.Mutation, organizationID uuid.UUID, expectedVersion uint64) error {
	s.organizationDeletes++
	s.deleteID, s.deleteVersion = organizationID, expectedVersion
	return s.deleteError
}

func (*fakeCatalogService) CreateProject(_ context.Context, _ catalogapp.Principal, _ catalogapp.Mutation, organizationID uuid.UUID, name string) (catalog.Project, error) {
	return catalog.Project{ID: uuid.New(), OrganizationID: organizationID, Name: name, Active: true, Version: 1}, nil
}

func (s *fakeCatalogService) CreateEnvironment(_ context.Context, _ catalogapp.Principal, _ catalogapp.Mutation, organizationID, projectID uuid.UUID, name string, environmentType catalog.EnvironmentType) (catalog.Environment, error) {
	s.environmentCreates++
	return catalog.Environment{ID: uuid.New(), OrganizationID: organizationID, ProjectID: projectID, Name: name, Type: environmentType, Active: true, Version: 1}, nil
}

func (*fakeCatalogService) CreateEnvironmentLabelMapping(_ context.Context, _ catalogapp.Principal, _ catalogapp.Mutation, organizationID, projectID, environmentID uuid.UUID, key, value string) (catalog.EnvironmentLabelMapping, error) {
	return catalog.EnvironmentLabelMapping{ID: uuid.New(), OrganizationID: organizationID, ProjectID: projectID, EnvironmentID: environmentID, LabelKey: key, LabelValue: value}, nil
}

type fakeOnboardingService struct {
	state             argodomain.Onboarding
	dryRuns, confirms int
	confirmError      error
}

type fakeCandidateService struct {
	scopes     []argoapp.AssignmentScope
	scopeCalls int
}

type fakeAccessManagementService struct {
	value                           authzapp.AccessSnapshot
	capabilities                    authzapp.AccessCapabilities
	candidatePage                   authzapp.AccessListPage[authzapp.AccessUser]
	candidateQuery, candidateCursor string
	calls                           int
	mutationCalls                   int
}

func (s *fakeAccessManagementService) Load(context.Context, authzapp.AccessPrincipal) (authzapp.AccessSnapshot, error) {
	s.calls++
	return s.value, nil
}
func (s *fakeAccessManagementService) Capabilities(context.Context, authzapp.AccessPrincipal) (authzapp.AccessCapabilities, error) {
	return s.capabilities, nil
}

type fakeLocalUserCreator struct{ err error }

func (f *fakeLocalUserCreator) CreateUser(context.Context, uuid.UUID, string, string, string) (identity.User, error) {
	if f.err != nil {
		return identity.User{}, f.err
	}
	return identity.User{ID: uuid.New(), Username: "operator"}, nil
}
func (*fakeAccessManagementService) ListUsers(context.Context, authzapp.AccessPrincipal, string, string, string, int) (authzapp.AccessListPage[authzapp.AccessUser], error) {
	return authzapp.AccessListPage[authzapp.AccessUser]{Items: []authzapp.AccessUser{}}, nil
}
func (*fakeAccessManagementService) ListGroups(context.Context, authzapp.AccessPrincipal, string, string, string, int) (authzapp.AccessListPage[authzapp.AccessGroup], error) {
	return authzapp.AccessListPage[authzapp.AccessGroup]{Items: []authzapp.AccessGroup{}}, nil
}
func (*fakeAccessManagementService) ListRoles(context.Context, authzapp.AccessPrincipal, string, string, string, int) (authzapp.AccessListPage[authzapp.AccessRole], error) {
	return authzapp.AccessListPage[authzapp.AccessRole]{Items: []authzapp.AccessRole{}}, nil
}
func (*fakeAccessManagementService) ListMemberships(context.Context, authzapp.AccessPrincipal, *uuid.UUID, *uuid.UUID, string, string, int) (authzapp.AccessListPage[authzapp.AccessMembership], error) {
	return authzapp.AccessListPage[authzapp.AccessMembership]{Items: []authzapp.AccessMembership{}}, nil
}
func (*fakeAccessManagementService) ListBindings(context.Context, authzapp.AccessPrincipal, string, string, int) (authzapp.AccessListPage[authzapp.AccessBinding], error) {
	return authzapp.AccessListPage[authzapp.AccessBinding]{Items: []authzapp.AccessBinding{}}, nil
}
func (*fakeAccessManagementService) ListDenies(context.Context, authzapp.AccessPrincipal, string, string, int) (authzapp.AccessListPage[authzapp.AccessDeny], error) {
	return authzapp.AccessListPage[authzapp.AccessDeny]{Items: []authzapp.AccessDeny{}}, nil
}
func (s *fakeAccessManagementService) MembershipCandidates(_ context.Context, _ authzapp.AccessPrincipal, _ uuid.UUID, query, cursor string, _ int) (authzapp.AccessListPage[authzapp.AccessUser], error) {
	s.candidateQuery = query
	s.candidateCursor = cursor
	if s.candidatePage.Items == nil {
		return authzapp.AccessListPage[authzapp.AccessUser]{Items: []authzapp.AccessUser{}}, nil
	}
	return s.candidatePage, nil
}
func (*fakeAccessManagementService) ScopeOptions(context.Context, authzapp.AccessPrincipal, string, string) (authzapp.AccessScopeOptions, error) {
	return authzapp.AccessScopeOptions{Groups: []authzapp.AccessGroup{}, Roles: []authzapp.AccessRole{}, Permissions: []authzapp.AccessPermission{}}, nil
}

func (s *fakeAccessManagementService) CreateGroup(_ context.Context, _ authzapp.AccessPrincipal, _ authzapp.AccessMutation, input authzapp.CreateGroupInput) (authzapp.AccessGroup, error) {
	s.mutationCalls++
	return authzapp.AccessGroup{ID: uuid.New(), OwnerKind: input.OwnerKind, OwnerID: input.OwnerID, Name: input.Name, OIDCViewerOnly: input.OIDCViewerOnly}, nil
}
func (*fakeAccessManagementService) DisableGroup(context.Context, authzapp.AccessPrincipal, authzapp.AccessMutation, uuid.UUID) error {
	return nil
}
func (*fakeAccessManagementService) CreateRole(context.Context, authzapp.AccessPrincipal, authzapp.AccessMutation, authzapp.CreateRoleInput) (authzapp.AccessRole, error) {
	return authzapp.AccessRole{}, nil
}
func (*fakeAccessManagementService) AddMembership(context.Context, authzapp.AccessPrincipal, authzapp.AccessMutation, uuid.UUID, uuid.UUID) (authzapp.AccessMembership, error) {
	return authzapp.AccessMembership{}, nil
}
func (*fakeAccessManagementService) CreateBinding(context.Context, authzapp.AccessPrincipal, authzapp.AccessMutation, authzapp.CreateBindingInput) (authzapp.AccessBinding, error) {
	return authzapp.AccessBinding{}, nil
}
func (*fakeAccessManagementService) CreateDeny(context.Context, authzapp.AccessPrincipal, authzapp.AccessMutation, authzapp.CreateDenyInput) (authzapp.AccessDeny, error) {
	return authzapp.AccessDeny{}, nil
}
func (*fakeAccessManagementService) DisableUser(context.Context, authzapp.AccessPrincipal, authzapp.AccessMutation, uuid.UUID) error {
	return nil
}
func (*fakeAccessManagementService) RevokeMembership(context.Context, authzapp.AccessPrincipal, authzapp.AccessMutation, uuid.UUID) error {
	return nil
}
func (*fakeAccessManagementService) DisableRole(context.Context, authzapp.AccessPrincipal, authzapp.AccessMutation, uuid.UUID) error {
	return nil
}
func (*fakeAccessManagementService) RevokeBinding(context.Context, authzapp.AccessPrincipal, authzapp.AccessMutation, uuid.UUID) error {
	return nil
}
func (*fakeAccessManagementService) RevokeDeny(context.Context, authzapp.AccessPrincipal, authzapp.AccessMutation, uuid.UUID) error {
	return nil
}

func (*fakeCandidateService) List(context.Context, argoapp.OnboardingPrincipal) ([]argoapp.Candidate, error) {
	return []argoapp.Candidate{}, nil
}

func (s *fakeCandidateService) ListAssignmentScopes(context.Context, argoapp.OnboardingPrincipal) ([]argoapp.AssignmentScope, error) {
	s.scopeCalls++
	return s.scopes, nil
}

func (*fakeCandidateService) Assign(context.Context, argoapp.OnboardingPrincipal, argoapp.AssignCandidateInput) (catalog.Application, error) {
	return catalog.Application{}, nil
}

func (s *fakeOnboardingService) DryRun(_ context.Context, _ argoapp.OnboardingPrincipal, _ argoapp.OnboardingMutation, _ uuid.UUID) (argodomain.Onboarding, error) {
	s.dryRuns++
	return s.state, nil
}
func (s *fakeOnboardingService) ConfirmIdempotent(_ context.Context, _ argoapp.OnboardingPrincipal, _ argoapp.OnboardingMutation, _ uuid.UUID, _ uint64, _ string) (argodomain.Onboarding, error) {
	s.confirms++
	if s.confirmError != nil {
		return argodomain.Onboarding{}, s.confirmError
	}
	s.state.Status = argodomain.OnboardingManaged
	return s.state, nil
}

func (s *fakeCatalogService) FindApplication(context.Context, catalogapp.Principal, uuid.UUID) (catalog.Application, error) {
	return catalog.Application{}, catalogapp.ErrResourceNotFound
}

func (s *fakeCatalogService) ListApplications(_ context.Context, _ catalogapp.Principal, _ uuid.UUID, _ uuid.UUID, _ uuid.UUID) ([]catalog.Application, error) {
	s.listCalls++
	return []catalog.Application{}, nil
}

func (s *fakeCatalogService) ListVisibleApplications(_ context.Context, _ catalogapp.Principal) ([]catalog.Application, error) {
	s.visibleCalls++
	return []catalog.Application{}, nil
}

func (s *fakeCatalogService) ListResourceTree(context.Context, catalogapp.Principal) (catalogapp.ResourceTree, error) {
	s.treeCalls++
	return s.tree, s.treeError
}

func requestMetricCount(t *testing.T, registry *prometheus.Registry) float64 {
	t.Helper()
	families, err := registry.Gather()
	if err != nil {
		t.Fatalf("gather metrics: %v", err)
	}
	for _, family := range families {
		if family.GetName() != "releasehub_http_requests_total" {
			continue
		}
		var total float64
		for _, metric := range family.Metric {
			total += metric.GetCounter().GetValue()
		}
		return total
	}
	return 0
}
