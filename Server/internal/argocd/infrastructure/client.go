// Package infrastructure adapts the official Argo CD gRPC client to ReleaseHub contracts.
package infrastructure

import (
	"context"
	"crypto/tls"
	"errors"
	"fmt"
	"io"
	"slices"
	"strings"
	"time"

	accountpkg "github.com/argoproj/argo-cd/v3/pkg/apiclient/account"
	applicationpkg "github.com/argoproj/argo-cd/v3/pkg/apiclient/application"
	argov1alpha1 "github.com/argoproj/argo-cd/v3/pkg/apis/application/v1alpha1"
	repositorypkg "github.com/argoproj/argo-cd/v3/reposerver/apiclient"
	"go.opentelemetry.io/contrib/instrumentation/google.golang.org/grpc/otelgrpc"
	"google.golang.org/grpc"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/credentials"
	grpcinsecure "google.golang.org/grpc/credentials/insecure"
	"google.golang.org/grpc/metadata"
	"google.golang.org/grpc/status"
	corev1 "k8s.io/api/core/v1"

	argodomain "github.com/vincent119/ReleaseHub/Server/internal/argocd/domain"
	"github.com/vincent119/ReleaseHub/Server/internal/config"
)

const (
	readAttempts   = 3
	initialBackoff = 100 * time.Millisecond
)

type applicationLister interface {
	List(context.Context, *applicationpkg.ApplicationQuery, ...grpc.CallOption) (*argov1alpha1.ApplicationList, error)
}

type applicationManager interface {
	Get(context.Context, *applicationpkg.ApplicationQuery, ...grpc.CallOption) (*argov1alpha1.Application, error)
	GetManifests(context.Context, *applicationpkg.ApplicationManifestQuery, ...grpc.CallOption) (*repositorypkg.ManifestResponse, error)
	ManagedResources(context.Context, *applicationpkg.ResourcesQuery, ...grpc.CallOption) (*applicationpkg.ManagedResourcesResponse, error)
	ResourceTree(context.Context, *applicationpkg.ResourcesQuery, ...grpc.CallOption) (*argov1alpha1.ApplicationTree, error)
	TerminateOperation(context.Context, *applicationpkg.OperationTerminateRequest, ...grpc.CallOption) (*applicationpkg.OperationTerminateResponse, error)
	Patch(context.Context, *applicationpkg.ApplicationPatchRequest, ...grpc.CallOption) (*argov1alpha1.Application, error)
	Sync(context.Context, *applicationpkg.ApplicationSyncRequest, ...grpc.CallOption) (*argov1alpha1.Application, error)
	Watch(context.Context, *applicationpkg.ApplicationQuery, ...grpc.CallOption) (applicationpkg.ApplicationService_WatchClient, error)
}

type applicationRuntimeManager interface {
	ResourceTree(context.Context, *applicationpkg.ResourcesQuery, ...grpc.CallOption) (*argov1alpha1.ApplicationTree, error)
	GetResource(context.Context, *applicationpkg.ApplicationResourceRequest, ...grpc.CallOption) (*applicationpkg.ApplicationResourceResponse, error)
	ListResourceEvents(context.Context, *applicationpkg.ApplicationResourceEventsQuery, ...grpc.CallOption) (*corev1.EventList, error)
	PodLogs(context.Context, *applicationpkg.ApplicationPodLogsQuery, ...grpc.CallOption) (applicationpkg.ApplicationService_PodLogsClient, error)
}

// ApplicationWatch owns one authenticated Argo CD watch stream.
type ApplicationWatch struct {
	stream applicationpkg.ApplicationService_WatchClient
	cancel context.CancelFunc
}

type permissionChecker interface {
	CanI(context.Context, *accountpkg.CanIRequest, ...grpc.CallOption) (*accountpkg.CanIResponse, error)
}

// Client owns one reusable official Argo CD gRPC connection.
type Client struct {
	connection            io.Closer
	applications          applicationLister
	manager               applicationManager
	runtime               applicationRuntimeManager
	permissions           permissionChecker
	token                 string
	requestTimeout        time.Duration
	controlPlaneNamespace string
}

