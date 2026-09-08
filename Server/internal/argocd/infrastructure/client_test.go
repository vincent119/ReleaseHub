package infrastructure

import (
	"context"
	"encoding/json"
	"errors"
	"io"
	"testing"
	"time"

	accountpkg "github.com/argoproj/argo-cd/v3/pkg/apiclient/account"
	applicationpkg "github.com/argoproj/argo-cd/v3/pkg/apiclient/application"
	argov1alpha1 "github.com/argoproj/argo-cd/v3/pkg/apis/application/v1alpha1"
	repositorypkg "github.com/argoproj/argo-cd/v3/reposerver/apiclient"
	"google.golang.org/grpc"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/metadata"
	"google.golang.org/grpc/status"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"

	argodomain "github.com/vincent119/ReleaseHub/Server/internal/argocd/domain"
)

func TestOfficialClientMapsApplicationWithoutRequestingMutation(t *testing.T) {
	enabled := true
	reconciledAt := metav1.NewTime(time.Date(2026, 9, 1, 8, 0, 0, 0, time.FixedZone("test", 8*60*60)))
	service := &applicationListerStub{response: &argov1alpha1.ApplicationList{Items: []argov1alpha1.Application{{
		ObjectMeta: metav1.ObjectMeta{
			Namespace: "argocd", Name: "admin-production", ResourceVersion: "42",
			Labels: map[string]string{"releasehub.io/managed": "true", "env": "Global"},
		},
		Spec: argov1alpha1.ApplicationSpec{
			Project: "admin", Sources: argov1alpha1.ApplicationSources{
				{RepoURL: "https://git.example.com/manifests.git", TargetRevision: "main", Path: "production/admin", Name: "primary"},
				{RepoURL: "https://git.example.com/values.git", TargetRevision: "stable", Ref: "values"},
			},
			Destination: argov1alpha1.ApplicationDestination{Server: "https://kubernetes.default.svc", Namespace: "admin"},
			SyncPolicy:  &argov1alpha1.SyncPolicy{Automated: &argov1alpha1.SyncPolicyAutomated{Enabled: &enabled}},
		},
		Status: argov1alpha1.ApplicationStatus{
			Sync:           argov1alpha1.SyncStatus{Status: "OutOfSync", Revision: "commit-a", Revisions: []string{"commit-a", "commit-b"}},
			Health:         argov1alpha1.AppHealthStatus{Status: "Healthy"},
			OperationState: &argov1alpha1.OperationState{Phase: "Succeeded"}, ReconciledAt: &reconciledAt,
		},
	}}}}
	closer := &closerStub{}
	client, err := newClientForTest(closer, service, "service-account-token", time.Second)
	if err != nil {
		t.Fatalf("create test client: %v", err)
	}

	applications, err := client.ListApplications(context.Background())
	if err != nil {
		t.Fatalf("list Applications: %v", err)
	}
	if len(applications) != 1 {
		t.Fatalf("expected one Application, got %d", len(applications))
	}
	value := applications[0]
	if !value.IsCandidate() || !value.AutomatedSync || len(value.Sources) != 2 {
		t.Fatalf("Application contract was not preserved: %#v", value)
	}
	if value.ResolvedRevision != "commit-a" || value.SyncStatus != "OutOfSync" || value.HealthStatus != "Healthy" || value.OperationPhase != "Succeeded" {
		t.Fatalf("status mapping mismatch: %#v", value)
	}
	if value.ReconciledAt == nil || value.ReconciledAt.Location() != time.UTC {
		t.Fatalf("reconciled time should be UTC: %#v", value.ReconciledAt)
	}
	if service.query == nil || service.query.Refresh != nil || service.query.Selector != nil {
		t.Fatalf("reconciliation must not request refresh, sync, or a lossy selector: %#v", service.query)
	}
	if service.token != "service-account-token" || !service.hadDeadline {
		t.Fatalf("token metadata or deadline was missing: token=%q deadline=%v", service.token, service.hadDeadline)
	}
	if err := client.Close(); err != nil || !closer.closed {
		t.Fatalf("close reusable connection: %v", err)
	}
}

