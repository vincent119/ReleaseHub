// Package application coordinates deployment governance use cases.
package application

import (
	"context"
	"errors"
	"fmt"
	"sync"
	"time"

	"github.com/google/uuid"

	argodomain "github.com/vincent119/ReleaseHub/Server/internal/argocd/domain"
	deploydomain "github.com/vincent119/ReleaseHub/Server/internal/deployment/domain"
	ecrdomain "github.com/vincent119/ReleaseHub/Server/internal/ecr/domain"
)

// CandidateTarget identifies one managed production Application with an active Workflow and Plan binding.
type CandidateTarget struct {
	ApplicationID     uuid.UUID
	OrganizationID    uuid.UUID
	ProjectID         uuid.UUID
	EnvironmentID     uuid.UUID
	ApplicationKey    string
	Argo              argodomain.ApplicationIdentity
	ArgoProject       string
	WorkflowVersionID uuid.UUID
	PlanVersionID     uuid.UUID
}

// CandidateRepository is the transactional boundary for automatic Request creation.
type CandidateRepository interface {
	ListCandidateTargets(context.Context) ([]CandidateTarget, error)
	CreateDeploymentRequest(context.Context, deploydomain.CandidateObservation) (bool, error)
	ListPendingWorkflowStarts(context.Context) ([]PendingWorkflowStart, error)
}

// CandidateArgoReader exposes only fresh, read-only Argo CD operations.
type CandidateArgoReader interface {
	GetApplication(context.Context, argodomain.ApplicationIdentity, string) (argodomain.Application, error)
	GetTargetManifestsAtRevision(context.Context, argodomain.ApplicationIdentity, string, string) ([]string, string, error)
	GetManagedResourceDiffs(context.Context, argodomain.ApplicationIdentity, string) ([]argodomain.ResourceDiff, error)
}

// CandidateImageLocker reuses the ECR resolver's immutable digest rules.
type CandidateImageLocker interface {
	Lock(context.Context, uuid.UUID, string, []string) ([]ecrdomain.DigestSnapshot, error)
}

// CandidateObserver records low-cardinality reconciliation telemetry.
type CandidateObserver interface {
	ObserveCandidateReconciliation(string, time.Duration, CandidateResult, error)
}

// CandidateResult summarizes one reconciliation without exposing resource identities.
type CandidateResult struct {
	EligibleApplications int
	ChangedApplications  int
	CreatedRequests      int
	DuplicateCandidates  int
	FailedApplications   int
}

// CandidateReconciler turns fresh Application-level content differences into immutable Requests.
type CandidateReconciler struct {
	repository CandidateRepository
	argo       CandidateArgoReader
	images     CandidateImageLocker
	observer   CandidateObserver
	workflows  CandidateWorkflowStarter
	interval   time.Duration
	now        func() time.Time
	mu         sync.Mutex
}

// CandidateReconcilerOptions groups candidate reconciliation dependencies.
type CandidateReconcilerOptions struct {
	Repository CandidateRepository
	Argo       CandidateArgoReader
	Images     CandidateImageLocker
	Observer   CandidateObserver
	Workflows  CandidateWorkflowStarter
	Interval   time.Duration
}

type candidateTargetContent struct {
	application argodomain.Application
	manifests   []string
	revision    string
	observedAt  time.Time
}

type candidateEvidence struct {
	manifestHash string
	diffHash     string
	diffs        []deploydomain.ResourceDiffEvidence
	images       []deploydomain.ImageSnapshot
}

// NewCandidateReconciler validates the candidate reconciliation dependencies.
func NewCandidateReconciler(options CandidateReconcilerOptions) (*CandidateReconciler, error) {
	if options.Repository == nil || options.Argo == nil || options.Images == nil || options.Observer == nil || options.Workflows == nil || options.Interval <= 0 {
		return nil, errors.New("invalid candidate reconciler dependencies")
	}
	return &CandidateReconciler{
		repository: options.Repository, argo: options.Argo, images: options.Images,
		observer: options.Observer, workflows: options.Workflows, interval: options.Interval, now: time.Now,
	}, nil
}