// NewClient creates an official Argo CD Application client with the configured transport and OpenTelemetry propagation.
func NewClient(cfg config.ArgoCDConfig) (*Client, error) {
	if err := cfg.Validate(); err != nil {
		return nil, err
	}
	connection, err := grpc.NewClient(
		cfg.Address,
		grpc.WithTransportCredentials(transportCredentials(cfg)),
		grpc.WithStatsHandler(otelgrpc.NewClientHandler()),
	)
	if err != nil {
		return nil, fmt.Errorf("create Argo CD gRPC connection: %w", err)
	}
	applicationClient := applicationpkg.NewApplicationServiceClient(connection)
	return &Client{
		connection: connection, applications: applicationClient, manager: applicationClient, runtime: applicationClient,
		permissions: accountpkg.NewAccountServiceClient(connection), token: cfg.Token,
		requestTimeout: cfg.RequestTimeout, controlPlaneNamespace: cfg.ApplicationNamespace,
	}, nil
}

func transportCredentials(cfg config.ArgoCDConfig) credentials.TransportCredentials {
	if cfg.Plaintext {
		return grpcinsecure.NewCredentials()
	}
	tlsConfig := &tls.Config{MinVersion: tls.VersionTLS12}
	if cfg.Insecure {
		// This option remains explicit for internal TLS endpoints whose certificate chain is not trusted by the container.
		tlsConfig.InsecureSkipVerify = true //nolint:gosec
	}
	return credentials.NewTLS(tlsConfig)
}

func newClientForTest(connection io.Closer, applications applicationLister, token string, requestTimeout time.Duration) (*Client, error) {
	if connection == nil || applications == nil || requestTimeout <= 0 {
		return nil, fmt.Errorf("invalid Argo CD client dependencies: connection, Application client, and request timeout are required")
	}
	return &Client{connection: connection, applications: applications, token: token, requestTimeout: requestTimeout}, nil
}

func newManagementClientForTest(connection io.Closer, manager applicationManager, permissions permissionChecker, token string, requestTimeout time.Duration, controlPlaneNamespace string) (*Client, error) {
	if connection == nil || manager == nil || permissions == nil || requestTimeout <= 0 || strings.TrimSpace(controlPlaneNamespace) == "" {
		return nil, fmt.Errorf("invalid Argo CD management client dependencies")
	}
	return &Client{
		connection: connection, manager: manager, permissions: permissions, token: token,
		requestTimeout: requestTimeout, controlPlaneNamespace: controlPlaneNamespace,
	}, nil
}

// ListApplications reads one complete observation without requesting refresh or mutation.
func (c *Client) ListApplications(ctx context.Context) ([]argodomain.Application, error) {
	requestCtx, cancel := c.requestContext(ctx)
	defer cancel()

	var response *argov1alpha1.ApplicationList
	err := retryRead(requestCtx, func() error {
		var err error
		response, err = c.applications.List(requestCtx, &applicationpkg.ApplicationQuery{})
		return err
	})
	if err != nil {
		return nil, fmt.Errorf("list Argo CD Applications: %w", err)
	}
	result := make([]argodomain.Application, 0, len(response.Items))
	for i := range response.Items {
		mapped, err := mapApplication(&response.Items[i])
		if err != nil {
			return nil, fmt.Errorf("map Argo CD Application at index %d: %w", i, err)
		}
		result = append(result, mapped)
	}
	return result, nil
}

// GetApplication reads one exact Application without requesting refresh.
func (c *Client) GetApplication(ctx context.Context, identity argodomain.ApplicationIdentity, project string) (argodomain.Application, error) {
	if c.manager == nil {
		return argodomain.Application{}, fmt.Errorf("argo CD management client is unavailable")
	}
	requestCtx, cancel := c.requestContext(ctx)
	defer cancel()
	query := &applicationpkg.ApplicationQuery{Name: &identity.Name, AppNamespace: &identity.Namespace}
	if strings.TrimSpace(project) != "" {
		query.Projects = []string{project}
	}
	var response *argov1alpha1.Application
	err := retryRead(requestCtx, func() error {
		var err error
		response, err = c.manager.Get(requestCtx, query)
		return err
	})
	if err != nil {
		return argodomain.Application{}, fmt.Errorf("get Argo CD Application: %w", err)
	}
	mapped, err := mapApplication(response)
	if err != nil {
		return argodomain.Application{}, fmt.Errorf("map Argo CD Application: %w", err)
	}
	return mapped, nil
}

