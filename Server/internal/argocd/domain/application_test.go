package domain_test

import (
	"testing"
	"time"

	"github.com/vincent119/ReleaseHub/Server/internal/argocd/domain"
)

func TestManagedLabelDiscoveryRequiresExactOptIn(t *testing.T) {
	for _, test := range []struct {
		name     string
		labels   map[string]string
		expected bool
	}{
		{name: "exact", labels: map[string]string{"releasehub.io/managed": "true"}, expected: true},
		{name: "wrong value case", labels: map[string]string{"releasehub.io/managed": "True"}},
		{name: "unrelated label", labels: map[string]string{"app.kubernetes.io/part-of": "app-deployments"}},
		{name: "missing", labels: map[string]string{}},
	} {
		t.Run(test.name, func(t *testing.T) {
			value, err := domain.NewApplication(domain.Application{Identity: domain.ApplicationIdentity{Namespace: "argocd", Name: "admin"}, Labels: test.labels})
			if err != nil {
				t.Fatalf("create observation: %v", err)
			}
			if value.IsCandidate() != test.expected {
				t.Fatalf("candidate mismatch: got %v", value.IsCandidate())
			}
		})
	}
}

func TestApplicationObservationCopiesMutableInput(t *testing.T) {
	reconciledAt := time.Now()
	labels := map[string]string{"releasehub.io/managed": "true"}
	sources := []domain.Source{{RepositoryURL: "https://git.example.com/manifests.git"}}
	revisions := []string{"commit-a"}
	value, err := domain.NewApplication(domain.Application{
		Identity: domain.ApplicationIdentity{Namespace: " argocd ", Name: " admin "}, Labels: labels,
		Sources: sources, ResolvedRevisions: revisions, ReconciledAt: &reconciledAt,
	})
	if err != nil {
		t.Fatalf("create observation: %v", err)
	}
	labels[domain.ManagedLabelKey] = "false"
	sources[0].RepositoryURL = "changed"
	revisions[0] = "changed"
	if !value.IsCandidate() || value.Sources[0].RepositoryURL == "changed" || value.ResolvedRevisions[0] == "changed" {
		t.Fatal("observation should own copies of mutable input")
	}
	if value.Identity.Namespace != "argocd" || value.Identity.Name != "admin" || value.ReconciledAt.Location() != time.UTC {
		t.Fatalf("observation was not normalized: %#v", value)
	}
}
