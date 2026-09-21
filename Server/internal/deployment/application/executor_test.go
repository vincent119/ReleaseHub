package application

import (
	"context"
	"errors"
	"slices"
	"sync"
	"testing"
	"time"

	"github.com/google/uuid"

	argodomain "github.com/vincent119/ReleaseHub/Server/internal/argocd/domain"
	deploydomain "github.com/vincent119/ReleaseHub/Server/internal/deployment/domain"
	ecrdomain "github.com/vincent119/ReleaseHub/Server/internal/ecr/domain"
)

func TestDeploymentSyncsPinnedRevision(t *testing.T) {
	executor, argo, repository := newExecutorFixture(t, "sha256:approved")
	if err := executor.Execute(context.Background(), repository.snapshot.RequestVersionID); err != nil {
		t.Fatalf("Execute(): %v", err)
	}
	if len(argo.syncs) != 1 || argo.syncs[0].Revision != "approved-commit" {
		t.Fatalf("executor did not pin the reviewed revision: %#v", argo.syncs)
	}
	if revision := argo.targetManifestRevision(); revision != "latest-commit" {
		t.Fatalf("preflight manifests were not pinned to the refreshed revision: %q", revision)
	}
	if len(repository.updates) != 2 || repository.updates[1].Status != "Succeeded" {
		t.Fatalf("node result was not persisted: %#v", repository.updates)
	}
}

func TestDeploymentBlockedOnDigestDriftWithoutSync(t *testing.T) {
	executor, argo, repository := newExecutorFixture(t, "sha256:changed")
	if err := executor.Execute(context.Background(), repository.snapshot.RequestVersionID); err != nil {
		t.Fatalf("business preflight block must complete the queue attempt: %v", err)
	}
	if len(argo.syncs) != 0 || repository.blockCode != "image_digest_drift" {
		t.Fatalf("digest drift must block before Sync: syncs=%d code=%q", len(argo.syncs), repository.blockCode)
	}
}

func TestDeploymentExecutorWaitsForUpstreamBeforeDownstream(t *testing.T) {
	executor, argo, repository := newExecutorFixture(t, "sha256:approved")
	upstream := repository.snapshot.Targets[0]
	upstream.Node.Key, upstream.Node.ApplicationKey = "a", "a"
	upstream.Preflight.Snapshot.ApplicationKey = "a"
	downstream := upstream
	downstream.Node.Key, downstream.Node.ApplicationKey = "b", "b"
	downstream.Preflight.Snapshot.ID = uuid.New()
	downstream.Preflight.Snapshot.ApplicationID = uuid.New()
	downstream.Preflight.Snapshot.ApplicationKey = "b"
	downstream.Preflight.Identity.Name = "b-production"
	repository.snapshot.Targets = []ExecutionTarget{upstream, downstream}
	repository.snapshot.Plan = deploydomain.DeploymentPlanDocument{
		Nodes: []deploydomain.DeploymentPlanNode{upstream.Node, downstream.Node},
		Edges: []deploydomain.DeploymentPlanEdge{{From: "a", To: "b", Condition: deploydomain.PlanEdgeUpstreamSucceeded}},
	}
	if err := executor.Execute(context.Background(), repository.snapshot.RequestVersionID); err != nil {
		t.Fatalf("Execute(): %v", err)
	}
	if len(argo.syncs) != 2 || argo.syncs[0].Identity.Name != "api-production" || argo.syncs[1].Identity.Name != "b-production" {
		t.Fatalf("DAG execution order mismatch: %#v", argo.syncs)
	}
}

func TestWatchInterruptionReconcilesWithoutBlindSync(t *testing.T) {
	executor, argo, repository := newExecutorFixture(t, "sha256:approved")
	argo.syncPending = true
	if err := executor.Execute(context.Background(), repository.snapshot.RequestVersionID); err != nil {
		t.Fatalf("Execute(): %v", err)
	}
	if len(argo.syncs) != 1 || argo.getCalls != 1 {
		t.Fatalf("watch recovery must reconcile without another Sync: syncs=%d gets=%d", len(argo.syncs), argo.getCalls)
	}
}