// HardRefreshApplication requests cache invalidation and returns the newly reconciled read model.
func (c *Client) HardRefreshApplication(ctx context.Context, identity argodomain.ApplicationIdentity, project string) (argodomain.Application, error) {
	if c.manager == nil {
		return argodomain.Application{}, fmt.Errorf("argo CD management client is unavailable")
	}
	requestCtx, cancel := c.requestContext(ctx)
	defer cancel()
	refresh := "hard"
	query := applicationQuery(identity, project)
	query.Refresh = &refresh
	response, err := c.manager.Get(requestCtx, query)
	if err != nil {
		return argodomain.Application{}, fmt.Errorf("hard refresh Argo CD Application: %w", err)
	}
	return mapApplication(response)
}

// SyncApplication starts one operation against the exact reviewed revision snapshot.
func (c *Client) SyncApplication(ctx context.Context, value argodomain.SyncRequest) (argodomain.Application, error) {
	if c.manager == nil {
		return argodomain.Application{}, fmt.Errorf("argo CD management client is unavailable")
	}
	requestCtx, cancel := c.requestContext(ctx)
	defer cancel()
	request, err := syncRequest(value)
	if err != nil {
		return argodomain.Application{}, err
	}
	response, err := c.manager.Sync(requestCtx, request)
	if err != nil {
		return argodomain.Application{}, fmt.Errorf("sync Argo CD Application: %w", err)
	}
	return mapApplication(response)
}

// WatchApplication observes one exact Application after a known resource version.
func (c *Client) WatchApplication(ctx context.Context, identity argodomain.ApplicationIdentity, project, resourceVersion string) (argodomain.ApplicationWatch, error) {
	if c.manager == nil {
		return nil, fmt.Errorf("argo CD management client is unavailable")
	}
	requestCtx, cancel := c.requestContext(ctx)
	query := applicationQuery(identity, project)
	query.ResourceVersion = &resourceVersion
	stream, err := c.manager.Watch(requestCtx, query)
	if err != nil {
		cancel()
		return nil, fmt.Errorf("watch Argo CD Application: %w", err)
	}
	return &ApplicationWatch{stream: stream, cancel: cancel}, nil
}

// Recv returns the next immutable Application observation.
func (w *ApplicationWatch) Recv() (argodomain.Application, error) {
	event, err := w.stream.Recv()
	if err != nil {
		return argodomain.Application{}, err
	}
	return mapApplication(&event.Application)
}

// Close cancels the watch stream.
func (w *ApplicationWatch) Close() { w.cancel() }

// GetTargetManifests reads Argo CD rendered target manifests without changing Git, sync state, or Application configuration.
func (c *Client) GetTargetManifests(ctx context.Context, identity argodomain.ApplicationIdentity, project string) ([]string, string, error) {
	return c.getTargetManifests(ctx, identity, project, "")
}

// GetTargetManifestsAtRevision reads manifests pinned to one observed Application revision.
func (c *Client) GetTargetManifestsAtRevision(ctx context.Context, identity argodomain.ApplicationIdentity, project, revision string) ([]string, string, error) {
	revision = strings.TrimSpace(revision)
	if revision == "" {
		return nil, "", errors.New("target manifest revision is required")
	}
	manifests, resolvedRevision, err := c.getTargetManifests(ctx, identity, project, revision)
	if err != nil {
		return nil, "", err
	}
	if resolvedRevision != "" && resolvedRevision != revision {
		return nil, "", fmt.Errorf("target manifest revision mismatch: expected %q, got %q", revision, resolvedRevision)
	}
	return manifests, revision, nil
}

func (c *Client) getTargetManifests(ctx context.Context, identity argodomain.ApplicationIdentity, project, revision string) ([]string, string, error) {
	if c.manager == nil {
		return nil, "", fmt.Errorf("argo CD management client is unavailable")
	}
	requestCtx, cancel := c.requestContext(ctx)
	defer cancel()
	query := targetManifestQuery(identity, project, revision)
	var response *repositorypkg.ManifestResponse
	err := retryRead(requestCtx, func() error {
		var err error
		response, err = c.manager.GetManifests(requestCtx, query)
		return err
	})
	if err != nil {
		return nil, "", fmt.Errorf("get Argo CD target manifests: %w", err)
	}
	return slices.Clone(response.GetManifests()), response.GetRevision(), nil
}

