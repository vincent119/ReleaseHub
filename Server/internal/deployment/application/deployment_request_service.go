package application

import (
	"context"
	"errors"
	"fmt"
	"time"

	"github.com/google/uuid"

	authz "github.com/vincent119/ReleaseHub/Server/internal/authorization/domain"
	deploydomain "github.com/vincent119/ReleaseHub/Server/internal/deployment/domain"
)

var (
	// ErrRequestForbidden hides deployment requests outside the caller scope.
	ErrRequestForbidden = errors.New("deployment request operation is not allowed")
	// ErrRequestNotFound identifies an unavailable deployment request or version.
	ErrRequestNotFound = errors.New("deployment request was not found")
	// ErrRequestConflict identifies a stale request version update.
	ErrRequestConflict = errors.New("deployment request changed concurrently")
	// ErrRequestInvalid identifies rejected request metadata.
	ErrRequestInvalid = errors.New("deployment request is invalid")
)

// RequestPrincipal is the authenticated identity for request operations.
type RequestPrincipal struct {
	UserID   uuid.UUID
	Disabled bool
}

// RequestMutation carries actor and audit correlation for a new immutable version.
type RequestMutation struct {
	ActorID    uuid.UUID
	RequestID  string
	OccurredAt time.Time
}

// DeploymentRequestRepository is the consumer-owned request persistence port.
type DeploymentRequestRepository interface {
	List(context.Context, authz.Scope) ([]deploydomain.DeploymentRequestSummary, error)
	Load(context.Context, uuid.UUID) (deploydomain.DeploymentRequestDetail, error)
	SupersedeMetadata(context.Context, RequestMutation, MetadataVersionChange) (deploydomain.DeploymentRequestDetail, error)
}

// MetadataVersionChange creates a new version from immutable snapshots.
type MetadataVersionChange struct {
	RequestID       uuid.UUID
	RequestVersion  uuid.UUID
	ExpectedVersion uint64
	Metadata        deploydomain.DeploymentRequestMetadata
}

// DeploymentRequestService exposes request viewing and immutable metadata versioning.
type DeploymentRequestService struct {
	repository DeploymentRequestRepository
	authorizer WorkflowAuthorizer
	clock      WorkflowClock
	view       authz.Permission
	update     authz.Permission
}

// NewDeploymentRequestService creates the request use case.
func NewDeploymentRequestService(repository DeploymentRequestRepository, authorizer WorkflowAuthorizer, clock WorkflowClock) (*DeploymentRequestService, error) {
	if repository == nil || authorizer == nil || clock == nil {
		return nil, errors.New("deployment request dependencies are required")
	}
	view, _ := authz.NewPermission("deployment_request.view")
	update, _ := authz.NewPermission("deployment_request.update")
	return &DeploymentRequestService{repository: repository, authorizer: authorizer, clock: clock, view: view, update: update}, nil
}

// List returns requests only after scope authorization.
func (s *DeploymentRequestService) List(ctx context.Context, principal RequestPrincipal, scope authz.Scope) ([]deploydomain.DeploymentRequestSummary, error) {
	if err := s.authorize(ctx, principal, s.view, scope); err != nil {
		return nil, err
	}
	return s.repository.List(ctx, scope)
}

// Get returns a single request after resolving its persisted scope.
func (s *DeploymentRequestService) Get(ctx context.Context, principal RequestPrincipal, requestID uuid.UUID) (deploydomain.DeploymentRequestDetail, error) {
	detail, err := s.repository.Load(ctx, requestID)
	if err != nil {
		return deploydomain.DeploymentRequestDetail{}, err
	}
	if err := s.authorize(ctx, principal, s.view, requestScope(detail.Summary)); err != nil {
		return deploydomain.DeploymentRequestDetail{}, err
	}
	return s.withCapabilities(ctx, principal, detail)
}

// UpdateMetadata supersedes an undeployed version with a newly reviewed metadata snapshot.
func (s *DeploymentRequestService) UpdateMetadata(ctx context.Context, principal RequestPrincipal, input MetadataVersionChange, requestID string) (deploydomain.DeploymentRequestDetail, error) {
	input, detail, err := s.prepareMetadataVersion(ctx, input)
	if err != nil {
		return deploydomain.DeploymentRequestDetail{}, err
	}
	if err := s.authorize(ctx, principal, s.update, requestScope(detail.Summary)); err != nil {
		return deploydomain.DeploymentRequestDetail{}, err
	}
	mutation := RequestMutation{ActorID: principal.UserID, RequestID: requestID, OccurredAt: s.clock.Now()}
	updated, err := s.repository.SupersedeMetadata(ctx, mutation, input)
	if err != nil {
		return deploydomain.DeploymentRequestDetail{}, err
	}
	return s.withCapabilities(ctx, principal, updated)
}

func (s *DeploymentRequestService) prepareMetadataVersion(ctx context.Context, input MetadataVersionChange) (MetadataVersionChange, deploydomain.DeploymentRequestDetail, error) {
	if input.RequestID == uuid.Nil || input.RequestVersion == uuid.Nil || input.ExpectedVersion == 0 {
		return input, deploydomain.DeploymentRequestDetail{}, ErrRequestInvalid
	}
	detail, err := s.repository.Load(ctx, input.RequestID)
	if err != nil || detail.Version.ID != input.RequestVersion || detail.Version.LockVersion != input.ExpectedVersion || !detail.Version.CanBeSuperseded() {
		return input, detail, requestPreparationError(err)
	}
	metadata, err := deploydomain.NewDeploymentRequestMetadata(input.Metadata)
	if err != nil {
		return input, detail, fmt.Errorf("%w: %v", ErrRequestInvalid, err)
	}
	input.Metadata = metadata
	return input, detail, nil
}

func requestPreparationError(err error) error {
	if err != nil {
		return err
	}
	return ErrRequestConflict
}

func (s *DeploymentRequestService) authorize(ctx context.Context, principal RequestPrincipal, permission authz.Permission, scope authz.Scope) error {
	allowed, err := s.authorizer.AuthorizeFresh(ctx, authz.AuthorizationRequest{
		UserID: principal.UserID, Disabled: principal.Disabled, Permission: permission, Scope: scope,
	})
	if err != nil {
		return fmt.Errorf("authorize deployment request: %w", err)
	}
	if !allowed {
		return ErrRequestForbidden
	}
	return nil
}

func requestScope(value deploydomain.DeploymentRequestSummary) authz.Scope {
	scope, _ := authz.NewEnvironmentScope(value.OrganizationID, value.ProjectID, value.EnvironmentID)
	return scope
}