func TestDeploymentExecutorReconcilesAfterWatchTimeout(t *testing.T) {
	executor, argo, repository := newExecutorFixture(t, "sha256:approved")
	argo.syncPending = true
	watch := newBlockingApplicationWatch()
	argo.watch = watch
	executor.watchTimeout = time.Millisecond
	if err := executor.Execute(context.Background(), repository.snapshot.RequestVersionID); err != nil {
		t.Fatalf("execute deployment after watch timeout: %v", err)
	}
	if len(argo.syncs) != 1 || argo.getCalls != 1 || !watch.closed {
		t.Fatalf("watch timeout reconciliation = syncs %d, gets %d, closed %t", len(argo.syncs), argo.getCalls, watch.closed)
	}
	if len(repository.updates) != 2 || repository.updates[1].Status != "Succeeded" {
		t.Fatalf("watch timeout did not persist success: %#v", repository.updates)
	}
}

func TestDeploymentExecutorRejectsDifferentOperationAfterWatchTimeout(t *testing.T) {
	executor, argo, repository := newExecutorFixture(t, "sha256:approved")
	argo.syncPending = true
	argo.operationID = "another-operation"
	argo.watch = newBlockingApplicationWatch()
	executor.watchTimeout = time.Millisecond
	if err := executor.Execute(context.Background(), repository.snapshot.RequestVersionID); err != nil {
		t.Fatalf("different operation should be a business failure: %v", err)
	}
	if len(argo.syncs) != 1 || len(repository.updates) != 2 || repository.updates[1].ErrorCode != "watch_failed" {
		t.Fatalf("different operation was not rejected: syncs=%d updates=%#v", len(argo.syncs), repository.updates)
	}
}

func TestDeploymentExecutorAcceptsEquivalentManifestsAtAdvancedRevision(t *testing.T) {
	executor, argo, repository := newExecutorFixture(t, "sha256:approved")
	argo.syncPending = true
	argo.resolvedRevision = "newer-commit"
	argo.watch = newBlockingApplicationWatch()
	executor.watchTimeout = time.Millisecond
	if err := executor.Execute(context.Background(), repository.snapshot.RequestVersionID); err != nil {
		t.Fatalf("equivalent manifests should complete deployment: %v", err)
	}
	if len(argo.syncs) != 1 || argo.getCalls == 0 || len(repository.updates) != 2 || repository.updates[1].Status != "Succeeded" {
		t.Fatalf("equivalent manifests did not succeed: syncs=%d gets=%d updates=%#v", len(argo.syncs), argo.getCalls, repository.updates)
	}
	if update := repository.updates[1]; update.ActualRevision != "newer-commit" {
		t.Fatalf("success did not preserve actual revision: %#v", update)
	}
	if !slices.Contains(argo.targetManifestRevisions(), "newer-commit") {
		t.Fatalf("completion did not verify the actual revision: %#v", argo.targetManifestRevisions())
	}
	if countRevision(argo.targetManifestRevisions(), "newer-commit") != 1 {
		t.Fatalf("completion repeated manifest verification: %#v", argo.targetManifestRevisions())
	}
}

func TestDeploymentExecutorRejectsChangedManifestsAtAdvancedRevision(t *testing.T) {
	testDeploymentExecutorRejectsChangedManifestsAtAdvancedRevision(t)
}

func TestDeploymentExecutorRejectsCompletedOperationWithTargetRevisionMismatch(t *testing.T) {
	testDeploymentExecutorRejectsChangedManifestsAtAdvancedRevision(t)
}

func testDeploymentExecutorRejectsChangedManifestsAtAdvancedRevision(t *testing.T) {
	t.Helper()
	executor, argo, repository := newExecutorFixture(t, "sha256:approved")
	argo.syncPending = true
	argo.resolvedRevision = "newer-commit"
	argo.manifestsByRevision = map[string][]string{"newer-commit": {"apiVersion: v1\nkind: Service\n"}}
	argo.watch = newBlockingApplicationWatch()
	executor.watchTimeout = time.Millisecond
	if err := executor.Execute(context.Background(), repository.snapshot.RequestVersionID); err != nil {
		t.Fatalf("revision mismatch should be a business failure: %v", err)
	}
	assertTargetRevisionMismatch(t, argo, repository)
}