func TestOfficialClientRetriesOnlyRetryableReadFailure(t *testing.T) {
	service := &applicationListerStub{
		response: &argov1alpha1.ApplicationList{},
		errors:   []error{status.Error(codes.Unavailable, "temporary"), nil},
	}
	client, err := newClientForTest(io.NopCloser(nilReader{}), service, "token", time.Second)
	if err != nil {
		t.Fatalf("create test client: %v", err)
	}
	if _, err := client.ListApplications(context.Background()); err != nil {
		t.Fatalf("retry read: %v", err)
	}
	if service.calls != 2 {
		t.Fatalf("expected one retry, got %d calls", service.calls)
	}

	service = &applicationListerStub{errors: []error{status.Error(codes.PermissionDenied, "denied")}}
	client, _ = newClientForTest(io.NopCloser(nilReader{}), service, "token", time.Second)
	if _, err := client.ListApplications(context.Background()); err == nil || service.calls != 1 {
		t.Fatalf("permission failure must not retry: calls=%d err=%v", service.calls, err)
	}
}

func TestOfficialClientPreservesExplicitlyDisabledAutomatedSync(t *testing.T) {
	disabled := false
	service := &applicationListerStub{response: &argov1alpha1.ApplicationList{Items: []argov1alpha1.Application{{
		ObjectMeta: metav1.ObjectMeta{Namespace: "argocd", Name: "disabled-automation"},
		Spec: argov1alpha1.ApplicationSpec{
			SyncPolicy: &argov1alpha1.SyncPolicy{Automated: &argov1alpha1.SyncPolicyAutomated{Enabled: &disabled}},
		},
	}}}}
	client, err := newClientForTest(io.NopCloser(nilReader{}), service, "token", time.Second)
	if err != nil {
		t.Fatalf("create test client: %v", err)
	}
	values, err := client.ListApplications(context.Background())
	if err != nil {
		t.Fatalf("list Applications: %v", err)
	}
	if len(values) != 1 || values[0].AutomatedSync {
		t.Fatalf("explicit automated.enabled=false must remain disabled: %#v", values)
	}
}

func TestManagementClientUsesExactReadPermissionAndAutomatedSyncPatch(t *testing.T) {
	manager := &applicationManagerStub{response: &argov1alpha1.Application{
		ObjectMeta: metav1.ObjectMeta{Namespace: "argocd", Name: "payment-production"},
		Spec:       argov1alpha1.ApplicationSpec{Project: "payment", SyncPolicy: &argov1alpha1.SyncPolicy{Automated: &argov1alpha1.SyncPolicyAutomated{}}},
	}}
	permissions := &permissionCheckerStub{response: &accountpkg.CanIResponse{Value: "yes"}}
	client, err := newManagementClientForTest(io.NopCloser(nilReader{}), manager, permissions, "token", time.Second, "argocd")
	if err != nil {
		t.Fatalf("create management client: %v", err)
	}
	identity := argodomain.ApplicationIdentity{Namespace: "argocd", Name: "payment-production"}
	value, err := client.GetApplication(context.Background(), identity, "payment")
	if err != nil || !value.AutomatedSync {
		t.Fatalf("read Application: %#v %v", value, err)
	}
	allowed, err := client.CanUpdateApplication(context.Background(), identity, "payment")
	if err != nil || !allowed {
		t.Fatalf("check update permission: %v %v", allowed, err)
	}
	if err := client.DisableAutomatedSync(context.Background(), identity, "payment"); err != nil {
		t.Fatalf("disable automated sync: %v", err)
	}
	if manager.getQuery == nil || manager.getQuery.Refresh != nil || manager.getQuery.GetAppNamespace() != "argocd" || manager.getQuery.GetProjects()[0] != "payment" {
		t.Fatalf("exact Get query mismatch: %#v", manager.getQuery)
	}
	if permissions.request == nil || permissions.request.Resource != "applications" || permissions.request.Action != "update" || permissions.request.Subresource != "payment/payment-production" {
		t.Fatalf("exact permission request mismatch: %#v", permissions.request)
	}
	if manager.patchRequest == nil || manager.patchRequest.GetPatchType() != "merge" || !json.Valid([]byte(manager.patchRequest.GetPatch())) {
		t.Fatalf("automated sync patch mismatch: %#v", manager.patchRequest)
	}
	if manager.patchRequest.GetPatch() != `{"spec":{"syncPolicy":{"automated":null}}}` {
		t.Fatalf("management client may only remove automated sync: %s", manager.patchRequest.GetPatch())
	}
}

