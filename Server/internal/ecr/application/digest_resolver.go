package application

import (
	"context"
	"errors"
	"fmt"

	"github.com/google/uuid"

	argodomain "github.com/vincent119/ReleaseHub/Server/internal/argocd/domain"
	authz "github.com/vincent119/ReleaseHub/Server/internal/authorization/domain"
	ecrdomain "github.com/vincent119/ReleaseHub/Server/internal/ecr/domain"
)

var (
	// ErrDigestTargetNotFound intentionally hides unavailable and unauthorized Applications.
	ErrDigestTargetNotFound = errors.New("digest resolution target not found")
	// ErrTargetManifestUnavailable means ReleaseHub could not safely obtain the current Argo CD target manifests.
	ErrTargetManifestUnavailable = errors.New("target manifests are unavailable")
	// ErrDigestUnavailable means at least one target image could not be locked to an immutable digest.
	ErrDigestUnavailable = errors.New("target image digest is unavailable")
)

// DigestTarget is a managed production Application eligible for target-image resolution.
type DigestTarget struct {
	ApplicationID  uuid.UUID
	OrganizationID uuid.UUID
	ProjectID      uuid.UUID
	EnvironmentID  uuid.UUID
	Argo           argodomain.ApplicationIdentity
	ArgoProject    string
}

// DigestMutation identifies the authenticated actor attached to an auditable resolution command.
type DigestMutation struct {
	ActorID   uuid.UUID
	RequestID string
}

// DigestPrincipal contains the local account state required by authorization.
type DigestPrincipal struct {
	UserID   uuid.UUID
	Disabled bool
}

// DigestRepository persists immutable resolution outcomes and reads managed targets.
type DigestRepository interface {
	LoadDigestTarget(context.Context, uuid.UUID) (DigestTarget, error)
	AppendDigestSnapshots(context.Context, DigestMutation, DigestTarget, []ecrdomain.DigestSnapshot) error
}

// TargetManifestReader exposes only Argo CD's read-only target-manifest operation.
type TargetManifestReader interface {
	GetTargetManifests(context.Context, argodomain.ApplicationIdentity, string) ([]string, string, error)
}

// DigestRegistry resolves exactly one validated ECR image identifier.
type DigestRegistry interface {
	ResolveDigest(context.Context, ecrdomain.ImageReference) (string, error)
}

// DigestAuthorizer verifies the latest authorization policy before a resolver command.
type DigestAuthorizer interface {
	AuthorizeFresh(context.Context, authz.AuthorizationRequest) (bool, error)
}

// DigestResolver coordinates target-manifest extraction, read-only ECR lookup, and immutable persistence.
type DigestResolver struct {
	repository DigestRepository
	manifests  TargetManifestReader
	locker     *ManifestImageLocker
	authorizer DigestAuthorizer
	permission authz.Permission
}

// NewDigestResolver validates the capability-limited dependencies for ECR digest resolution.
func NewDigestResolver(repository DigestRepository, manifests TargetManifestReader, registry DigestRegistry, authorizer DigestAuthorizer, scope ecrdomain.RegistryScope) (*DigestResolver, error) {
	if repository == nil || manifests == nil || registry == nil || authorizer == nil || len(scope.Repositories) == 0 {
		return nil, errors.New("invalid digest resolver dependencies")
	}
	permission, err := authz.NewPermission("ecr.digest.read")
	if err != nil {
		return nil, err
	}
	locker, err := NewManifestImageLocker(registry, scope)
	if err != nil {
		return nil, err
	}
	return &DigestResolver{repository: repository, manifests: manifests, locker: locker, authorizer: authorizer, permission: permission}, nil
}

// Resolve locks every target-manifest image reference to an exact ECR digest or records a stable unavailable outcome.
func (r *DigestResolver) Resolve(ctx context.Context, principal DigestPrincipal, mutation DigestMutation, applicationID uuid.UUID) ([]ecrdomain.DigestSnapshot, error) {
	mutation.ActorID = principal.UserID
	target, err := r.authorizedTarget(ctx, principal, applicationID)
	if err != nil {
		return nil, err
	}
	manifests, revision, err := r.manifests.GetTargetManifests(ctx, target.Argo, target.ArgoProject)
	if err != nil || revision == "" {
		return nil, ErrTargetManifestUnavailable
	}
	snapshots, resolutionErr := r.locker.Lock(ctx, target.ApplicationID, revision, manifests)
	if len(snapshots) == 0 {
		return nil, resolutionErr
	}
	if err := r.repository.AppendDigestSnapshots(ctx, mutation, target, snapshots); err != nil {
		return nil, err
	}
	return snapshots, resolutionErr
}

func (r *DigestResolver) authorizedTarget(ctx context.Context, principal DigestPrincipal, applicationID uuid.UUID) (DigestTarget, error) {
	target, err := r.repository.LoadDigestTarget(ctx, applicationID)
	if err != nil {
		return DigestTarget{}, ErrDigestTargetNotFound
	}
	scope, err := authz.NewApplicationScope(target.OrganizationID, target.ProjectID, target.EnvironmentID, target.ApplicationID)
	if err != nil {
		return DigestTarget{}, err
	}
	allowed, err := r.authorizer.AuthorizeFresh(ctx, authz.AuthorizationRequest{UserID: principal.UserID, Disabled: principal.Disabled, Permission: r.permission, Scope: scope})
	if err != nil {
		return DigestTarget{}, fmt.Errorf("authorize ecr digest resolution: %w", err)
	}
	if !allowed {
		return DigestTarget{}, ErrDigestTargetNotFound
	}
	return target, nil
}

func referenceErrorCode(err error) string {
	if errors.Is(err, ecrdomain.ErrRepositoryOutOfScope) {
		return "repository_out_of_scope"
	}
	return "invalid_image_reference"
}

func registryErrorCode(err error) string {
	switch {
	case errors.Is(err, ecrdomain.ErrImageNotFound):
		return "image_not_found"
	case errors.Is(err, ecrdomain.ErrDigestMismatch):
		return "digest_mismatch"
	default:
		return "ecr_unavailable"
	}
}