func TestDeploymentExecutorFailsClosedWhenAdvancedRevisionCannotBeVerified(t *testing.T) {
	tests := map[string]func(*deploymentArgoStub){
		"query error": func(argo *deploymentArgoStub) {
			argo.manifestErrors = map[string]error{"newer-commit": errors.New("manifest unavailable")}
		},
		"revision mismatch": func(argo *deploymentArgoStub) {
			argo.manifestResolvedRevisions = map[string]string{"newer-commit": "different-commit"}
		},
		"invalid manifests": func(argo *deploymentArgoStub) {
			argo.manifestsByRevision = map[string][]string{"newer-commit": {"apiVersion: ["}}
		},
	}
	for name, configure := range tests {
		t.Run(name, func(t *testing.T) {
			executor, argo, repository := newExecutorFixture(t, "sha256:approved")
			argo.syncPending = true
			argo.resolvedRevision = "newer-commit"
			configure(argo)
			argo.watch = newBlockingApplicationWatch()
			executor.watchTimeout = time.Millisecond
			if err := executor.Execute(context.Background(), repository.snapshot.RequestVersionID); err != nil {
				t.Fatalf("unverifiable revision should be a business failure: %v", err)
			}
			assertTargetRevisionMismatch(t, argo, repository)
		})
	}
}

func TestDeploymentExecutorKeepsMultiSourceRevisionMismatchFailClosed(t *testing.T) {
	executor, argo, repository := newExecutorFixture(t, "sha256:approved")
	repository.snapshot.Targets[0].Preflight.Snapshot.TargetRevision = ""
	repository.snapshot.Targets[0].Preflight.Snapshot.TargetRevisions = []string{"approved-a", "approved-b"}
	argo.syncPending = true
	argo.resolvedRevisions = []string{"newer-a", "approved-b"}
	argo.watch = newBlockingApplicationWatch()
	executor.watchTimeout = time.Millisecond
	if err := executor.Execute(context.Background(), repository.snapshot.RequestVersionID); err != nil {
		t.Fatalf("multi-source mismatch should be a business failure: %v", err)
	}
	assertTargetRevisionMismatch(t, argo, repository)
	if slices.Contains(argo.targetManifestRevisions(), "newer-a") {
		t.Fatalf("multi-source mismatch used a partial manifest query: %#v", argo.targetManifestRevisions())
	}
}

func assertTargetRevisionMismatch(t *testing.T, argo *deploymentArgoStub, repository *executionRepositoryStub) {
	t.Helper()
	if len(argo.syncs) != 1 || argo.getCalls == 0 || len(repository.updates) != 2 || repository.updates[1].ErrorCode != "target_revision_mismatch" {
		t.Fatalf("completed operation revision mismatch was not rejected: syncs=%d gets=%d updates=%#v", len(argo.syncs), argo.getCalls, repository.updates)
	}
	if update := repository.updates[1]; update.SyncStatus != "Synced" || update.HealthStatus != "Healthy" {
		t.Fatalf("revision mismatch did not preserve deployment evidence: %#v", update)
	}
	if len(repository.snapshot.Targets[0].Preflight.Snapshot.TargetRevisions) == 0 && repository.updates[1].ActualRevision != "newer-commit" {
		t.Fatalf("revision mismatch did not preserve actual revision: %#v", repository.updates[1])
	}
	if repository.completed != deploydomain.ExecutionFailed {
		t.Fatalf("revision mismatch execution status = %q", repository.completed)
	}
}

func countRevision(values []string, target string) int {
	count := 0
	for _, value := range values {
		if value == target {
			count++
		}
	}
	return count
}