func TestManagementClientReadsTargetManifestsWithoutMutation(t *testing.T) {
	verifyTargetManifestReadOnly(t)
}

func TestManagementClientReadsManagedResourceDiffsWithoutMutation(t *testing.T) {
	manager := &applicationManagerStub{resourcesResponse: &applicationpkg.ManagedResourcesResponse{Items: []*argov1alpha1.ResourceDiff{{
		Group: "apps", Kind: "Deployment", Namespace: "payment", Name: "api", Modified: true,
		NormalizedLiveState: `{"metadata":{"name":"api"}}`, PredictedLiveState: `{"metadata":{"name":"api"},"spec":{"replicas":2}}`,
	}}}}
	client, err := newManagementClientForTest(io.NopCloser(nilReader{}), manager, &permissionCheckerStub{}, "token", time.Second, "argocd")
	if err != nil {
		t.Fatalf("create management client: %v", err)
	}
	identity := argodomain.ApplicationIdentity{Namespace: "argocd", Name: "payment-production"}
	diffs, err := client.GetManagedResourceDiffs(context.Background(), identity, "payment")
	if err != nil || len(diffs) != 1 || !diffs[0].Modified {
		t.Fatalf("GetManagedResourceDiffs() = %#v, %v", diffs, err)
	}
	if manager.resourcesQuery == nil || manager.resourcesQuery.GetApplicationName() != identity.Name || manager.resourcesQuery.GetAppNamespace() != identity.Namespace || manager.resourcesQuery.GetProject() != "payment" {
		t.Fatalf("managed resources query mismatch: %#v", manager.resourcesQuery)
	}
	if manager.getQuery != nil || manager.patchRequest != nil {
		t.Fatal("managed resource read must not perform Application Get or Patch")
	}
}

func TestImageUpdaterResponsibilityBoundary(t *testing.T) {
	verifyTargetManifestReadOnly(t)
}

func TestPredeployRefreshIsReadOnly(t *testing.T) {
	manager := &applicationManagerStub{response: applicationResponse("55", "commit-b")}
	client, _ := newManagementClientForTest(io.NopCloser(nilReader{}), manager, &permissionCheckerStub{}, "token", time.Second, "argocd")
	identity := argodomain.ApplicationIdentity{Namespace: "argocd", Name: "payment-production"}
	value, err := client.HardRefreshApplication(context.Background(), identity, "payment")
	if err != nil || value.ResolvedRevision != "commit-b" {
		t.Fatalf("HardRefreshApplication() = %#v, %v", value, err)
	}
	if manager.getQuery.GetRefresh() != "hard" || manager.syncRequest != nil || manager.patchRequest != nil {
		t.Fatalf("preflight must only hard refresh: %#v", manager)
	}
}

func TestPredeployRefreshTimeoutFailsBeforeMutation(t *testing.T) {
	manager := &applicationManagerStub{getError: func(ctx context.Context) error {
		<-ctx.Done()
		return ctx.Err()
	}}
	client, _ := newManagementClientForTest(io.NopCloser(nilReader{}), manager, &permissionCheckerStub{}, "token", time.Millisecond, "argocd")
	identity := argodomain.ApplicationIdentity{Namespace: "argocd", Name: "payment-production"}
	_, err := client.HardRefreshApplication(context.Background(), identity, "payment")
	if !errors.Is(err, context.DeadlineExceeded) {
		t.Fatalf("HardRefreshApplication() error = %v", err)
	}
	if manager.syncRequest != nil || manager.patchRequest != nil {
		t.Fatal("timed out refresh must not perform Sync or Patch")
	}
}

