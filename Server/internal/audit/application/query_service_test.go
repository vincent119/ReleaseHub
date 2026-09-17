package application

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/google/uuid"

	auditdomain "github.com/vincent119/ReleaseHub/Server/internal/audit/domain"
	authz "github.com/vincent119/ReleaseHub/Server/internal/authorization/domain"
)

func TestQueryServiceListFreshAuthorizesAndEncodesCursor(t *testing.T) {
	project := mustProjectScope(t)
	last := auditdomain.EventSummary{ID: uuid.New(), OccurredAt: time.Now().UTC()}
	repository := &repositoryStub{page: StoredPage{Items: []auditdomain.EventSummary{last}, HasMore: true}}
	authorizer := &authorizerStub{allowed: true}
	visibility := &visibilityStub{}
	service := newTestService(t, repository, authorizer, visibility)
	page, err := service.List(context.Background(), Principal{UserID: uuid.New()}, auditdomain.QueryFilter{Scope: project}, "")
	if err != nil || page.NextCursor == "" || !page.HasMore {
		t.Fatalf("page = %#v, error = %v", page, err)
	}
	if authorizer.calls != 1 || repository.listCalls != 1 || visibility.calls != 1 {
		t.Fatalf("calls authorize=%d list=%d visibility=%d", authorizer.calls, repository.listCalls, visibility.calls)
	}
	if _, err := service.List(context.Background(), Principal{UserID: uuid.New()}, auditdomain.QueryFilter{Scope: project, Action: "changed"}, page.NextCursor); !errors.Is(err, auditdomain.ErrInvalidCursor) {
		t.Fatalf("cross-filter cursor error = %v", err)
	}
	if authorizer.calls != 2 || repository.listCalls != 1 {
		t.Fatalf("fresh authorization should precede cursor validation; authorize=%d list=%d", authorizer.calls, repository.listCalls)
	}
}

func TestQueryServiceDetailMasksDeniedAndSanitizesMetadata(t *testing.T) {
	project := mustProjectScope(t)
	repository := &repositoryStub{
		ancestry: auditdomain.ScopeAncestry{OrganizationID: &project.OrganizationID, ProjectID: &project.ProjectID, Resolution: auditdomain.ScopeResolved},
		detail:   auditdomain.StoredDetail{EventSummary: auditdomain.EventSummary{ID: uuid.New()}, Metadata: []byte(`{"password":"hidden","safe":"ok"}`)},
	}
	authorizer := &authorizerStub{sequence: []bool{false, true}}
	service := newTestService(t, repository, authorizer, &visibilityStub{})
	detail, err := service.Detail(context.Background(), Principal{UserID: uuid.New()}, repository.detail.ID)
	if err != nil || !detail.MetadataTruncated || detail.Metadata["password"] != "[REDACTED]" || detail.Metadata["safe"] != "ok" {
		t.Fatalf("detail = %#v, error = %v", detail, err)
	}

	denied := newTestService(t, repository, &authorizerStub{sequence: []bool{false, false}}, &visibilityStub{})
	if _, err := denied.Detail(context.Background(), Principal{UserID: uuid.New()}, repository.detail.ID); !errors.Is(err, ErrNotFound) {
		t.Fatalf("denied detail error = %v", err)
	}
}

func TestQueryServiceCapabilitiesUseFreshAuthorizationForEveryServerRoot(t *testing.T) {
	organizationID, projectID, environmentID, applicationID := uuid.New(), uuid.New(), uuid.New(), uuid.New()
	project, err := authz.NewProjectScope(organizationID, projectID)
	if err != nil {
		t.Fatal(err)
	}
	environment, err := authz.NewEnvironmentScope(organizationID, projectID, environmentID)
	if err != nil {
		t.Fatal(err)
	}
	application, err := authz.NewApplicationScope(organizationID, projectID, environmentID, applicationID)
	if err != nil {
		t.Fatal(err)
	}
	visibility := &visibilityStub{roots: []ScopeRoot{
		{Scope: project, Label: "Project"},
		{Scope: environment, Label: "Environment"},
		{Scope: application, Label: "Application"},
	}}
	authorizer := &authorizerStub{sequence: []bool{false, true, false, true}}
	service := newTestService(t, &repositoryStub{}, authorizer, visibility)
	principal := Principal{UserID: uuid.New(), Disabled: true}

	capabilities, err := service.Capabilities(context.Background(), principal)
	if err != nil {
		t.Fatal(err)
	}
	if !capabilities.Visible || len(capabilities.ScopeRoots) != 2 || capabilities.ScopeRoots[0].Scope != project || capabilities.ScopeRoots[1].Scope != application {
		t.Fatalf("capabilities = %#v", capabilities)
	}
	if authorizer.calls != 4 {
		t.Fatalf("authorization calls = %d", authorizer.calls)
	}
	for _, request := range authorizer.requests {
		if request.UserID != principal.UserID || !request.Disabled || request.Permission != authz.Permission("audit.view") {
			t.Fatalf("authorization request = %#v", request)
		}
	}
}

