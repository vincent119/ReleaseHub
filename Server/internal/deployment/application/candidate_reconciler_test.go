package application

import (
	"context"
	"errors"
	"slices"
	"strings"
	"testing"
	"time"

	"github.com/google/uuid"

	argodomain "github.com/vincent119/ReleaseHub/Server/internal/argocd/domain"
	deploydomain "github.com/vincent119/ReleaseHub/Server/internal/deployment/domain"
	ecrdomain "github.com/vincent119/ReleaseHub/Server/internal/ecr/domain"
)

func TestDeploymentRequestCreatedFromArgoCDRevision(t *testing.T) {
	repository := newCandidateRepositoryStub(candidateTarget())
	workflows := &candidateWorkflowStarterStub{}
	requestedRevisions := []string{}
	reconciler := newCandidateReconcilerForTest(t, repository, candidateArgoStub{requestedRevisions: &requestedRevisions}, workflows)
	result, err := reconciler.ReconcileOnce(context.Background())
	if err != nil {
		t.Fatalf("reconcile candidate: %v", err)
	}
	if result.CreatedRequests != 1 || len(repository.observations) != 1 || len(workflows.started) != 1 {
		t.Fatalf("unexpected result: %#v observations=%d", result, len(repository.observations))
	}
	observation := repository.observations[0]
	if observation.TargetRevision != "commit-b" || observation.Images[0].Digest == "" || observation.Fingerprint == "" {
		t.Fatalf("incomplete observation: %#v", observation)
	}
	if len(requestedRevisions) != 1 || requestedRevisions[0] != "commit-b" {
		t.Fatalf("target manifests were not pinned to the observed revision: %#v", requestedRevisions)
	}
}

func TestCandidateObservationStoresScalarRevisionForSingleSource(t *testing.T) {
	repository := newCandidateRepositoryStub(candidateTarget())
	reconciler := newCandidateReconcilerForTest(t, repository, candidateArgoStub{}, &candidateWorkflowStarterStub{})
	if _, err := reconciler.ReconcileOnce(context.Background()); err != nil {
		t.Fatalf("reconcile candidate: %v", err)
	}
	observation := repository.observations[0]
	if observation.TargetRevision != "commit-b" || observation.TargetRevisions == nil || len(observation.TargetRevisions) != 0 {
		t.Fatalf("single-source revision representation = %q, %#v", observation.TargetRevision, observation.TargetRevisions)
	}
}

func TestCandidateObservationRejectsIncompleteMultiSourceRevisionVector(t *testing.T) {
	content := candidateTargetContent{
		application: argodomain.Application{
			Sources:           []argodomain.Source{{Name: "app"}, {Name: "values"}},
			ResolvedRevisions: []string{"commit-a"},
		},
		revision: "commit-a", observedAt: time.Now(),
	}
	_, err := newCandidateObservation(candidateTarget(), content, candidateRevisionEvidence())
	if err == nil || err.Error() != "candidate multi-source revision vector is incomplete" {
		t.Fatalf("incomplete multi-source revision vector error = %v", err)
	}
}

func TestCandidateObservationStoresCompleteMultiSourceRevisionVector(t *testing.T) {
	content := candidateTargetContent{
		application: argodomain.Application{
			Sources:           []argodomain.Source{{Name: "app"}, {Name: "values"}},
			ResolvedRevisions: []string{"commit-a", "commit-b"},
		},
		revision: "aggregate-commit", observedAt: time.Now(),
	}
	observation, err := newCandidateObservation(candidateTarget(), content, candidateRevisionEvidence())
	if err != nil {
		t.Fatalf("create multi-source observation: %v", err)
	}
	if !slices.Equal(observation.TargetRevisions, content.application.ResolvedRevisions) {
		t.Fatalf("multi-source revisions = %#v", observation.TargetRevisions)
	}
}

func candidateRevisionEvidence() candidateEvidence {
	return candidateEvidence{
		manifestHash: strings.Repeat("a", 64), diffHash: strings.Repeat("b", 64),
		diffs:  []deploydomain.ResourceDiffEvidence{{Kind: "Deployment", Name: "api"}},
		images: []deploydomain.ImageSnapshot{{ImageReference: "registry/api:v1", Digest: "sha256:approved"}},
	}
}

