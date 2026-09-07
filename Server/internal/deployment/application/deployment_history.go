package application

import (
	"context"
	"errors"
	"fmt"
	"strings"

	"github.com/google/uuid"

	authz "github.com/vincent119/ReleaseHub/Server/internal/authorization/domain"
	deploydomain "github.com/vincent119/ReleaseHub/Server/internal/deployment/domain"
)

var (
	ErrHistoryNotFound  = errors.New("deployment history scope was not found")
	ErrHistoryForbidden = errors.New("deployment history is not available")
	ErrHistoryInvalid   = errors.New("deployment history query is invalid")
)

// DeploymentHistoryQuery identifies one scope and opaque cursor page.
type DeploymentHistoryQuery struct {
	ProjectID     uuid.UUID
	EnvironmentID uuid.UUID
	Cursor        string
	Limit         int
}

// DeploymentHistoryPage is one stable keyset page of successful deployments.
type DeploymentHistoryPage struct {
	Items      []deploydomain.DeploymentHistoryItem
	NextCursor string
	HasMore    bool
}

// DeploymentHistoryRepository is the consumer-owned history read port.
type DeploymentHistoryRepository interface {
	ResolveScope(context.Context, uuid.UUID, uuid.UUID) (authz.Scope, error)
	List(context.Context, authz.Scope, string, int) (DeploymentHistoryPage, error)
}

// DeploymentHistoryService authorizes successful deployment history queries.
type DeploymentHistoryService struct {
	repository DeploymentHistoryRepository
	authorizer WorkflowAuthorizer
	view       authz.Permission
}

// NewDeploymentHistoryService creates the history query use case.
func NewDeploymentHistoryService(repository DeploymentHistoryRepository, authorizer WorkflowAuthorizer) (*DeploymentHistoryService, error) {
	if repository == nil || authorizer == nil {
		return nil, errors.New("deployment history dependencies are required")
	}
	view, _ := authz.NewPermission("deployment_history.view")
	return &DeploymentHistoryService{repository: repository, authorizer: authorizer, view: view}, nil
}

// List returns one authorized page containing only complete successful deployments.
func (s *DeploymentHistoryService) List(ctx context.Context, principal RequestPrincipal, query DeploymentHistoryQuery) (DeploymentHistoryPage, error) {
	if query.ProjectID == uuid.Nil || query.EnvironmentID == uuid.Nil || query.Limit < 1 || query.Limit > 100 {
		return DeploymentHistoryPage{}, ErrHistoryInvalid
	}
	query.Cursor = strings.TrimSpace(query.Cursor)
	scope, err := s.repository.ResolveScope(ctx, query.ProjectID, query.EnvironmentID)
	if err != nil {
		return DeploymentHistoryPage{}, err
	}
	if err := s.authorize(ctx, principal, scope); err != nil {
		return DeploymentHistoryPage{}, err
	}
	return s.repository.List(ctx, scope, query.Cursor, query.Limit)
}

func (s *DeploymentHistoryService) authorize(ctx context.Context, principal RequestPrincipal, scope authz.Scope) error {
	allowed, err := s.authorizer.AuthorizeFresh(ctx, authz.AuthorizationRequest{
		UserID: principal.UserID, Disabled: principal.Disabled, Permission: s.view, Scope: scope,
	})
	if err != nil {
		return fmt.Errorf("authorize deployment history: %w", err)
	}
	if !allowed {
		return ErrHistoryForbidden
	}
	return nil
}