func TestWorkerRestartResumesPersistedOperationWithoutSync(t *testing.T) {
	executor, argo, repository := newExecutorFixture(t, "sha256:approved")
	target := &repository.snapshot.Targets[0]
	repository.snapshot.Status = "Running"
	target.OperationID = repository.snapshot.ID.String() + ":" + target.Node.Key
	argo.operationID = target.OperationID
	argo.resolvedRevision = "approved-commit"
	if err := executor.Execute(context.Background(), repository.snapshot.RequestVersionID); err != nil {
		t.Fatalf("Execute(): %v", err)
	}
	if len(argo.syncs) != 0 || argo.getCalls != 1 {
		t.Fatalf("restart must resume operation without Sync: syncs=%d gets=%d", len(argo.syncs), argo.getCalls)
	}
}

func TestPartialFailedHoldsOperationLock(t *testing.T) {
	executor, argo, repository := newExecutorFixture(t, "sha256:approved")
	first := repository.snapshot.Targets[0]
	second := first
	second.Node.Key, second.Node.ApplicationKey = "worker", "worker"
	second.Preflight.Snapshot.ID, second.Preflight.Snapshot.ApplicationID = uuid.New(), uuid.New()
	second.Preflight.Snapshot.ApplicationKey = "worker"
	second.Preflight.Identity.Name = "worker-production"
	repository.snapshot.Targets = []ExecutionTarget{first, second}
	repository.snapshot.Plan.Nodes = []deploydomain.DeploymentPlanNode{first.Node, second.Node}
	argo.failName = second.Preflight.Identity.Name
	if err := executor.Execute(context.Background(), repository.snapshot.RequestVersionID); err != nil {
		t.Fatalf("Execute(): %v", err)
	}
	if repository.completed != deploydomain.ExecutionPartialFailed || repository.locks.released {
		t.Fatalf("partial result must retain locks: status=%q released=%t", repository.completed, repository.locks.released)
	}
}

func newExecutorFixture(t *testing.T, actualDigest string) (*DeploymentExecutor, *deploymentArgoStub, *executionRepositoryStub) {
	t.Helper()
	applicationID, requestApplicationID := uuid.New(), uuid.New()
	manifests := []string{"apiVersion: v1\nkind: Pod\nspec:\n  containers:\n    - image: registry/repository:v1\n"}
	manifestHash, _ := HashTargetManifests(manifests)
	_, diffHash, _ := NormalizeResourceDiffs(nil)
	target := testExecutionTarget(applicationID, requestApplicationID, manifestHash, diffHash)
	repository := &executionRepositoryStub{snapshot: testExecutionSnapshot(target)}
	argo := &deploymentArgoStub{manifests: manifests}
	images := &imageLockerStub{values: []ecrdomain.DigestSnapshot{{
		ApplicationID: applicationID, ManifestRevision: "latest-commit",
		ImageReference: "registry/repository:v1", Registry: "registry", Repository: "repository",
		Tag: "v1", ResolvedDigest: actualDigest, Status: ecrdomain.DigestAvailable,
	}}}
	preflight, _ := NewPreflightService(argo, images)
	locks := &applicationLocksStub{}
	repository.locks = locks
	executor, err := NewDeploymentExecutor(DeploymentExecutorOptions{
		Repository: repository, Preflight: preflight, Argo: argo,
		Locks: locks, MaxParallel: 2, LockTTL: time.Minute, WatchTimeout: time.Second,
	})
	if err != nil {
		t.Fatalf("NewDeploymentExecutor(): %v", err)
	}
	return executor, argo, repository
}