func TestDeploymentSyncsPinnedRevision(t *testing.T) {
	manager := &applicationManagerStub{response: applicationResponse("56", "commit-a")}
	client, _ := newManagementClientForTest(io.NopCloser(nilReader{}), manager, &permissionCheckerStub{}, "token", time.Second, "argocd")
	request := argodomain.SyncRequest{
		Identity: argodomain.ApplicationIdentity{Namespace: "argocd", Name: "payment-production"},
		Project:  "payment", Revision: "commit-a", OperationID: "execution:node",
	}
	value, err := client.SyncApplication(context.Background(), request)
	if err != nil || value.ResolvedRevision != "commit-a" {
		t.Fatalf("SyncApplication() = %#v, %v", value, err)
	}
	if manager.syncRequest.GetRevision() != "commit-a" || manager.syncRequest.GetName() != request.Identity.Name {
		t.Fatalf("pinned Sync request mismatch: %#v", manager.syncRequest)
	}
	if manager.syncRequest.GetInfos()[0].Value != "execution:node" {
		t.Fatalf("operation identity is missing: %#v", manager.syncRequest.GetInfos())
	}
}

func TestDeploymentSyncsPinnedMultiSourceRevisions(t *testing.T) {
	manager := &applicationManagerStub{response: applicationResponse("56", "")}
	client, _ := newManagementClientForTest(io.NopCloser(nilReader{}), manager, &permissionCheckerStub{}, "token", time.Second, "argocd")
	request := argodomain.SyncRequest{
		Identity: argodomain.ApplicationIdentity{Namespace: "argocd", Name: "payment-production"},
		Project:  "payment", Revision: "moving-head", Revisions: []string{"commit-a", "commit-b"},
		OperationID: "execution:node",
	}
	if _, err := client.SyncApplication(context.Background(), request); err != nil {
		t.Fatalf("SyncApplication(): %v", err)
	}
	if manager.syncRequest.Revision != nil || len(manager.syncRequest.GetRevisions()) != 2 {
		t.Fatalf("multi-source Sync must use only pinned revisions: %#v", manager.syncRequest)
	}
}

func TestApplicationWatchMapsOperationIdentity(t *testing.T) {
	stream := &applicationWatchStub{events: []*argov1alpha1.ApplicationWatchEvent{{
		Application: *applicationResponse("57", "commit-a"),
	}}}
	stream.events[0].Application.Status.OperationState = operationState("execution:node")
	manager := &applicationManagerStub{watch: stream}
	client, _ := newManagementClientForTest(io.NopCloser(nilReader{}), manager, &permissionCheckerStub{}, "token", time.Second, "argocd")
	identity := argodomain.ApplicationIdentity{Namespace: "argocd", Name: "payment-production"}
	watch, err := client.WatchApplication(context.Background(), identity, "payment", "56")
	if err != nil {
		t.Fatalf("WatchApplication(): %v", err)
	}
	defer watch.Close()
	value, err := watch.Recv()
	if err != nil || value.OperationID != "execution:node" {
		t.Fatalf("watch observation = %#v, %v", value, err)
	}
	if manager.watchQuery.GetResourceVersion() != "56" {
		t.Fatalf("watch resource version mismatch: %#v", manager.watchQuery)
	}
}

func TestExecutionControlReadsImagesAndTerminatesOperation(t *testing.T) {
	manager := &applicationManagerStub{treeResponse: &argov1alpha1.ApplicationTree{Nodes: []argov1alpha1.ResourceNode{
		{Images: []string{"registry.example/api@sha256:bbbb", "registry.example/api@sha256:aaaa"}},
		{Images: []string{"registry.example/api@sha256:aaaa"}},
	}}}
	client, _ := newManagementClientForTest(io.NopCloser(nilReader{}), manager, &permissionCheckerStub{}, "token", time.Second, "argocd")
	identity := argodomain.ApplicationIdentity{Namespace: "argocd", Name: "payment-production"}
	images, err := client.GetApplicationImages(context.Background(), identity, "payment")
	if err != nil || len(images) != 2 || images[0] != "registry.example/api@sha256:aaaa" {
		t.Fatalf("GetApplicationImages() = %#v, %v", images, err)
	}
	if err := client.TerminateApplication(context.Background(), identity, "payment"); err != nil {
		t.Fatalf("TerminateApplication() = %v", err)
	}
	if manager.resourcesQuery.GetApplicationName() != identity.Name || manager.terminateRequest.GetProject() != "payment" {
		t.Fatalf("execution control Argo requests mismatch: %#v", manager)
	}
}

