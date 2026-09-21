package application

import (
	"context"
	"errors"
	"fmt"

	"github.com/google/uuid"

	argodomain "github.com/vincent119/ReleaseHub/Server/internal/argocd/domain"
	deploydomain "github.com/vincent119/ReleaseHub/Server/internal/deployment/domain"
	ecrdomain "github.com/vincent119/ReleaseHub/Server/internal/ecr/domain"
)

// PreflightTarget combines reviewed content with its current Argo CD identity.
type PreflightTarget struct {
	Snapshot    deploydomain.DeploymentRequestApplicationSnapshot
	Identity    argodomain.ApplicationIdentity
	ArgoProject string
}

// PreflightArgoReader exposes the read-only calls required before deployment.
type PreflightArgoReader interface {
	HardRefreshApplication(context.Context, argodomain.ApplicationIdentity, string) (argodomain.Application, error)
	GetTargetManifestsAtRevision(context.Context, argodomain.ApplicationIdentity, string, string) ([]string, string, error)
	GetManagedResourceDiffs(context.Context, argodomain.ApplicationIdentity, string) ([]argodomain.ResourceDiff, error)
}

// PreflightService validates fresh content without changing Argo CD desired or live state.
type PreflightService struct {
	argo   PreflightArgoReader
	images CandidateImageLocker
}

type preflightContent struct {
	application  argodomain.Application
	manifests    []string
	revision     string
	manifestHash string
}

// NewPreflightService creates the read-only deployment gate.
func NewPreflightService(argo PreflightArgoReader, images CandidateImageLocker) (*PreflightService, error) {
	if argo == nil || images == nil {
		return nil, errors.New("invalid deployment preflight dependencies")
	}
	return &PreflightService{argo: argo, images: images}, nil
}

// Check hard-refreshes Argo CD and compares Application-scoped evidence.
func (s *PreflightService) Check(ctx context.Context, target PreflightTarget) (deploydomain.PreflightResult, error) {
	if !validPreflightTarget(target) {
		return deploydomain.PreflightResult{}, errors.New("invalid deployment preflight target")
	}
	application, err := s.argo.HardRefreshApplication(ctx, target.Identity, target.ArgoProject)
	if err != nil {
		return deploydomain.PreflightResult{}, fmt.Errorf("refresh deployment target: %w", err)
	}
	evidence, err := s.collectEvidence(ctx, target, application)
	if err != nil {
		return deploydomain.PreflightResult{}, err
	}
	return deploydomain.EvaluatePreflight(target.Snapshot, evidence), nil
}

func (s *PreflightService) manifestHashMatchesAtRevision(ctx context.Context, target PreflightTarget, revision string) (bool, error) {
	if revision == "" {
		return false, errors.New("completed Application revision is empty")
	}
	manifests, resolved, err := s.argo.GetTargetManifestsAtRevision(ctx, target.Identity, target.ArgoProject, revision)
	if err != nil {
		return false, fmt.Errorf("read completed revision manifests: %w", err)
	}
	if resolved != revision {
		return false, fmt.Errorf("completed manifest revision mismatch: expected %q, got %q", revision, resolved)
	}
	hash, err := HashTargetManifests(manifests)
	if err != nil {
		return false, fmt.Errorf("hash completed revision manifests: %w", err)
	}
	return hash == target.Snapshot.ManifestHash, nil
}

func (s *PreflightService) collectEvidence(ctx context.Context, target PreflightTarget, application argodomain.Application) (deploydomain.PreflightEvidence, error) {
	if application.ResolvedRevision == "" {
		return deploydomain.PreflightEvidence{}, errors.New("preflight application resolved revision is empty")
	}
	manifests, revision, err := s.argo.GetTargetManifestsAtRevision(ctx, target.Identity, target.ArgoProject, application.ResolvedRevision)
	if err != nil {
		return deploydomain.PreflightEvidence{}, fmt.Errorf("read preflight manifests: %w", err)
	}
	manifestHash, err := HashTargetManifests(manifests)
	if err != nil {
		return deploydomain.PreflightEvidence{}, err
	}
	return s.collectDiffAndImages(ctx, target, preflightContent{
		application: application, manifests: manifests, revision: revision, manifestHash: manifestHash,
	})
}

func (s *PreflightService) collectDiffAndImages(ctx context.Context, target PreflightTarget, content preflightContent) (deploydomain.PreflightEvidence, error) {
	diffs, err := s.argo.GetManagedResourceDiffs(ctx, target.Identity, target.ArgoProject)
	if err != nil {
		return deploydomain.PreflightEvidence{}, fmt.Errorf("read preflight differences: %w", err)
	}
	_, diffHash, err := NormalizeResourceDiffs(diffs)
	if err != nil {
		return deploydomain.PreflightEvidence{}, err
	}
	images, err := s.images.Lock(ctx, target.Snapshot.ApplicationID, content.revision, content.manifests)
	if err != nil && len(images) == 0 {
		return deploydomain.PreflightEvidence{}, fmt.Errorf("resolve preflight images: %w", err)
	}
	return deploydomain.PreflightEvidence{
		AutomatedSync: content.application.AutomatedSync, ResolvedRevision: content.revision,
		ManifestHash: content.manifestHash,
		DiffHash:     diffHash, Images: preflightImages(images),
	}, nil
}

func preflightImages(values []ecrdomain.DigestSnapshot) []deploydomain.DeploymentRequestImageSnapshot {
	result := make([]deploydomain.DeploymentRequestImageSnapshot, 0, len(values))
	for _, value := range values {
		result = append(result, deploydomain.DeploymentRequestImageSnapshot{
			ImageReference: value.ImageReference, Registry: value.Registry,
			Repository: value.Repository, Tag: value.Tag, Digest: value.ResolvedDigest,
		})
	}
	return result
}

func validPreflightTarget(value PreflightTarget) bool {
	return value.Snapshot.ID != uuid.Nil && value.Snapshot.ApplicationID != uuid.Nil &&
		value.Identity.Name != "" && value.Identity.Namespace != ""
}