func targetManifestQuery(identity argodomain.ApplicationIdentity, project, revision string) *applicationpkg.ApplicationManifestQuery {
	noCache := true
	query := &applicationpkg.ApplicationManifestQuery{
		Name: &identity.Name, AppNamespace: &identity.Namespace, Project: &project, NoCache: &noCache,
	}
	if revision != "" {
		query.Revision = &revision
	}
	return query
}

// GetManagedResourceDiffs reads normalized live and predicted target state without mutating the Application.
func (c *Client) GetManagedResourceDiffs(ctx context.Context, identity argodomain.ApplicationIdentity, project string) ([]argodomain.ResourceDiff, error) {
	if c.manager == nil {
		return nil, fmt.Errorf("argo CD management client is unavailable")
	}
	requestCtx, cancel := c.requestContext(ctx)
	defer cancel()
	query := &applicationpkg.ResourcesQuery{
		ApplicationName: &identity.Name, AppNamespace: &identity.Namespace, Project: &project,
	}
	var response *applicationpkg.ManagedResourcesResponse
	err := retryRead(requestCtx, func() error {
		var err error
		response, err = c.manager.ManagedResources(requestCtx, query)
		return err
	})
	if err != nil {
		return nil, fmt.Errorf("get Argo CD managed resources: %w", err)
	}
	result := make([]argodomain.ResourceDiff, 0, len(response.GetItems()))
	for _, item := range response.GetItems() {
		if item == nil {
			continue
		}
		result = append(result, argodomain.ResourceDiff{
			Group: item.Group, Kind: item.Kind, Namespace: item.Namespace, Name: item.Name,
			NormalizedLiveState: item.NormalizedLiveState, PredictedLiveState: item.PredictedLiveState, Modified: item.Modified,
		})
	}
	return result, nil
}

// CanUpdateApplication checks the service account against the exact Argo CD RBAC object.
func (c *Client) CanUpdateApplication(ctx context.Context, identity argodomain.ApplicationIdentity, project string) (bool, error) {
	if c.permissions == nil {
		return false, fmt.Errorf("argo CD permission client is unavailable")
	}
	requestCtx, cancel := c.requestContext(ctx)
	defer cancel()
	response, err := c.permissions.CanI(requestCtx, &accountpkg.CanIRequest{
		Resource: "applications", Action: "update", Subresource: c.rbacApplicationName(identity, project),
	})
	if err != nil {
		return false, fmt.Errorf("check Argo CD Application update permission: %w", err)
	}
	return response.GetValue() == "yes", nil
}

// DisableAutomatedSync applies only the merge patch needed to remove automated sync.
func (c *Client) DisableAutomatedSync(ctx context.Context, identity argodomain.ApplicationIdentity, project string) error {
	if c.manager == nil {
		return fmt.Errorf("argo CD management client is unavailable")
	}
	requestCtx, cancel := c.requestContext(ctx)
	defer cancel()
	patch, patchType := `{"spec":{"syncPolicy":{"automated":null}}}`, "merge"
	_, err := c.manager.Patch(requestCtx, &applicationpkg.ApplicationPatchRequest{
		Name: &identity.Name, AppNamespace: &identity.Namespace, Project: &project, Patch: &patch, PatchType: &patchType,
	})
	if err != nil {
		return fmt.Errorf("disable Argo CD automated sync: %w", err)
	}
	return nil
}

// Close releases the reusable Argo CD gRPC connection.
func (c *Client) Close() error {
	if c == nil || c.connection == nil {
		return nil
	}
	if err := c.connection.Close(); err != nil {
		return fmt.Errorf("close Argo CD gRPC connection: %w", err)
	}
	return nil
}

func (c *Client) requestContext(ctx context.Context) (context.Context, context.CancelFunc) {
	requestCtx, cancel := context.WithTimeout(ctx, c.requestTimeout)
	return metadata.AppendToOutgoingContext(requestCtx, "token", c.token), cancel
}

func applicationQuery(identity argodomain.ApplicationIdentity, project string) *applicationpkg.ApplicationQuery {
	query := &applicationpkg.ApplicationQuery{Name: &identity.Name, AppNamespace: &identity.Namespace}
	if strings.TrimSpace(project) != "" {
		query.Projects = []string{project}
	}
	return query
}

