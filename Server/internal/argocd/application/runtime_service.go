package application

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"time"

	"github.com/google/uuid"

	argodomain "github.com/vincent119/ReleaseHub/Server/internal/argocd/domain"
	catalogapp "github.com/vincent119/ReleaseHub/Server/internal/catalog/application"
	catalogdomain "github.com/vincent119/ReleaseHub/Server/internal/catalog/domain"
)

const (
	maxTopologyNodes   = 500
	maxTopologyEdges   = 1000
	maxRuntimeEvents   = 100
	secretResourceKind = "Secret"
)

var (
	// ErrRuntimeNotFound intentionally represents missing and unauthorized runtime resources.
	ErrRuntimeNotFound = errors.New("runtime resource not found")
	// ErrInvalidRuntimeQuery identifies invalid topology or resource input.
	ErrInvalidRuntimeQuery = errors.New("invalid runtime query")
)

// RuntimePrincipal identifies one authenticated runtime observer.
type RuntimePrincipal struct {
	UserID    uuid.UUID
	Disabled  bool
	RequestID string
}

// RuntimeReader is the read-only Argo CD runtime boundary.
type RuntimeReader interface {
	GetRuntimeTree(context.Context, argodomain.ApplicationIdentity, string) (argodomain.RuntimeTree, error)
	GetRuntimeResource(context.Context, argodomain.ApplicationIdentity, string, argodomain.RuntimeResourceRef) (string, error)
	ListRuntimeEvents(context.Context, argodomain.ApplicationIdentity, string, argodomain.RuntimeResourceRef) ([]argodomain.RuntimeEvent, error)
	GetRuntimePodLogs(context.Context, argodomain.ApplicationIdentity, string, argodomain.RuntimeLogQuery) ([]argodomain.RuntimeLogEntry, error)
}

type runtimeCatalog interface {
	FindApplication(context.Context, catalogapp.Principal, uuid.UUID) (catalogdomain.Application, error)
}

// RuntimeAuditRecord contains metadata only; runtime content must never be stored in Audit.
type RuntimeAuditRecord struct {
	ActorID     uuid.UUID
	RequestID   string
	Application catalogdomain.Application
	Resource    argodomain.RuntimeResourceRef
	Action      string
}

// RuntimeAuditor records sensitive diagnostic reads.
type RuntimeAuditor interface {
	RecordRuntimeRead(context.Context, RuntimeAuditRecord) error
}

// RuntimeServiceOptions supplies the runtime observation dependencies.
type RuntimeServiceOptions struct {
	Catalog runtimeCatalog
	Reader  RuntimeReader
	Auditor RuntimeAuditor
}

// RuntimeService authorizes Application visibility before every Argo CD runtime read.
type RuntimeService struct {
	catalog runtimeCatalog
	reader  RuntimeReader
	auditor RuntimeAuditor
}

// NewRuntimeService creates the read-only runtime observation service.
func NewRuntimeService(options RuntimeServiceOptions) (*RuntimeService, error) {
	if options.Catalog == nil || options.Reader == nil || options.Auditor == nil {
		return nil, errors.New("runtime catalog, reader, and auditor are required")
	}
	return &RuntimeService{catalog: options.Catalog, reader: options.Reader, auditor: options.Auditor}, nil
}

// RuntimeTopologyNode is one normalized node disclosed to HTTP delivery.
type RuntimeTopologyNode struct {
	Resource argodomain.RuntimeResource
}

// RuntimeTopologyEdge is one evidence-backed relationship.
type RuntimeTopologyEdge struct {
	ID     string
	Source string
	Target string
	Kind   string
}

// RuntimeTopology is one bounded resource or network projection.
type RuntimeTopology struct {
	ApplicationID uuid.UUID
	View          string
	ObservedAt    time.Time
	Nodes         []RuntimeTopologyNode
	Edges         []RuntimeTopologyEdge
	Warnings      []string
	Partial       bool
}

// ResourceDetail combines a safe summary with the redacted live manifest.
type ResourceDetail struct {
	Resource argodomain.RuntimeResource
	Manifest string
}

// Topology returns an authorized bounded topology projection.
func (s *RuntimeService) Topology(ctx context.Context, principal RuntimePrincipal, applicationID uuid.UUID, view string) (RuntimeTopology, error) {
	if view != "resources" && view != "network" {
		return RuntimeTopology{}, ErrInvalidRuntimeQuery
	}
	application, err := s.authorizedApplication(ctx, principal, applicationID)
	if err != nil {
		return RuntimeTopology{}, err
	}
	tree, err := s.reader.GetRuntimeTree(ctx, argoIdentity(application), application.ArgoProject)
	if err != nil {
		return RuntimeTopology{}, fmt.Errorf("read runtime topology: %w", err)
	}
	return buildRuntimeTopology(applicationID, view, tree), nil
}

// Resource returns an authorized summary and redacted live manifest.
func (s *RuntimeService) Resource(ctx context.Context, principal RuntimePrincipal, applicationID uuid.UUID, resource argodomain.RuntimeResourceRef) (ResourceDetail, error) {
	application, node, err := s.authorizedResource(ctx, principal, applicationID, resource)
	if err != nil {
		return ResourceDetail{}, err
	}
	manifest, err := s.reader.GetRuntimeResource(ctx, argoIdentity(application), application.ArgoProject, node.Ref)
	if err != nil {
		return ResourceDetail{}, fmt.Errorf("read runtime resource: %w", err)
	}
	manifest = redactRuntimeManifest(node.Ref, manifest)
	if err := s.audit(ctx, principal, application, node.Ref, "argocd.runtime.manifest.read"); err != nil {
		return ResourceDetail{}, err
	}
	return ResourceDetail{Resource: node, Manifest: manifest}, nil
}