// Run reconciles immediately and then at the configured interval until cancellation.
func (r *CandidateReconciler) Run(ctx context.Context) error {
	_, _ = r.ReconcileOnce(ctx)
	ticker := time.NewTicker(r.interval)
	defer ticker.Stop()
	for {
		select {
		case <-ctx.Done():
			return nil
		case <-ticker.C:
			_, _ = r.ReconcileOnce(ctx)
		}
	}
}

// ReconcileOnce processes every eligible Application independently and preserves successful peers.
func (r *CandidateReconciler) ReconcileOnce(ctx context.Context) (CandidateResult, error) {
	r.mu.Lock()
	defer r.mu.Unlock()
	started := r.now().UTC()
	targets, err := r.repository.ListCandidateTargets(ctx)
	if err != nil {
		r.observeReconciliation(started, CandidateResult{}, err)
		return CandidateResult{}, err
	}
	result, reconciliationErr := r.reconcileTargets(ctx, targets, started)
	reconciliationErr = errors.Join(reconciliationErr, r.startPendingWorkflows(ctx), r.recoverApprovedReviews(ctx))
	r.observeReconciliation(started, result, reconciliationErr)
	return result, reconciliationErr
}

func (r *CandidateReconciler) reconcileTargets(ctx context.Context, targets []CandidateTarget, observedAt time.Time) (CandidateResult, error) {
	result := CandidateResult{EligibleApplications: len(targets)}
	var reconciliationErr error
	for _, target := range targets {
		created, changed, err := r.reconcileTarget(ctx, target, observedAt)
		if err != nil {
			result.FailedApplications++
			reconciliationErr = errors.Join(reconciliationErr, fmt.Errorf("reconcile deployment candidate: %w", err))
			continue
		}
		accumulateCandidateResult(&result, created, changed)
	}
	return result, reconciliationErr
}

func (r *CandidateReconciler) observeReconciliation(started time.Time, result CandidateResult, err error) {
	status := "success"
	if err != nil {
		status = "partial_error"
	}
	r.observer.ObserveCandidateReconciliation(status, r.now().UTC().Sub(started), result, err)
}

func (r *CandidateReconciler) reconcileTarget(ctx context.Context, target CandidateTarget, observedAt time.Time) (bool, bool, error) {
	application, err := r.argo.GetApplication(ctx, target.Argo, target.ArgoProject)
	if err != nil {
		return false, false, fmt.Errorf("read Application: %w", err)
	}
	if application.AutomatedSync || !application.IsCandidate() || application.SyncStatus != "OutOfSync" {
		return false, false, nil
	}
	observation, changed, err := r.observeTarget(ctx, target, application, observedAt)
	if err != nil || !changed {
		return false, changed, err
	}
	created, err := r.repository.CreateDeploymentRequest(ctx, observation)
	return created, true, err
}

func (r *CandidateReconciler) observeTarget(ctx context.Context, target CandidateTarget, application argodomain.Application, observedAt time.Time) (deploydomain.CandidateObservation, bool, error) {
	if application.ResolvedRevision == "" {
		return deploydomain.CandidateObservation{}, false, errors.New("application resolved revision is empty")
	}
	manifests, revision, err := r.argo.GetTargetManifestsAtRevision(ctx, target.Argo, target.ArgoProject, application.ResolvedRevision)
	if err != nil {
		return deploydomain.CandidateObservation{}, false, fmt.Errorf("read target manifests: %w", err)
	}
	if revision == "" {
		return deploydomain.CandidateObservation{}, false, errors.New("target manifest revision is empty")
	}
	return r.buildObservation(ctx, target, candidateTargetContent{
		application: application, manifests: manifests, revision: revision, observedAt: observedAt,
	})
}

