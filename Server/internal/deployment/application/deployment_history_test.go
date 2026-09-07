package application

import (
	"context"
	"testing"

	"github.com/google/uuid"

	authz "github.com/vincent119/ReleaseHub/Server/internal/authorization/domain"
)

func TestDeploymentHistoryRequiresCurrentPermission(t *testing.T) {
	repository := &deploymentHistoryRepositoryStub{}
	service, err := NewDeploymentHistoryService(repository, executionAuthorizerStub{allowed: false})
	if err != nil {
		t.Fatalf("create deployment history service: %v", err)
	}
	_, err = service.List(context.Background(), RequestPrincipal{UserID: uuid.New()}, DeploymentHistoryQuery{
		ProjectID: uuid.New(), EnvironmentID: uuid.New(), Limit: 20,
	})
	if err != ErrHistoryForbidden || repository.listed {
		t.Fatalf("history permission = %v, listed %v", err, repository.listed)
	}
}

type deploymentHistoryRepositoryStub struct{ listed bool }

func (s *deploymentHistoryRepositoryStub) ResolveScope(context.Context, uuid.UUID, uuid.UUID) (authz.Scope, error) {
	return authz.NewEnvironmentScope(uuid.New(), uuid.New(), uuid.New())
}

func (s *deploymentHistoryRepositoryStub) List(context.Context, authz.Scope, string, int) (DeploymentHistoryPage, error) {
	s.listed = true
	return DeploymentHistoryPage{}, nil
}
