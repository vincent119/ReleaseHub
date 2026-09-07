package application

import (
	"context"
	"errors"
	"testing"

	"github.com/google/uuid"

	argodomain "github.com/vincent119/ReleaseHub/Server/internal/argocd/domain"
	authz "github.com/vincent119/ReleaseHub/Server/internal/authorization/domain"
	catalogapp "github.com/vincent119/ReleaseHub/Server/internal/catalog/application"
	catalog "github.com/vincent119/ReleaseHub/Server/internal/catalog/domain"
)

func TestCandidateQueueRequiresPlatformPermissionBeforeRepositoryRead(t *testing.T) {
	repository := &candidateRepositoryStub{}
	service, err := NewCandidateService(repository, &candidateAuthorizerStub{allowed: false})
	if err != nil {
		t.Fatalf("create candidate service: %v", err)
	}
	_, err = service.List(context.Background(), OnboardingPrincipal{UserID: uuid.New()})
	if !errors.Is(err, ErrCandidateQueueNotFound) || repository.calls != 0 {
		t.Fatalf("unauthorized candidate list error = %v, repository calls = %d", err, repository.calls)
	}
}

func TestCandidateQueueUsesPlatformScope(t *testing.T) {
	repository := &candidateRepositoryStub{values: []Candidate{{ID: uuid.New()}}}
	authorizer := &candidateAuthorizerStub{allowed: true}
	service, err := NewCandidateService(repository, authorizer)
	if err != nil {
		t.Fatalf("create candidate service: %v", err)
	}
	values, err := service.List(context.Background(), OnboardingPrincipal{UserID: uuid.New()})
	if err != nil || len(values) != 1 || repository.calls != 1 {
		t.Fatalf("candidate list = %#v, calls = %d, error = %v", values, repository.calls, err)
	}
	if authorizer.request.Scope.Kind != authz.ScopePlatform || authorizer.request.Permission != "argocd.candidate.view" {
		t.Fatalf("authorization request = %#v", authorizer.request)
	}
}

func TestCandidateAssignmentScopesRequirePlatformAssignmentPermission(t *testing.T) {
	repository := &candidateRepositoryStub{scopes: []AssignmentScope{{EnvironmentID: uuid.New()}}}
	authorizer := &candidateAuthorizerStub{allowed: true}
	service, err := NewCandidateService(repository, authorizer)
	if err != nil {
		t.Fatalf("create candidate service: %v", err)
	}
	values, err := service.ListAssignmentScopes(context.Background(), OnboardingPrincipal{UserID: uuid.New()})
	if err != nil || len(values) != 1 || repository.scopeCalls != 1 {
		t.Fatalf("assignment scopes = %#v, calls = %d, error = %v", values, repository.scopeCalls, err)
	}
	if authorizer.request.Scope.Kind != authz.ScopePlatform || authorizer.request.Permission != "argocd.candidate.assign" {
		t.Fatalf("authorization request = %#v", authorizer.request)
	}

	unauthorizedRepository := &candidateRepositoryStub{}
	unauthorized, err := NewCandidateService(unauthorizedRepository, &candidateAuthorizerStub{allowed: false})
	if err != nil {
		t.Fatalf("create unauthorized candidate service: %v", err)
	}
	_, err = unauthorized.ListAssignmentScopes(context.Background(), OnboardingPrincipal{UserID: uuid.New()})
	if !errors.Is(err, ErrCandidateQueueNotFound) || unauthorizedRepository.scopeCalls != 0 {
		t.Fatalf("unauthorized scopes error = %v, calls = %d", err, unauthorizedRepository.scopeCalls)
	}
}

func TestCandidateAssignmentUsesFreshObservedMapping(t *testing.T) {
	candidate := Candidate{ID: uuid.New(), Version: 4, ArgoCDNamespace: "argocd", ArgoCDApplicationName: "payment-production", ArgoCDProject: "payment", DestinationServer: "https://kubernetes.default.svc", DestinationNamespace: "payment", Sources: []argodomain.Source{{RepositoryURL: "https://git.example.com/manifests.git", TargetRevision: "main", Path: "production/payment"}}}
	repository := &candidateRepositoryStub{values: []Candidate{candidate}}
	service, err := NewCandidateService(repository, &candidateAuthorizerStub{allowed: true})
	if err != nil {
		t.Fatalf("create candidate service: %v", err)
	}
	organizationID, projectID, environmentID := uuid.New(), uuid.New(), uuid.New()
	value, err := service.Assign(context.Background(), OnboardingPrincipal{UserID: uuid.New()}, AssignCandidateInput{CandidateID: candidate.ID, ExpectedVersion: 4, OrganizationID: organizationID, ProjectID: projectID, EnvironmentID: environmentID, ApplicationName: "payment", Source: catalog.GitOpsSource{RepositoryURL: candidate.Sources[0].RepositoryURL, TargetRevision: "main", Path: "production/payment"}, RequestID: "assign-1"})
	if err != nil || repository.assigns != 1 {
		t.Fatalf("assign candidate = %#v, assigns = %d, error = %v", value, repository.assigns, err)
	}
	if value.Argo.Namespace != candidate.ArgoCDNamespace || value.Argo.Name != candidate.ArgoCDApplicationName || value.DestinationServer != candidate.DestinationServer {
		t.Fatalf("candidate-controlled mapping was not preserved: %#v", value)
	}
}

type candidateRepositoryStub struct {
	values     []Candidate
	scopes     []AssignmentScope
	calls      int
	scopeCalls int
	assigns    int
}

func (s *candidateRepositoryStub) ListAssignmentScopes(context.Context) ([]AssignmentScope, error) {
	s.scopeCalls++
	return s.scopes, nil
}

func (s *candidateRepositoryStub) ListPresent(context.Context) ([]Candidate, error) {
	s.calls++
	return s.values, nil
}

func (s *candidateRepositoryStub) LoadPresent(_ context.Context, id uuid.UUID) (Candidate, error) {
	for _, value := range s.values {
		if value.ID == id {
			return value, nil
		}
	}
	return Candidate{}, errors.New("not found")
}

func (s *candidateRepositoryStub) Assign(context.Context, catalogapp.Mutation, uuid.UUID, uint64, catalog.Application) error {
	s.assigns++
	return nil
}

type candidateAuthorizerStub struct {
	allowed bool
	request authz.AuthorizationRequest
}

func (s *candidateAuthorizerStub) AuthorizeFresh(_ context.Context, request authz.AuthorizationRequest) (bool, error) {
	s.request = request
	return s.allowed, nil
}
