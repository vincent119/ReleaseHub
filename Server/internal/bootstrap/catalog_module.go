package bootstrap

import (
	"fmt"

	"github.com/google/uuid"
	argoinfra "github.com/vincent119/ReleaseHub/Server/internal/argocd/infrastructure"
	authzinfra "github.com/vincent119/ReleaseHub/Server/internal/authorization/infrastructure"
	catalogapp "github.com/vincent119/ReleaseHub/Server/internal/catalog/application"
	cataloginfra "github.com/vincent119/ReleaseHub/Server/internal/catalog/infrastructure"
	"github.com/vincent119/ReleaseHub/Server/internal/transport/httpserver"
	"gorm.io/gorm"
)

func newCatalogHandlerOptions(db *gorm.DB, policy *authzinfra.PolicyEngine, defaultOrganizationValue string) (httpserver.CatalogHandlerOptions, error) {
	repository, err := cataloginfra.NewCatalogRepository(db)
	if err != nil {
		return httpserver.CatalogHandlerOptions{}, err
	}
	defaultOrganizationID, err := uuid.Parse(defaultOrganizationValue)
	if err != nil {
		return httpserver.CatalogHandlerOptions{}, fmt.Errorf("parse default Organization ID: %w", err)
	}
	service, err := catalogapp.NewService(repository, policy, defaultOrganizationID)
	if err != nil {
		return httpserver.CatalogHandlerOptions{}, err
	}
	status, err := argoinfra.NewStatusRepository(db)
	return httpserver.CatalogHandlerOptions{Service: service, StatusReader: status}, err
}