func (r *CandidateReconciler) buildObservation(ctx context.Context, target CandidateTarget, content candidateTargetContent) (deploydomain.CandidateObservation, bool, error) {
	evidence, changed, err := r.collectCandidateEvidence(ctx, target, content)
	if err != nil || !changed {
		return deploydomain.CandidateObservation{}, changed, err
	}
	observation, err := newCandidateObservation(target, content, evidence)
	return observation, true, err
}

func (r *CandidateReconciler) collectCandidateEvidence(ctx context.Context, target CandidateTarget, content candidateTargetContent) (candidateEvidence, bool, error) {
	evidence, changed, err := r.collectTargetDiff(ctx, target, content.manifests)
	if err != nil || !changed {
		return candidateEvidence{}, changed, err
	}
	imageSnapshots, err := r.images.Lock(ctx, target.ApplicationID, content.revision, content.manifests)
	if err != nil {
		return candidateEvidence{}, true, fmt.Errorf("lock target images: %w", err)
	}
	evidence.images = imageEvidence(imageSnapshots)
	return evidence, true, nil
}

func (r *CandidateReconciler) collectTargetDiff(ctx context.Context, target CandidateTarget, manifests []string) (candidateEvidence, bool, error) {
	diffs, err := r.argo.GetManagedResourceDiffs(ctx, target.Argo, target.ArgoProject)
	if err != nil {
		return candidateEvidence{}, false, fmt.Errorf("read managed resource differences: %w", err)
	}
	manifestHash, err := HashTargetManifests(manifests)
	if err != nil {
		return candidateEvidence{}, false, err
	}
	diffEvidence, diffHash, err := NormalizeResourceDiffs(diffs)
	if err != nil {
		return candidateEvidence{}, false, err
	}
	if len(diffEvidence) == 0 {
		return candidateEvidence{}, false, nil
	}
	return candidateEvidence{
		manifestHash: manifestHash, diffHash: diffHash, diffs: diffEvidence,
	}, true, nil
}

func newCandidateObservation(target CandidateTarget, content candidateTargetContent, evidence candidateEvidence) (deploydomain.CandidateObservation, error) {
	return deploydomain.NewCandidateObservation(deploydomain.CandidateObservation{
		ApplicationID: target.ApplicationID, OrganizationID: target.OrganizationID, ProjectID: target.ProjectID,
		EnvironmentID: target.EnvironmentID, ApplicationKey: target.ApplicationKey,
		WorkflowVersionID: target.WorkflowVersionID, PlanVersionID: target.PlanVersionID,
		TargetRevision: content.revision, TargetRevisions: []string{content.revision},
		ManifestHash: evidence.manifestHash, DiffHash: evidence.diffHash, Diffs: evidence.diffs,
		Sources: sourceEvidence(content.application.Sources), Images: evidence.images, ObservedAt: content.observedAt,
	})
}

func accumulateCandidateResult(result *CandidateResult, created, changed bool) {
	if !changed {
		return
	}
	result.ChangedApplications++
	if created {
		result.CreatedRequests++
		return
	}
	result.DuplicateCandidates++
}

func sourceEvidence(values []argodomain.Source) []deploydomain.SourceEvidence {
	result := make([]deploydomain.SourceEvidence, 0, len(values))
	for _, value := range values {
		result = append(result, deploydomain.SourceEvidence{
			RepositoryURL: value.RepositoryURL, TargetRevision: value.TargetRevision, Path: value.Path,
			Chart: value.Chart, Reference: value.Reference, Name: value.Name,
		})
	}
	return result
}

func imageEvidence(values []ecrdomain.DigestSnapshot) []deploydomain.ImageSnapshot {
	result := make([]deploydomain.ImageSnapshot, 0, len(values))
	for _, value := range values {
		result = append(result, deploydomain.ImageSnapshot{
			ImageReference: value.ImageReference, Registry: value.Registry, Repository: value.Repository,
			Tag: value.Tag, Digest: value.ResolvedDigest,
		})
	}
	return result
}