func TestApplicationCandidateIsRepositoryTopologyIndependent(t *testing.T) {
	target := candidateTarget()
	repository := newCandidateRepositoryStub(target)
	central := candidateArgoStub{source: argodomain.Source{RepositoryURL: "https://git.example/central.git", Path: "production/api", TargetRevision: "main"}}
	first := newCandidateReconcilerForTest(t, repository, central, &candidateWorkflowStarterStub{})
	if _, err := first.ReconcileOnce(context.Background()); err != nil {
		t.Fatalf("reconcile central repository: %v", err)
	}
	perApplication := candidateArgoStub{source: argodomain.Source{RepositoryURL: "https://git.example/api.git", Path: "deploy", TargetRevision: "main"}, revision: "commit-c"}
	second := newCandidateReconcilerForTest(t, repository, perApplication, &candidateWorkflowStarterStub{})
	result, err := second.ReconcileOnce(context.Background())
	if err != nil {
		t.Fatalf("reconcile per-Application repository: %v", err)
	}
	if result.DuplicateCandidates != 1 || len(repository.observations) != 1 {
		t.Fatalf("repository topology created a new candidate: %#v observations=%d", result, len(repository.observations))
	}
}

func TestCandidateReconciliationIsDuplicateSafeAfterRestart(t *testing.T) {
	repository := newCandidateRepositoryStub(candidateTarget())
	if _, err := newCandidateReconcilerForTest(t, repository, candidateArgoStub{}, &candidateWorkflowStarterStub{}).ReconcileOnce(context.Background()); err != nil {
		t.Fatalf("first reconciliation: %v", err)
	}
	result, err := newCandidateReconcilerForTest(t, repository, candidateArgoStub{}, &candidateWorkflowStarterStub{}).ReconcileOnce(context.Background())
	if err != nil {
		t.Fatalf("reconciliation after restart: %v", err)
	}
	if result.DuplicateCandidates != 1 || result.CreatedRequests != 0 || len(repository.observations) != 1 {
		t.Fatalf("restart duplicated request: %#v observations=%d", result, len(repository.observations))
	}
}

func TestCandidateReconciliationSkipsApplicationsWithoutLiveDifference(t *testing.T) {
	repository := newCandidateRepositoryStub(candidateTarget())
	argo := candidateArgoStub{diffs: []argodomain.ResourceDiff{{Kind: "Deployment", Name: "api", Modified: false}}}
	result, err := newCandidateReconcilerForTest(t, repository, argo, &candidateWorkflowStarterStub{}).ReconcileOnce(context.Background())
	if err != nil || result.CreatedRequests != 0 || len(repository.observations) != 0 {
		t.Fatalf("unchanged Application created request: %#v %v", result, err)
	}
}

func TestCandidateReconciliationRecoversApprovedReviews(t *testing.T) {
	workflows := &candidateWorkflowStarterStub{}
	reconciler := newCandidateReconcilerForTest(t, newCandidateRepositoryStub(), candidateArgoStub{}, workflows)
	if _, err := reconciler.ReconcileOnce(context.Background()); err != nil {
		t.Fatalf("reconcile approved reviews: %v", err)
	}
	if workflows.recoveries != 1 {
		t.Fatalf("approved review recoveries = %d, want 1", workflows.recoveries)
	}
}

func TestCandidateReconciliationRejectsEmptyObservedRevision(t *testing.T) {
	repository := newCandidateRepositoryStub(candidateTarget())
	argo := candidateArgoStub{emptyRevision: true}
	result, err := newCandidateReconcilerForTest(t, repository, argo, &candidateWorkflowStarterStub{}).ReconcileOnce(context.Background())
	if err == nil || result.CreatedRequests != 0 || len(repository.observations) != 0 {
		t.Fatalf("empty observed revision did not fail closed: result=%#v observations=%d err=%v", result, len(repository.observations), err)
	}
}

func newCandidateReconcilerForTest(t *testing.T, repository CandidateRepository, argo CandidateArgoReader, workflows CandidateWorkflowStarter) *CandidateReconciler {
	t.Helper()
	reconciler, err := NewCandidateReconciler(CandidateReconcilerOptions{
		Repository: repository, Argo: argo, Images: candidateImageLockerStub{},
		Observer: candidateObserverStub{}, Workflows: workflows, Interval: time.Minute,
	})
	if err != nil {
		t.Fatalf("create candidate reconciler: %v", err)
	}
	reconciler.now = func() time.Time { return time.Date(2026, 9, 3, 1, 2, 3, 0, time.UTC) }
	return reconciler
}

func candidateTarget() CandidateTarget {
	return CandidateTarget{
		ApplicationID: uuid.New(), OrganizationID: uuid.New(), ProjectID: uuid.New(), EnvironmentID: uuid.New(),
		ApplicationKey: "api", Argo: argodomain.ApplicationIdentity{Namespace: "argocd", Name: "api-production"},
		ArgoProject: "api", WorkflowVersionID: uuid.New(), PlanVersionID: uuid.New(),
	}
}

type candidateRepositoryStub struct {
	targets      []CandidateTarget
	observations []deploydomain.CandidateObservation
	seen         map[string]struct{}
	pending      []PendingWorkflowStart
}