func verifyTargetManifestReadOnly(t *testing.T) {
	t.Helper()
	manager := &applicationManagerStub{manifestResponse: &repositorypkg.ManifestResponse{
		Manifests: []string{"apiVersion: v1\nkind: Pod\n"}, Revision: "commit-a",
	}}
	client, err := newManagementClientForTest(io.NopCloser(nilReader{}), manager, &permissionCheckerStub{}, "token", time.Second, "argocd")
	if err != nil {
		t.Fatalf("create management client: %v", err)
	}
	identity := argodomain.ApplicationIdentity{Namespace: "argocd", Name: "payment-production"}
	manifests, revision, err := client.GetTargetManifests(context.Background(), identity, "payment")
	if err != nil || revision != "commit-a" || len(manifests) != 1 {
		t.Fatalf("GetTargetManifests() = %#v, %q, %v", manifests, revision, err)
	}
	if manager.manifestQuery == nil || manager.manifestQuery.GetName() != identity.Name || manager.manifestQuery.GetAppNamespace() != identity.Namespace || manager.manifestQuery.GetProject() != "payment" || !manager.manifestQuery.GetNoCache() {
		t.Fatalf("target manifest query mismatch: %#v", manager.manifestQuery)
	}
	if manager.getQuery != nil || manager.patchRequest != nil {
		t.Fatalf("target manifest read must not perform Application Get or Patch")
	}
}

type applicationListerStub struct {
	response    *argov1alpha1.ApplicationList
	errors      []error
	calls       int
	query       *applicationpkg.ApplicationQuery
	token       string
	hadDeadline bool
}

type applicationManagerStub struct {
	response          *argov1alpha1.Application
	manifestResponse  *repositorypkg.ManifestResponse
	resourcesResponse *applicationpkg.ManagedResourcesResponse
	treeResponse      *argov1alpha1.ApplicationTree
	getQuery          *applicationpkg.ApplicationQuery
	manifestQuery     *applicationpkg.ApplicationManifestQuery
	resourcesQuery    *applicationpkg.ResourcesQuery
	patchRequest      *applicationpkg.ApplicationPatchRequest
	syncRequest       *applicationpkg.ApplicationSyncRequest
	watchQuery        *applicationpkg.ApplicationQuery
	watch             applicationpkg.ApplicationService_WatchClient
	terminateRequest  *applicationpkg.OperationTerminateRequest
	getError          func(context.Context) error
}

func (s *applicationManagerStub) Sync(_ context.Context, request *applicationpkg.ApplicationSyncRequest, _ ...grpc.CallOption) (*argov1alpha1.Application, error) {
	s.syncRequest = request
	return s.response, nil
}

func (s *applicationManagerStub) Watch(_ context.Context, query *applicationpkg.ApplicationQuery, _ ...grpc.CallOption) (applicationpkg.ApplicationService_WatchClient, error) {
	s.watchQuery = query
	return s.watch, nil
}

func (s *applicationManagerStub) Get(ctx context.Context, query *applicationpkg.ApplicationQuery, _ ...grpc.CallOption) (*argov1alpha1.Application, error) {
	s.getQuery = query
	if s.getError != nil {
		return nil, s.getError(ctx)
	}
	return s.response, nil
}

