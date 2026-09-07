package domain

import (
	"slices"
	"testing"

	"github.com/google/uuid"
)

func TestOnboardingValidationPreservesRepositoryTopology(t *testing.T) {
	target := OnboardingTarget{
		ApplicationID: uuid.New(), Production: true, CandidatePresent: true,
		Argo:        ApplicationIdentity{Namespace: "argocd", Name: "payment"},
		ArgoProject: "payment", DestinationServer: "https://kubernetes.default.svc", DestinationNamespace: "payment",
		Source: CatalogSource{RepositoryURL: "https://git.example.com/platform/manifests.git", TargetRevision: "main", Path: "production/payment"},
	}
	observation, err := NewApplication(Application{
		Identity: ApplicationIdentity{Namespace: "argocd", Name: "payment"}, Project: "payment",
		Labels: map[string]string{ManagedLabelKey: ManagedLabelValue},
		Sources: []Source{
			{RepositoryURL: "https://git.example.com/values.git", TargetRevision: "stable", Reference: "values"},
			{RepositoryURL: "https://git.example.com/platform/manifests.git", TargetRevision: "main", Path: "production/payment"},
		},
		Destination: Destination{Server: "https://kubernetes.default.svc", Namespace: "payment"},
	})
	if err != nil {
		t.Fatalf("create observation: %v", err)
	}
	if issues := target.ValidateObservation(observation); len(issues) != 0 {
		t.Fatalf("valid multi-source mapping was rejected: %#v", issues)
	}
}

func TestOnboardingValidationFailsClosed(t *testing.T) {
	target := OnboardingTarget{
		ApplicationID: uuid.New(), Production: false, CandidatePresent: false,
		Argo:        ApplicationIdentity{Namespace: "argocd", Name: "expected"},
		ArgoProject: "expected", DestinationServer: "cluster-a", DestinationNamespace: "expected",
		Source: CatalogSource{RepositoryURL: "repo-a", TargetRevision: "main", Path: "production/app"},
	}
	observation, err := NewApplication(Application{
		Identity: ApplicationIdentity{Namespace: "argocd", Name: "app"}, Project: "actual",
		Labels: map[string]string{}, Sources: []Source{{RepositoryURL: "repo-b", TargetRevision: "main", Path: "other"}},
		Destination: Destination{Server: "cluster-b", Namespace: "actual"},
	})
	if err != nil {
		t.Fatalf("create observation: %v", err)
	}
	issues := target.ValidateObservation(observation)
	for _, expected := range []ValidationIssueCode{
		IssueProductionRequired, IssueCandidateUnavailable, IssueManagedLabelMissing,
		IssueApplicationIdentityMismatch, IssueProjectMismatch, IssueDestinationMismatch, IssueSourceMismatch,
	} {
		if !slices.Contains(issues, expected) {
			t.Fatalf("missing validation issue %q from %#v", expected, issues)
		}
	}
}