func syncRequest(value argodomain.SyncRequest) (*applicationpkg.ApplicationSyncRequest, error) {
	if strings.TrimSpace(value.OperationID) == "" || strings.TrimSpace(value.Identity.Name) == "" ||
		strings.TrimSpace(value.Identity.Namespace) == "" || strings.TrimSpace(value.Project) == "" {
		return nil, errors.New("invalid Argo CD Sync request")
	}
	info := &argov1alpha1.Info{Name: "releasehub.io/operation-id", Value: value.OperationID}
	request := &applicationpkg.ApplicationSyncRequest{
		Name: &value.Identity.Name, AppNamespace: &value.Identity.Namespace, Project: &value.Project,
		Infos: []*argov1alpha1.Info{info},
	}
	if len(value.Revisions) > 0 {
		request.Revisions = slices.Clone(value.Revisions)
	} else if strings.TrimSpace(value.Revision) != "" {
		request.Revision = &value.Revision
	} else {
		return nil, errors.New("Argo CD Sync request requires a pinned revision")
	}
	return request, nil
}

func (c *Client) rbacApplicationName(identity argodomain.ApplicationIdentity, project string) string {
	if identity.Namespace == c.controlPlaneNamespace {
		return project + "/" + identity.Name
	}
	return project + "/" + identity.Namespace + "/" + identity.Name
}

func retryRead(ctx context.Context, call func() error) error {
	backoff := initialBackoff
	for attempt := 1; attempt <= readAttempts; attempt++ {
		err := call()
		if err == nil {
			return nil
		}
		if !retryable(err) || attempt == readAttempts {
			return err
		}
		timer := time.NewTimer(backoff)
		select {
		case <-ctx.Done():
			timer.Stop()
			return ctx.Err()
		case <-timer.C:
		}
		backoff *= 2
	}
	return nil
}

func retryable(err error) bool {
	switch status.Code(err) {
	case codes.Unavailable, codes.ResourceExhausted:
		return true
	default:
		return false
	}
}

func mapApplication(source *argov1alpha1.Application) (argodomain.Application, error) {
	sources := source.Spec.Sources
	if len(sources) == 0 && source.Spec.Source != nil {
		sources = argov1alpha1.ApplicationSources{*source.Spec.Source}
	}
	mappedSources := make([]argodomain.Source, 0, len(sources))
	for _, item := range sources {
		mappedSources = append(mappedSources, argodomain.Source{
			RepositoryURL: item.RepoURL, TargetRevision: item.TargetRevision, Path: item.Path,
			Chart: item.Chart, Reference: item.Ref, Name: item.Name,
		})
	}
	operationPhase := ""
	operationID := ""
	if source.Status.OperationState != nil {
		operationPhase = string(source.Status.OperationState.Phase)
		operationID = operationInfo(source.Status.OperationState, "releasehub.io/operation-id")
	}
	var reconciledAt *time.Time
	if source.Status.ReconciledAt != nil {
		value := source.Status.ReconciledAt.Time.UTC()
		reconciledAt = &value
	}
	automatedSync := source.Spec.SyncPolicy != nil && source.Spec.SyncPolicy.IsAutomatedSyncEnabled()
	return argodomain.NewApplication(argodomain.Application{
		Identity: argodomain.ApplicationIdentity{Namespace: source.Namespace, Name: source.Name},
		Project:  source.Spec.Project, Labels: source.Labels, Sources: mappedSources,
		Destination:   argodomain.Destination{Server: source.Spec.Destination.Server, Name: source.Spec.Destination.Name, Namespace: source.Spec.Destination.Namespace},
		AutomatedSync: automatedSync, SyncStatus: string(source.Status.Sync.Status),
		HealthStatus: string(source.Status.Health.Status), OperationPhase: operationPhase, OperationID: operationID,
		ResolvedRevision: source.Status.Sync.Revision, ResolvedRevisions: source.Status.Sync.Revisions,
		ResourceVersion: source.ResourceVersion, ReconciledAt: reconciledAt,
	})
}

func operationInfo(state *argov1alpha1.OperationState, name string) string {
	if state.Operation.Sync == nil {
		return ""
	}
	for _, info := range state.Operation.Info {
		if info != nil && info.Name == name {
			return info.Value
		}
	}
	return ""
}