func (s *applicationManagerStub) GetManifests(_ context.Context, query *applicationpkg.ApplicationManifestQuery, _ ...grpc.CallOption) (*repositorypkg.ManifestResponse, error) {
	s.manifestQuery = query
	if s.manifestResponse == nil {
		return nil, errors.New("manifest response is required")
	}
	return s.manifestResponse, nil
}

func (s *applicationManagerStub) ManagedResources(_ context.Context, query *applicationpkg.ResourcesQuery, _ ...grpc.CallOption) (*applicationpkg.ManagedResourcesResponse, error) {
	s.resourcesQuery = query
	if s.resourcesResponse == nil {
		return &applicationpkg.ManagedResourcesResponse{}, nil
	}
	return s.resourcesResponse, nil
}

func (s *applicationManagerStub) ResourceTree(_ context.Context, query *applicationpkg.ResourcesQuery, _ ...grpc.CallOption) (*argov1alpha1.ApplicationTree, error) {
	s.resourcesQuery = query
	if s.treeResponse == nil {
		return &argov1alpha1.ApplicationTree{}, nil
	}
	return s.treeResponse, nil
}

func (s *applicationManagerStub) TerminateOperation(_ context.Context, request *applicationpkg.OperationTerminateRequest, _ ...grpc.CallOption) (*applicationpkg.OperationTerminateResponse, error) {
	s.terminateRequest = request
	return &applicationpkg.OperationTerminateResponse{}, nil
}

func (s *applicationManagerStub) Patch(_ context.Context, request *applicationpkg.ApplicationPatchRequest, _ ...grpc.CallOption) (*argov1alpha1.Application, error) {
	s.patchRequest = request
	return s.response, nil
}

type permissionCheckerStub struct {
	response *accountpkg.CanIResponse
	request  *accountpkg.CanIRequest
}

func (s *permissionCheckerStub) CanI(_ context.Context, request *accountpkg.CanIRequest, _ ...grpc.CallOption) (*accountpkg.CanIResponse, error) {
	s.request = request
	return s.response, nil
}

func (s *applicationListerStub) List(ctx context.Context, query *applicationpkg.ApplicationQuery, _ ...grpc.CallOption) (*argov1alpha1.ApplicationList, error) {
	s.calls++
	s.query = query
	_, s.hadDeadline = ctx.Deadline()
	if values, ok := metadata.FromOutgoingContext(ctx); ok {
		tokens := values.Get("token")
		if len(tokens) > 0 {
			s.token = tokens[0]
		}
	}
	if s.calls <= len(s.errors) && s.errors[s.calls-1] != nil {
		return nil, s.errors[s.calls-1]
	}
	if s.response == nil {
		return nil, errors.New("response is required")
	}
	return s.response, nil
}

type closerStub struct{ closed bool }

func (c *closerStub) Close() error { c.closed = true; return nil }

type nilReader struct{}

func (nilReader) Read([]byte) (int, error) { return 0, io.EOF }

type applicationWatchStub struct {
	applicationpkg.ApplicationService_WatchClient
	events []*argov1alpha1.ApplicationWatchEvent
}

func (s *applicationWatchStub) Recv() (*argov1alpha1.ApplicationWatchEvent, error) {
	if len(s.events) == 0 {
		return nil, io.EOF
	}
	value := s.events[0]
	s.events = s.events[1:]
	return value, nil
}

func applicationResponse(resourceVersion, revision string) *argov1alpha1.Application {
	return &argov1alpha1.Application{
		ObjectMeta: metav1.ObjectMeta{Namespace: "argocd", Name: "payment-production", ResourceVersion: resourceVersion},
		Spec:       argov1alpha1.ApplicationSpec{Project: "payment"},
		Status:     argov1alpha1.ApplicationStatus{Sync: argov1alpha1.SyncStatus{Revision: revision}},
	}
}

func operationState(operationID string) *argov1alpha1.OperationState {
	return &argov1alpha1.OperationState{
		Phase:     "Running",
		Operation: argov1alpha1.Operation{Sync: &argov1alpha1.SyncOperation{}, Info: []*argov1alpha1.Info{{Name: "releasehub.io/operation-id", Value: operationID}}},
	}
}
