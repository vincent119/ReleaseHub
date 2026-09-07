package domain_test

import (
	"testing"

	"github.com/google/uuid"
	"github.com/vincent119/ReleaseHub/Server/internal/catalog/domain"
)

func TestCustomEnvironmentFixedType(t *testing.T) {
	environment, err := domain.NewEnvironment(uuid.New(), uuid.New(), "uat-tw", domain.EnvironmentTesting)
	if err != nil {
		t.Fatalf("create custom Environment: %v", err)
	}
	if environment.Name != "uat-tw" || environment.Type != domain.EnvironmentTesting {
		t.Fatalf("unexpected Environment: %#v", environment)
	}

	if _, err := domain.NewEnvironment(uuid.New(), uuid.New(), "uat-tw", "UAT"); err == nil {
		t.Fatal("unknown Environment type should be rejected")
	}
}

func TestApplicationSourceSupportsRepositoryTopologies(t *testing.T) {
	organizationID, projectID, environmentID := uuid.New(), uuid.New(), uuid.New()
	for _, source := range []domain.GitOpsSource{
		{RepositoryURL: "https://git.example.com/team/application.git", TargetRevision: "main", Path: "deploy/production"},
		{RepositoryURL: "https://git.example.com/platform/manifests.git", TargetRevision: "main", Path: "production/admin"},
	} {
		application, err := domain.NewApplication(
			organizationID, projectID, environmentID, "admin",
			domain.ArgoApplicationIdentity{Namespace: "argocd", Name: "admin-production"},
			"admin", "https://kubernetes.default.svc", "admin", source,
		)
		if err != nil {
			t.Fatalf("create Application mapping: %v", err)
		}
		if application.Source.RepositoryURL != source.RepositoryURL || application.Source.Path != source.Path {
			t.Fatalf("source mapping changed: %#v", application.Source)
		}
	}
}

func TestApplicationRejectsUnsafeGitOpsPath(t *testing.T) {
	_, err := domain.NewApplication(
		uuid.New(), uuid.New(), uuid.New(), "admin",
		domain.ArgoApplicationIdentity{Namespace: "argocd", Name: "admin-production"},
		"admin", "https://kubernetes.default.svc", "admin",
		domain.GitOpsSource{RepositoryURL: "https://git.example.com/manifests.git", TargetRevision: "main", Path: "../other-app"},
	)
	if err == nil {
		t.Fatal("parent traversal should be rejected")
	}
}
