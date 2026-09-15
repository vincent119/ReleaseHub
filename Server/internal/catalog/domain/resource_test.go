package domain_test

import (
	"errors"
	"testing"

	"github.com/google/uuid"
	"github.com/vincent119/ReleaseHub/Server/internal/catalog/domain"
)

func TestOrganizationRenamePreservesIdentityAndDefaultProtection(t *testing.T) {
	defaultID := uuid.New()
	organization := domain.Organization{ID: defaultID, Name: "default", Active: true, Version: 4}

	renamed, err := organization.Rename("  Platform Engineering  ")
	if err != nil {
		t.Fatalf("rename Organization: %v", err)
	}
	if renamed.ID != defaultID || renamed.Name != "Platform Engineering" || renamed.Version != 5 || !renamed.Active {
		t.Fatalf("renamed Organization changed protected state: %#v", renamed)
	}
	if err := renamed.ValidateDeletion(defaultID); !errors.Is(err, domain.ErrDefaultOrganizationProtected) {
		t.Fatalf("renamed default Organization should remain protected: %v", err)
	}
	if err := renamed.ValidateDeletion(uuid.New()); err != nil {
		t.Fatalf("non-default Organization should pass the default guard: %v", err)
	}
}

func TestOrganizationRenameRejectsInvalidState(t *testing.T) {
	tests := []struct {
		name         string
		organization domain.Organization
		newName      string
	}{
		{name: "missing identity", organization: domain.Organization{Active: true, Version: 1}, newName: "Workspace"},
		{name: "inactive", organization: domain.Organization{ID: uuid.New(), Version: 1}, newName: "Workspace"},
		{name: "missing version", organization: domain.Organization{ID: uuid.New(), Active: true}, newName: "Workspace"},
		{name: "blank name", organization: domain.Organization{ID: uuid.New(), Active: true, Version: 1}, newName: "  "},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			if _, err := test.organization.Rename(test.newName); err == nil {
				t.Fatal("invalid Organization rename should fail")
			}
		})
	}
}

func TestOrganizationDeactivateProtectsDefaultAndAdvancesVersion(t *testing.T) {
	organizationID := uuid.New()
	organization := domain.Organization{ID: organizationID, Name: "Tenant", Active: true, Version: 2}

	deactivated, err := organization.Deactivate(uuid.New())
	if err != nil {
		t.Fatalf("deactivate non-default Organization: %v", err)
	}
	if deactivated.Active || deactivated.Version != 3 || deactivated.ID != organizationID {
		t.Fatalf("deactivated Organization = %#v", deactivated)
	}
	if _, err := organization.Deactivate(organizationID); !errors.Is(err, domain.ErrDefaultOrganizationProtected) {
		t.Fatalf("deactivate default Organization = %v", err)
	}
}

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
