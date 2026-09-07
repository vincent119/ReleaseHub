package application

import (
	"context"
	"errors"
	"fmt"
	"time"

	"github.com/google/uuid"

	argodomain "github.com/vincent119/ReleaseHub/Server/internal/argocd/domain"
	authz "github.com/vincent119/ReleaseHub/Server/internal/authorization/domain"
	catalogapp "github.com/vincent119/ReleaseHub/Server/internal/catalog/application"
	catalog "github.com/vincent119/ReleaseHub/Server/internal/catalog/domain"
)

// ErrCandidateQueueNotFound hides the platform candidate queue from unauthorized users.
var ErrCandidateQueueNotFound = errors.New("candidate queue not found")

// ErrCandidateAssignmentConflict indicates stale discovery state, invalid mapping, or an already assigned candidate.
var ErrCandidateAssignmentConflict = errors.New("candidate assignment conflict")

// Candidate is a present Argo CD Application awaiting catalog assignment or onboarding.
type Candidate struct {
	ID                    uuid.UUID
	ArgoCDNamespace       string
	ArgoCDApplicationName string
	ArgoCDProject         string
	Sources               []argodomain.Source
	DestinationServer     string
	DestinationName       string
	DestinationNamespace  string
	AutomatedSync         bool
	SyncStatus            string
	HealthStatus          string
	OperationPhase        string
	ResolvedRevision      string
	ResourceVersion       string
	FirstSeenAt           time.Time
	LastSeenAt            time.Time
	Version               uint64
}

// AssignmentScope is one active production target available to a platform candidate administrator.
type AssignmentScope struct {
	OrganizationID   uuid.UUID
	OrganizationName string
	ProjectID        uuid.UUID
	ProjectName      string
	EnvironmentID    uuid.UUID
	EnvironmentName  string
}

// CandidateRepository reads candidate observations without contacting Argo CD.
type CandidateRepository interface {
	ListPresent(context.Context) ([]Candidate, error)
	LoadPresent(context.Context, uuid.UUID) (Candidate, error)
	Assign(context.Context, catalogapp.Mutation, uuid.UUID, uint64, catalog.Application) error
	ListAssignmentScopes(context.Context) ([]AssignmentScope, error)
}

// CandidateAuthorizer evaluates the global platform scope.
type CandidateAuthorizer interface {
	AuthorizeFresh(context.Context, authz.AuthorizationRequest) (bool, error)
}

// CandidateService protects the platform candidate queue.
type CandidateService struct {
	repository CandidateRepository
	authorizer CandidateAuthorizer
	view       authz.Permission
	assign     authz.Permission
}

// NewCandidateService creates the candidate queue use case.
func NewCandidateService(repository CandidateRepository, authorizer CandidateAuthorizer) (*CandidateService, error) {
	if repository == nil || authorizer == nil {
		return nil, errors.New("invalid candidate service dependencies")
	}
	view, err := authz.NewPermission("argocd.candidate.view")
	if err != nil {
		return nil, err
	}
	assign, err := authz.NewPermission("argocd.candidate.assign")
	if err != nil {
		return nil, err
	}
	return &CandidateService{repository: repository, authorizer: authorizer, view: view, assign: assign}, nil
}

// List returns present candidates only to a platform-authorized principal.
func (s *CandidateService) List(ctx context.Context, principal OnboardingPrincipal) ([]Candidate, error) {
	allowed, err := s.authorizer.AuthorizeFresh(ctx, authz.AuthorizationRequest{UserID: principal.UserID, Disabled: principal.Disabled, Permission: s.view, Scope: authz.NewPlatformScope()})
	if err != nil {
		return nil, fmt.Errorf("authorize candidate queue: %w", err)
	}
	if !allowed {
		return nil, ErrCandidateQueueNotFound
	}
	return s.repository.ListPresent(ctx)
}

// AssignCandidateInput is the administrator-confirmed mapping for one discovered candidate.
type AssignCandidateInput struct {
	CandidateID     uuid.UUID
	ExpectedVersion uint64
	OrganizationID  uuid.UUID
	ProjectID       uuid.UUID
	EnvironmentID   uuid.UUID
	ApplicationName string
	Source          catalog.GitOpsSource
	RequestID       string
}

// Assign creates one catalog Application from a fresh candidate observation.
func (s *CandidateService) Assign(ctx context.Context, principal OnboardingPrincipal, input AssignCandidateInput) (catalog.Application, error) {
	allowed, err := s.authorizer.AuthorizeFresh(ctx, authz.AuthorizationRequest{UserID: principal.UserID, Disabled: principal.Disabled, Permission: s.assign, Scope: authz.NewPlatformScope()})
	if err != nil {
		return catalog.Application{}, fmt.Errorf("authorize candidate assignment: %w", err)
	}
	if !allowed {
		return catalog.Application{}, ErrCandidateQueueNotFound
	}
	candidate, err := s.repository.LoadPresent(ctx, input.CandidateID)
	if err != nil || candidate.Version != input.ExpectedVersion || !candidateContainsSource(candidate, input.Source) {
		return catalog.Application{}, ErrCandidateAssignmentConflict
	}
	value, err := catalog.NewApplication(input.OrganizationID, input.ProjectID, input.EnvironmentID, input.ApplicationName, catalog.ArgoApplicationIdentity{Namespace: candidate.ArgoCDNamespace, Name: candidate.ArgoCDApplicationName}, candidate.ArgoCDProject, candidate.DestinationServer, candidate.DestinationNamespace, input.Source)
	if err != nil {
		return catalog.Application{}, fmt.Errorf("build candidate Application mapping: %w", err)
	}
	actorID := principal.UserID
	if err := s.repository.Assign(ctx, catalogapp.Mutation{ActorID: &actorID, RequestID: input.RequestID}, candidate.ID, input.ExpectedVersion, value); err != nil {
		return catalog.Application{}, ErrCandidateAssignmentConflict
	}
	return value, nil
}

// ListAssignmentScopes returns active production targets after platform assignment authorization.
func (s *CandidateService) ListAssignmentScopes(ctx context.Context, principal OnboardingPrincipal) ([]AssignmentScope, error) {
	allowed, err := s.authorizer.AuthorizeFresh(ctx, authz.AuthorizationRequest{UserID: principal.UserID, Disabled: principal.Disabled, Permission: s.assign, Scope: authz.NewPlatformScope()})
	if err != nil {
		return nil, fmt.Errorf("authorize candidate assignment scopes: %w", err)
	}
	if !allowed {
		return nil, ErrCandidateQueueNotFound
	}
	return s.repository.ListAssignmentScopes(ctx)
}

func candidateContainsSource(candidate Candidate, expected catalog.GitOpsSource) bool {
	expected = expected.Normalized()
	for _, source := range candidate.Sources {
		if source.RepositoryURL == expected.RepositoryURL && source.TargetRevision == expected.TargetRevision && source.Path == expected.Path {
			return true
		}
	}
	return false
}