// Events returns bounded events for one authorized resource.
func (s *RuntimeService) Events(ctx context.Context, principal RuntimePrincipal, applicationID uuid.UUID, resource argodomain.RuntimeResourceRef) ([]argodomain.RuntimeEvent, error) {
	application, node, err := s.authorizedResource(ctx, principal, applicationID, resource)
	if err != nil {
		return nil, err
	}
	values, err := s.reader.ListRuntimeEvents(ctx, argoIdentity(application), application.ArgoProject, node.Ref)
	if err != nil {
		return nil, fmt.Errorf("read runtime events: %w", err)
	}
	if len(values) > maxRuntimeEvents {
		values = values[:maxRuntimeEvents]
	}
	if err := s.audit(ctx, principal, application, node.Ref, "argocd.runtime.events.read"); err != nil {
		return nil, err
	}
	return values, nil
}

// PodLogs returns a bounded log snapshot for one authorized Pod.
func (s *RuntimeService) PodLogs(ctx context.Context, principal RuntimePrincipal, applicationID uuid.UUID, query argodomain.RuntimeLogQuery) ([]argodomain.RuntimeLogEntry, error) {
	application, node, err := s.authorizedResource(ctx, principal, applicationID, query.Resource)
	if err != nil {
		return nil, err
	}
	if node.Ref.Kind != "Pod" {
		return nil, ErrInvalidRuntimeQuery
	}
	query.Resource = node.Ref
	values, err := s.reader.GetRuntimePodLogs(ctx, argoIdentity(application), application.ArgoProject, query)
	if err != nil {
		return nil, fmt.Errorf("read runtime Pod logs: %w", err)
	}
	if err := s.audit(ctx, principal, application, node.Ref, "argocd.runtime.logs.read"); err != nil {
		return nil, err
	}
	return values, nil
}

func (s *RuntimeService) authorizedResource(ctx context.Context, principal RuntimePrincipal, applicationID uuid.UUID, requested argodomain.RuntimeResourceRef) (catalogdomain.Application, argodomain.RuntimeResource, error) {
	if err := requested.Validate(); err != nil {
		return catalogdomain.Application{}, argodomain.RuntimeResource{}, ErrInvalidRuntimeQuery
	}
	application, err := s.authorizedApplication(ctx, principal, applicationID)
	if err != nil {
		return catalogdomain.Application{}, argodomain.RuntimeResource{}, err
	}
	tree, err := s.reader.GetRuntimeTree(ctx, argoIdentity(application), application.ArgoProject)
	if err != nil {
		return catalogdomain.Application{}, argodomain.RuntimeResource{}, fmt.Errorf("verify runtime resource ownership: %w", err)
	}
	for _, node := range tree.Resources {
		if !node.Orphaned && node.Ref.Key() == requested.Key() {
			return application, node, nil
		}
	}
	return catalogdomain.Application{}, argodomain.RuntimeResource{}, ErrRuntimeNotFound
}

func (s *RuntimeService) authorizedApplication(ctx context.Context, principal RuntimePrincipal, applicationID uuid.UUID) (catalogdomain.Application, error) {
	if applicationID == uuid.Nil {
		return catalogdomain.Application{}, ErrRuntimeNotFound
	}
	value, err := s.catalog.FindApplication(ctx, catalogapp.Principal{UserID: principal.UserID, Disabled: principal.Disabled}, applicationID)
	if err != nil {
		return catalogdomain.Application{}, ErrRuntimeNotFound
	}
	return value, nil
}

func (s *RuntimeService) audit(ctx context.Context, principal RuntimePrincipal, application catalogdomain.Application, resource argodomain.RuntimeResourceRef, action string) error {
	err := s.auditor.RecordRuntimeRead(ctx, RuntimeAuditRecord{
		ActorID: principal.UserID, RequestID: principal.RequestID, Application: application,
		Resource: resource, Action: action,
	})
	if err != nil {
		return fmt.Errorf("record runtime read audit: %w", err)
	}
	return nil
}

func argoIdentity(application catalogdomain.Application) argodomain.ApplicationIdentity {
	return argodomain.ApplicationIdentity{Namespace: application.Argo.Namespace, Name: application.Argo.Name}
}

func buildRuntimeTopology(applicationID uuid.UUID, view string, tree argodomain.RuntimeTree) RuntimeTopology {
	resources, partial, warnings := boundedTopologyResources(tree.Resources)
	nodes, visible := topologyNodes(resources)
	projection := topologyEdges(view, resources, visible)
	edges, edgePartial, edgeWarnings := boundedTopologyEdges(projection.edges)
	partial = partial || edgePartial
	warnings = append(warnings, edgeWarnings...)
	partial, warnings = applyNetworkProjectionWarnings(view, projection, partial, warnings)
	return RuntimeTopology{
		ApplicationID: applicationID, View: view, ObservedAt: tree.ObservedAt.UTC(),
		Nodes: nodes, Edges: edges, Warnings: warnings, Partial: partial,
	}
}

func redactRuntimeManifest(resource argodomain.RuntimeResourceRef, manifest string) string {
	if resource.Kind != secretResourceKind {
		return manifest
	}
	var value map[string]any
	if json.Unmarshal([]byte(manifest), &value) == nil {
		delete(value, "data")
		delete(value, "stringData")
		redacted, err := json.MarshalIndent(value, "", "  ")
		if err == nil {
			return string(redacted)
		}
	}
	safe := map[string]any{
		"apiVersion": resource.Version, "kind": resource.Kind,
		"metadata": map[string]string{"name": resource.Name, "namespace": resource.Namespace},
	}
	redacted, _ := json.MarshalIndent(safe, "", "  ")
	return string(redacted)
}