func newCandidateRepositoryStub(targets ...CandidateTarget) *candidateRepositoryStub {
	return &candidateRepositoryStub{targets: targets, seen: make(map[string]struct{})}
}

func (s *candidateRepositoryStub) ListCandidateTargets(context.Context) ([]CandidateTarget, error) {
	return s.targets, nil
}

func (s *candidateRepositoryStub) CreateDeploymentRequest(_ context.Context, value deploydomain.CandidateObservation) (bool, error) {
	key := value.ApplicationID.String() + ":" + value.Fingerprint
	if _, exists := s.seen[key]; exists {
		return false, nil
	}
	s.seen[key] = struct{}{}
	s.observations = append(s.observations, value)
	s.pending = append(s.pending, PendingWorkflowStart{RequestID: uuid.New(), RequestVersionID: uuid.New()})
	return true, nil
}

func (s *candidateRepositoryStub) ListPendingWorkflowStarts(context.Context) ([]PendingWorkflowStart, error) {
	values := s.pending
	s.pending = nil
	return values, nil
}

type candidateWorkflowStarterStub struct {
	started    []WorkflowStartInput
	recoveries int
}

func (s *candidateWorkflowStarterStub) Start(_ context.Context, input WorkflowStartInput) (deploydomain.WorkflowResult, error) {
	s.started = append(s.started, input)
	return deploydomain.WorkflowResult{}, nil
}

func (s *candidateWorkflowStarterStub) RecoverApprovedReviews(context.Context) error {
	s.recoveries++
	return nil
}

type candidateArgoStub struct {
	source             argodomain.Source
	revision           string
	diffs              []argodomain.ResourceDiff
	requestedRevisions *[]string
	emptyRevision      bool
}

func (s candidateArgoStub) GetApplication(context.Context, argodomain.ApplicationIdentity, string) (argodomain.Application, error) {
	source := s.source
	if source.RepositoryURL == "" {
		source = argodomain.Source{RepositoryURL: "https://git.example/central.git", TargetRevision: "main", Path: "production/api"}
	}
	revision := s.revision
	if revision == "" && !s.emptyRevision {
		revision = "commit-b"
	}
	return argodomain.NewApplication(argodomain.Application{
		Identity: argodomain.ApplicationIdentity{Namespace: "argocd", Name: "api-production"},
		Labels:   map[string]string{argodomain.ManagedLabelKey: argodomain.ManagedLabelValue}, Sources: []argodomain.Source{source}, SyncStatus: "OutOfSync",
		ResolvedRevision: revision,
	})
}

func (s candidateArgoStub) GetTargetManifestsAtRevision(_ context.Context, _ argodomain.ApplicationIdentity, _ string, revision string) ([]string, string, error) {
	if s.requestedRevisions != nil {
		*s.requestedRevisions = append(*s.requestedRevisions, revision)
	}
	return []string{"apiVersion: apps/v1\nkind: Deployment\nmetadata:\n  name: api\nspec:\n  template:\n    spec:\n      containers:\n        - name: api\n          image: 123456789012.dkr.ecr.ap-northeast-1.amazonaws.com/platform/api:v1\n"}, revision, nil
}

func (s candidateArgoStub) GetManagedResourceDiffs(context.Context, argodomain.ApplicationIdentity, string) ([]argodomain.ResourceDiff, error) {
	if s.diffs != nil {
		return s.diffs, nil
	}
	return []argodomain.ResourceDiff{{
		Group: "apps", Kind: "Deployment", Namespace: "api", Name: "api", Modified: true,
		NormalizedLiveState: `{"spec":{"replicas":1}}`, PredictedLiveState: `{"spec":{"replicas":2}}`,
	}}, nil
}

type candidateImageLockerStub struct{}

func (candidateImageLockerStub) Lock(_ context.Context, applicationID uuid.UUID, revision string, _ []string) ([]ecrdomain.DigestSnapshot, error) {
	if applicationID == uuid.Nil || revision == "" {
		return nil, errors.New("invalid image lock input")
	}
	return []ecrdomain.DigestSnapshot{{
		ApplicationID: applicationID, ManifestRevision: revision,
		ImageReference: "123456789012.dkr.ecr.ap-northeast-1.amazonaws.com/platform/api:v1",
		Registry:       "123456789012.dkr.ecr.ap-northeast-1.amazonaws.com", Repository: "platform/api", Tag: "v1",
		ResolvedDigest: "sha256:aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa", Status: ecrdomain.DigestAvailable,
	}}, nil
}

type candidateObserverStub struct{}

func (candidateObserverStub) ObserveCandidateReconciliation(string, time.Duration, CandidateResult, error) {
}