func testExecutionTarget(applicationID, requestApplicationID uuid.UUID, manifestHash, diffHash string) ExecutionTarget {
	condition := deploydomain.DeploymentPlanCondition{SyncStatuses: []string{"Synced"}, HealthStatuses: []string{"Healthy"}}
	return ExecutionTarget{
		Node: deploydomain.DeploymentPlanNode{Key: "api", ApplicationKey: "api", SuccessCondition: condition},
		Preflight: PreflightTarget{
			Identity: argodomain.ApplicationIdentity{Namespace: "argocd", Name: "api-production"}, ArgoProject: "api",
			Snapshot: deploydomain.DeploymentRequestApplicationSnapshot{
				ID: requestApplicationID, ApplicationID: applicationID, ApplicationKey: "api",
				TargetRevision: "approved-commit", ManifestHash: manifestHash, DiffHash: diffHash,
				Images: []deploydomain.DeploymentRequestImageSnapshot{{
					ImageReference: "registry/repository:v1", Registry: "registry", Repository: "repository",
					Tag: "v1", Digest: "sha256:approved",
				}},
			},
		},
	}
}

func testExecutionSnapshot(target ExecutionTarget) ExecutionSnapshot {
	return ExecutionSnapshot{
		ID: uuid.New(), RequestVersionID: uuid.New(), Status: "Preflight", Targets: []ExecutionTarget{target},
		Plan: deploydomain.DeploymentPlanDocument{Nodes: []deploydomain.DeploymentPlanNode{target.Node}},
	}
}

type executionRepositoryStub struct {
	mu        sync.Mutex
	snapshot  ExecutionSnapshot
	updates   []ExecutionNodeUpdate
	blockCode string
	completed deploydomain.ExecutionStatus
	locks     *applicationLocksStub
}

func (s *executionRepositoryStub) Prepare(context.Context, uuid.UUID, time.Time) (ExecutionSnapshot, error) {
	return s.snapshot, nil
}
func (s *executionRepositoryStub) Load(context.Context, uuid.UUID) (ExecutionSnapshot, error) {
	return s.snapshot, nil
}
func (s *executionRepositoryStub) MarkRunning(context.Context, uuid.UUID, time.Time) error {
	return nil
}
func (s *executionRepositoryStub) UpdateNode(_ context.Context, value ExecutionNodeUpdate) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.updates = append(s.updates, value)
	return nil
}
func (s *executionRepositoryStub) Complete(context.Context, uuid.UUID, time.Time) (deploydomain.ExecutionStatus, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	statuses := make(map[uuid.UUID]string, len(s.snapshot.Targets))
	for _, target := range s.snapshot.Targets {
		statuses[target.Preflight.Snapshot.ApplicationID] = target.Status
	}
	for _, update := range s.updates {
		statuses[update.ApplicationID] = update.Status
	}
	values := make([]string, 0, len(statuses))
	for _, status := range statuses {
		values = append(values, status)
	}
	result, err := deploydomain.AggregateExecutionStatus(values)
	s.completed = result
	return result, err
}
func (s *executionRepositoryStub) Block(_ context.Context, value ExecutionBlock) error {
	s.blockCode = value.Code
	return nil
}

type deploymentArgoStub struct {
	mu                        sync.Mutex
	manifests                 []string
	syncs                     []argodomain.SyncRequest
	syncPending               bool
	watch                     *blockingApplicationWatch
	operationID               string
	refreshRevision           string
	resolvedRevision          string
	resolvedRevisions         []string
	manifestRevision          string
	manifestRevisions         []string
	manifestsByRevision       map[string][]string
	manifestErrors            map[string]error
	manifestResolvedRevisions map[string]string
	getCalls                  int
	failName                  string
}

func (s *deploymentArgoStub) HardRefreshApplication(context.Context, argodomain.ApplicationIdentity, string) (argodomain.Application, error) {
	revision := s.refreshRevision
	if revision == "" {
		revision = "latest-commit"
	}
	return argodomain.Application{ResolvedRevision: revision}, nil
}
func (s *deploymentArgoStub) GetTargetManifestsAtRevision(_ context.Context, _ argodomain.ApplicationIdentity, _ string, revision string) ([]string, string, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.manifestRevision = revision
	s.manifestRevisions = append(s.manifestRevisions, revision)
	if err := s.manifestErrors[revision]; err != nil {
		return nil, "", err
	}
	if manifests, ok := s.manifestsByRevision[revision]; ok {
		return manifests, s.manifestResolvedRevision(revision), nil
	}
	return s.manifests, s.manifestResolvedRevision(revision), nil
}

