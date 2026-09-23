package application

import (
	"context"
	"errors"
	"strings"
	"testing"
	"time"

	"github.com/google/uuid"

	argodomain "github.com/vincent119/ReleaseHub/Server/internal/argocd/domain"
	catalogapp "github.com/vincent119/ReleaseHub/Server/internal/catalog/application"
	catalogdomain "github.com/vincent119/ReleaseHub/Server/internal/catalog/domain"
)

func TestRuntimeTopologyAuthorizesBeforeReadingArgoCD(t *testing.T) {
	reader := &runtimeReaderStub{}
	service := newRuntimeServiceForTest(t, &runtimeCatalogStub{err: errors.New("denied")}, reader, &runtimeAuditorStub{})

	_, err := service.Topology(context.Background(), RuntimePrincipal{UserID: uuid.New()}, uuid.New(), "resources")

	if !errors.Is(err, ErrRuntimeNotFound) || reader.treeCalls != 0 {
		t.Fatalf("topology error = %v, reader calls = %d", err, reader.treeCalls)
	}
}

func TestRuntimeTopologyUsesOnlyReportedRelationships(t *testing.T) {
	application := runtimeApplication()
	parent := runtimeRef("apps", "v1", "Deployment", "payments", "api")
	child := runtimeRef("apps", "v1", "ReplicaSet", "payments", "api-7dd")
	reader := &runtimeReaderStub{tree: argodomain.RuntimeTree{
		ObservedAt: time.Date(2026, 9, 23, 8, 0, 0, 0, time.UTC),
		Resources: []argodomain.RuntimeResource{
			{Ref: parent},
			{Ref: child, ParentRefs: []argodomain.RuntimeResourceRef{parent}},
		},
	}}
	service := newRuntimeServiceForTest(t, &runtimeCatalogStub{application: application}, reader, &runtimeAuditorStub{})

	value, err := service.Topology(context.Background(), RuntimePrincipal{UserID: uuid.New()}, application.ID, "resources")

	if err != nil || len(value.Edges) != 1 || value.Edges[0].Source != parent.Key() || value.Edges[0].Target != child.Key() {
		t.Fatalf("topology = %#v, error = %v", value, err)
	}
}

func TestRuntimeSecretManifestIsRedactedAndAudited(t *testing.T) {
	application := runtimeApplication()
	secret := runtimeRef("", "v1", "Secret", "payments", "credentials")
	reader := &runtimeReaderStub{
		tree:     argodomain.RuntimeTree{Resources: []argodomain.RuntimeResource{{Ref: secret}}},
		manifest: `{"apiVersion":"v1","kind":"Secret","metadata":{"name":"credentials"},"data":{"token":"c2VjcmV0"}}`,
	}
	auditor := &runtimeAuditorStub{}
	service := newRuntimeServiceForTest(t, &runtimeCatalogStub{application: application}, reader, auditor)

	value, err := service.Resource(context.Background(), RuntimePrincipal{UserID: uuid.New(), RequestID: "request-1"}, application.ID, secret)

	if err != nil || strings.Contains(value.Manifest, "c2VjcmV0") || strings.Contains(value.Manifest, `"data"`) {
		t.Fatalf("manifest = %s, error = %v", value.Manifest, err)
	}
	if auditor.calls != 1 || auditor.record.Action != "argocd.runtime.manifest.read" || auditor.record.RequestID != "request-1" {
		t.Fatalf("audit = %#v, calls = %d", auditor.record, auditor.calls)
	}
}

func TestRuntimeLogsRejectNonPodBeforeVendorLogRead(t *testing.T) {
	application := runtimeApplication()
	deployment := runtimeRef("apps", "v1", "Deployment", "payments", "api")
	reader := &runtimeReaderStub{tree: argodomain.RuntimeTree{Resources: []argodomain.RuntimeResource{{Ref: deployment}}}}
	service := newRuntimeServiceForTest(t, &runtimeCatalogStub{application: application}, reader, &runtimeAuditorStub{})

	_, err := service.PodLogs(context.Background(), RuntimePrincipal{UserID: uuid.New()}, application.ID, argodomain.RuntimeLogQuery{Resource: deployment})

	if !errors.Is(err, ErrInvalidRuntimeQuery) || reader.logCalls != 0 {
		t.Fatalf("logs error = %v, reader calls = %d", err, reader.logCalls)
	}
}

func newRuntimeServiceForTest(t *testing.T, catalog runtimeCatalog, reader RuntimeReader, auditor RuntimeAuditor) *RuntimeService {
	t.Helper()
	service, err := NewRuntimeService(RuntimeServiceOptions{Catalog: catalog, Reader: reader, Auditor: auditor})
	if err != nil {
		t.Fatalf("create runtime service: %v", err)
	}
	return service
}

func runtimeApplication() catalogdomain.Application {
	return catalogdomain.Application{
		ID: uuid.New(), OrganizationID: uuid.New(), ProjectID: uuid.New(), EnvironmentID: uuid.New(),
		Argo: catalogdomain.ArgoApplicationIdentity{Namespace: "argocd", Name: "payments"}, ArgoProject: "payments",
	}
}

func runtimeRef(group, version, kind, namespace, name string) argodomain.RuntimeResourceRef {
	return argodomain.RuntimeResourceRef{Group: group, Version: version, Kind: kind, Namespace: namespace, Name: name}
}

type runtimeCatalogStub struct {
	application catalogdomain.Application
	err         error
}

func (s *runtimeCatalogStub) FindApplication(context.Context, catalogapp.Principal, uuid.UUID) (catalogdomain.Application, error) {
	return s.application, s.err
}

type runtimeReaderStub struct {
	tree      argodomain.RuntimeTree
	manifest  string
	treeCalls int
	logCalls  int
}

func (s *runtimeReaderStub) GetRuntimeTree(context.Context, argodomain.ApplicationIdentity, string) (argodomain.RuntimeTree, error) {
	s.treeCalls++
	return s.tree, nil
}

func (s *runtimeReaderStub) GetRuntimeResource(context.Context, argodomain.ApplicationIdentity, string, argodomain.RuntimeResourceRef) (string, error) {
	return s.manifest, nil
}

func (s *runtimeReaderStub) ListRuntimeEvents(context.Context, argodomain.ApplicationIdentity, string, argodomain.RuntimeResourceRef) ([]argodomain.RuntimeEvent, error) {
	return nil, nil
}

func (s *runtimeReaderStub) GetRuntimePodLogs(context.Context, argodomain.ApplicationIdentity, string, argodomain.RuntimeLogQuery) ([]argodomain.RuntimeLogEntry, error) {
	s.logCalls++
	return nil, nil
}

type runtimeAuditorStub struct {
	calls  int
	record RuntimeAuditRecord
}

func (s *runtimeAuditorStub) RecordRuntimeRead(_ context.Context, record RuntimeAuditRecord) error {
	s.calls++
	s.record = record
	return nil
}
