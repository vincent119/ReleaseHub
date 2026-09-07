package application

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/google/uuid"

	argodomain "github.com/vincent119/ReleaseHub/Server/internal/argocd/domain"
	authz "github.com/vincent119/ReleaseHub/Server/internal/authorization/domain"
	ecrdomain "github.com/vincent119/ReleaseHub/Server/internal/ecr/domain"
)

func TestDigestResolverLocksManifestTagsAndRecordsFailures(t *testing.T) {
	t.Parallel()
	repository := &digestRepositoryStub{target: validDigestTarget()}
	resolver := mustDigestResolver(t, repository, manifestReaderStub{manifests: []string{podManifest("123456789012.dkr.ecr.ap-northeast-1.amazonaws.com/platform/api:v1", "123456789012.dkr.ecr.ap-northeast-1.amazonaws.com/platform/worker:v1")}}, registryStub{digests: map[string]string{"platform/api:v1": "sha256:aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa"}})
	principal := DigestPrincipal{UserID: uuid.New()}
	snapshots, err := resolver.Resolve(context.Background(), principal, DigestMutation{ActorID: uuid.New(), RequestID: "request-1"}, repository.target.ApplicationID)
	if !errors.Is(err, ErrDigestUnavailable) || len(snapshots) != 2 || snapshots[0].Status != ecrdomain.DigestAvailable || snapshots[1].ErrorCode != "repository_out_of_scope" {
		t.Fatalf("Resolve() = %#v, %v", snapshots, err)
	}
	if repository.mutation.ActorID != principal.UserID || len(repository.snapshots) != 2 {
		t.Fatalf("persisted resolution mismatch: %#v %#v", repository.mutation, repository.snapshots)
	}
}

func TestDigestResolverFailsClosedBeforePersistingUnavailableManifest(t *testing.T) {
	t.Parallel()
	repository := &digestRepositoryStub{target: validDigestTarget()}
	resolver := mustDigestResolver(t, repository, manifestReaderStub{err: errors.New("Argo CD unavailable")}, registryStub{})
	_, err := resolver.Resolve(context.Background(), DigestPrincipal{UserID: uuid.New()}, DigestMutation{}, repository.target.ApplicationID)
	if !errors.Is(err, ErrTargetManifestUnavailable) || len(repository.snapshots) != 0 {
		t.Fatalf("Resolve() error = %v snapshots=%#v", err, repository.snapshots)
	}
}

func TestDigestResolverHidesUnauthorizedTarget(t *testing.T) {
	t.Parallel()
	repository := &digestRepositoryStub{target: validDigestTarget()}
	resolver, err := NewDigestResolver(repository, manifestReaderStub{}, registryStub{}, denyDigestAuthorizer{}, mustDigestScope(t))
	if err != nil {
		t.Fatalf("NewDigestResolver() error = %v", err)
	}
	_, err = resolver.Resolve(context.Background(), DigestPrincipal{UserID: uuid.New()}, DigestMutation{}, repository.target.ApplicationID)
	if !errors.Is(err, ErrDigestTargetNotFound) {
		t.Fatalf("unauthorized error = %v", err)
	}
}

func mustDigestResolver(t *testing.T, repository DigestRepository, manifests TargetManifestReader, registry DigestRegistry) *DigestResolver {
	t.Helper()
	resolver, err := NewDigestResolver(repository, manifests, registry, allowDigestAuthorizer{}, mustDigestScope(t))
	if err != nil {
		t.Fatalf("NewDigestResolver() error = %v", err)
	}
	resolver.locker.now = func() time.Time { return time.Date(2026, 9, 2, 9, 0, 0, 0, time.UTC) }
	return resolver
}

func mustDigestScope(t *testing.T) ecrdomain.RegistryScope {
	t.Helper()
	scope, err := ecrdomain.NewRegistryScope("123456789012", "ap-northeast-1", []string{"platform/api"})
	if err != nil {
		t.Fatalf("NewRegistryScope() error = %v", err)
	}
	return scope
}

func validDigestTarget() DigestTarget {
	return DigestTarget{ApplicationID: uuid.New(), OrganizationID: uuid.New(), ProjectID: uuid.New(), EnvironmentID: uuid.New(), Argo: argodomain.ApplicationIdentity{Namespace: "argocd", Name: "payment-production"}, ArgoProject: "payment"}
}

func podManifest(images ...string) string {
	result := "apiVersion: v1\nkind: Pod\nspec:\n  containers:\n"
	for index, image := range images {
		result += "    - name: container-" + string(rune('a'+index)) + "\n      image: " + image + "\n"
	}
	return result
}

type digestRepositoryStub struct {
	target    DigestTarget
	mutation  DigestMutation
	snapshots []ecrdomain.DigestSnapshot
}

func (s *digestRepositoryStub) LoadDigestTarget(_ context.Context, applicationID uuid.UUID) (DigestTarget, error) {
	if applicationID != s.target.ApplicationID {
		return DigestTarget{}, errors.New("not found")
	}
	return s.target, nil
}

func (s *digestRepositoryStub) AppendDigestSnapshots(_ context.Context, mutation DigestMutation, _ DigestTarget, snapshots []ecrdomain.DigestSnapshot) error {
	s.mutation = mutation
	s.snapshots = snapshots
	return nil
}

type manifestReaderStub struct {
	manifests []string
	revision  string
	err       error
}

func (s manifestReaderStub) GetTargetManifests(context.Context, argodomain.ApplicationIdentity, string) ([]string, string, error) {
	if s.err != nil {
		return nil, "", s.err
	}
	return s.manifests, "commit-a", nil
}

type registryStub struct{ digests map[string]string }

func (s registryStub) ResolveDigest(_ context.Context, reference ecrdomain.ImageReference) (string, error) {
	if digest, exists := s.digests[reference.Repository+":"+reference.Tag]; exists {
		return digest, nil
	}
	return "", ecrdomain.ErrImageNotFound
}

type allowDigestAuthorizer struct{}

func (allowDigestAuthorizer) AuthorizeFresh(context.Context, authz.AuthorizationRequest) (bool, error) {
	return true, nil
}

type denyDigestAuthorizer struct{}

func (denyDigestAuthorizer) AuthorizeFresh(context.Context, authz.AuthorizationRequest) (bool, error) {
	return false, nil
}