func TestQueryServiceDeniedListDoesNotReadVisibilityOrRepository(t *testing.T) {
	repository := &repositoryStub{}
	visibility := &visibilityStub{}
	service := newTestService(t, repository, &authorizerStub{allowed: false}, visibility)

	_, err := service.List(context.Background(), Principal{UserID: uuid.New()}, auditdomain.QueryFilter{Scope: mustProjectScope(t)}, "")
	if !errors.Is(err, ErrForbidden) {
		t.Fatalf("denied list error = %v", err)
	}
	if repository.listCalls != 0 || visibility.calls != 0 {
		t.Fatalf("denied list reached data layer: list=%d visibility=%d", repository.listCalls, visibility.calls)
	}
}

func TestQueryServiceFilterOptionsFreshAuthorizes(t *testing.T) {
	repository := &repositoryStub{options: []FilterOption{{Value: "workflow.published", Label: "workflow.published"}}}
	authorizer := &authorizerStub{allowed: true}
	visibility := &visibilityStub{}
	service := newTestService(t, repository, authorizer, visibility)
	options, err := service.FilterOptions(context.Background(), Principal{UserID: uuid.New()}, auditdomain.FilterOptionFilter{
		Scope: mustProjectScope(t), Field: auditdomain.FilterOptionAction,
	})
	if err != nil || len(options) != 1 || options[0].Value != "workflow.published" {
		t.Fatalf("options = %#v, error = %v", options, err)
	}
	if authorizer.calls != 1 || visibility.calls != 1 || repository.optionCalls != 1 {
		t.Fatalf("calls authorize=%d visibility=%d options=%d", authorizer.calls, visibility.calls, repository.optionCalls)
	}
}

type repositoryStub struct {
	page        StoredPage
	ancestry    auditdomain.ScopeAncestry
	detail      auditdomain.StoredDetail
	options     []FilterOption
	listCalls   int
	optionCalls int
}

func (s *repositoryStub) ResolveScope(_ context.Context, kind authz.ScopeKind, id *uuid.UUID) (authz.Scope, error) {
	if kind == authz.ScopePlatform && id == nil {
		return authz.NewPlatformScope(), nil
	}
	return authz.Scope{}, ErrNotFound
}

func (s *repositoryStub) List(context.Context, ListQuery) (StoredPage, error) {
	s.listCalls++
	return s.page, nil
}
func (s *repositoryStub) FilterOptions(context.Context, auditdomain.FilterOptionFilter, VisibilityConstraint) ([]FilterOption, error) {
	s.optionCalls++
	return s.options, nil
}
func (s *repositoryStub) Locate(context.Context, uuid.UUID) (auditdomain.ScopeAncestry, error) {
	return s.ancestry, nil
}
func (s *repositoryStub) Get(context.Context, uuid.UUID, VisibilityConstraint) (auditdomain.StoredDetail, error) {
	return s.detail, nil
}

type authorizerStub struct {
	allowed  bool
	sequence []bool
	calls    int
	requests []authz.AuthorizationRequest
}

func (s *authorizerStub) AuthorizeFresh(_ context.Context, request authz.AuthorizationRequest) (bool, error) {
	index := s.calls
	s.calls++
	s.requests = append(s.requests, request)
	if index < len(s.sequence) {
		return s.sequence[index], nil
	}
	return s.allowed, nil
}

type visibilityStub struct {
	calls int
	roots []ScopeRoot
}

func (s *visibilityStub) Resolve(_ context.Context, _ uuid.UUID, root authz.Scope) (VisibilityConstraint, error) {
	s.calls++
	return VisibilityConstraint{Root: root}, nil
}

func (s *visibilityStub) ListRoots(context.Context, uuid.UUID) ([]ScopeRoot, error) {
	return s.roots, nil
}

func newTestService(t *testing.T, repository Repository, authorizer Authorizer, visibility VisibilityResolver) *QueryService {
	t.Helper()
	codec, err := auditdomain.NewCursorCodec([]byte("0123456789abcdef0123456789abcdef"))
	if err != nil {
		t.Fatal(err)
	}
	capability, ok := visibility.(CapabilityResolver)
	if !ok {
		t.Fatal("test visibility must implement capabilities")
	}
	service, err := NewQueryService(repository, authorizer, visibility, capability, codec)
	if err != nil {
		t.Fatal(err)
	}
	service.now = func() time.Time { return time.Date(2026, 9, 17, 0, 0, 0, 0, time.UTC) }
	return service
}

func mustProjectScope(t *testing.T) authz.Scope {
	t.Helper()
	scope, err := authz.NewProjectScope(uuid.New(), uuid.New())
	if err != nil {
		t.Fatal(err)
	}
	return scope
}