func (s *deploymentArgoStub) manifestResolvedRevision(revision string) string {
	if resolved, ok := s.manifestResolvedRevisions[revision]; ok {
		return resolved
	}
	return revision
}

func (s *deploymentArgoStub) targetManifestRevision() string {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.manifestRevision
}
func (s *deploymentArgoStub) targetManifestRevisions() []string {
	s.mu.Lock()
	defer s.mu.Unlock()
	return slices.Clone(s.manifestRevisions)
}
func (s *deploymentArgoStub) GetManagedResourceDiffs(context.Context, argodomain.ApplicationIdentity, string) ([]argodomain.ResourceDiff, error) {
	return nil, nil
}
func (s *deploymentArgoStub) GetApplication(context.Context, argodomain.ApplicationIdentity, string) (argodomain.Application, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.getCalls++
	operationID := s.operationID
	if operationID == "" && len(s.syncs) > 0 {
		operationID = s.syncs[len(s.syncs)-1].OperationID
	}
	revision := s.resolvedRevision
	if revision == "" && len(s.syncs) > 0 {
		revision = s.syncs[len(s.syncs)-1].Revision
	}
	return argodomain.Application{OperationID: operationID, OperationPhase: "Succeeded",
		SyncStatus: "Synced", HealthStatus: "Healthy", ResolvedRevision: revision,
		ResolvedRevisions: slices.Clone(s.resolvedRevisions), ResourceVersion: "3"}, nil
}
func (s *deploymentArgoStub) SyncApplication(_ context.Context, value argodomain.SyncRequest) (argodomain.Application, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.syncs = append(s.syncs, value)
	if value.Identity.Name == s.failName {
		return argodomain.Application{}, errors.New("sync rejected")
	}
	if s.syncPending {
		return argodomain.Application{OperationID: value.OperationID, OperationPhase: "Running", ResourceVersion: "2"}, nil
	}
	return argodomain.Application{
		OperationID: value.OperationID, OperationPhase: "Succeeded", SyncStatus: "Synced",
		HealthStatus: "Healthy", ResolvedRevision: value.Revision, ResourceVersion: "2",
	}, nil
}
func (s *deploymentArgoStub) WatchApplication(context.Context, argodomain.ApplicationIdentity, string, string) (argodomain.ApplicationWatch, error) {
	if s.watch != nil {
		return s.watch, nil
	}
	return nil, errors.New("unexpected watch")
}

type blockingApplicationWatch struct {
	done   chan struct{}
	closed bool
	once   sync.Once
}

func newBlockingApplicationWatch() *blockingApplicationWatch {
	return &blockingApplicationWatch{done: make(chan struct{})}
}

func (w *blockingApplicationWatch) Recv() (argodomain.Application, error) {
	<-w.done
	return argodomain.Application{}, context.Canceled
}

func (w *blockingApplicationWatch) Close() {
	w.once.Do(func() {
		w.closed = true
		close(w.done)
	})
}

type imageLockerStub struct{ values []ecrdomain.DigestSnapshot }

func (s *imageLockerStub) Lock(context.Context, uuid.UUID, string, []string) ([]ecrdomain.DigestSnapshot, error) {
	return s.values, nil
}

type applicationLocksStub struct{ released bool }

func (applicationLocksStub) Acquire(_ context.Context, request deploydomain.ApplicationLockRequest) (deploydomain.ApplicationLockSet, error) {
	return deploydomain.ApplicationLockSet{Leases: []deploydomain.ApplicationLockLease{{
		ApplicationID: request.ApplicationIDs[0], ExecutionID: request.ExecutionID,
	}}}, nil
}
func (applicationLocksStub) Heartbeat(context.Context, deploydomain.ApplicationLockSet, time.Time) (deploydomain.ApplicationLockSet, error) {
	return deploydomain.ApplicationLockSet{}, nil
}

func (s *applicationLocksStub) Release(context.Context, deploydomain.ApplicationLockSet) error {
	s.released = true
	return nil
}
