package infrastructure

import (
	"context"
	"strings"
	"testing"

	"github.com/google/uuid"

	argodomain "github.com/vincent119/ReleaseHub/Server/internal/argocd/domain"
	deployapp "github.com/vincent119/ReleaseHub/Server/internal/deployment/application"
)

func TestArgoActualStateReaderRequiresDigestEvidence(t *testing.T) {
	identity := argodomain.ApplicationIdentity{Namespace: "argocd", Name: "api-production"}
	argo := &actualStateArgoStub{application: argodomain.Application{Identity: identity, ResolvedRevision: "commit-a"},
		images: []string{"registry.example/api:v1.0.0"}}
	reader, _ := NewArgoActualStateReader(argo)
	applicationID := uuid.New()
	_, err := reader.ReadActualState(context.Background(), deployapp.DeploymentExecutionNode{ApplicationID: applicationID, Identity: identity})
	if err == nil || !strings.Contains(err.Error(), "digest is unavailable") {
		t.Fatalf("tag-only actual state must fail closed: %v", err)
	}

	argo.images = []string{"registry.example/api:v1.0.0@sha256:" + strings.Repeat("a", 64)}
	state, err := reader.ReadActualState(context.Background(), deployapp.DeploymentExecutionNode{ApplicationID: applicationID, Identity: identity})
	if err != nil || state.Revision != "commit-a" || state.Images[0].Tag != "v1.0.0" {
		t.Fatalf("digest actual state = %#v, %v", state, err)
	}
}

type actualStateArgoStub struct {
	application argodomain.Application
	images      []string
}

func (s *actualStateArgoStub) GetApplication(context.Context, argodomain.ApplicationIdentity, string) (argodomain.Application, error) {
	return s.application, nil
}

func (s *actualStateArgoStub) GetApplicationImages(context.Context, argodomain.ApplicationIdentity, string) ([]string, error) {
	return s.images, nil
}
