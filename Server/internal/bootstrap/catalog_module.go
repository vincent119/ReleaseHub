package bootstrap

import (
	"fmt"

	"github.com/google/uuid"
	argoapp "github.com/vincent119/ReleaseHub/Server/internal/argocd/application"
	argoinfra "github.com/vincent119/ReleaseHub/Server/internal/argocd/infrastructure"
	authzinfra "github.com/vincent119/ReleaseHub/Server/internal/authorization/infrastructure"
	catalogapp "github.com/vincent119/ReleaseHub/Server/internal/catalog/application"
	cataloginfra "github.com/vincent119/ReleaseHub/Server/internal/catalog/infrastructure"
	"github.com/vincent119/ReleaseHub/Server/internal/transport/httpserver"
	"gorm.io/gorm"
)

func newCatalogHandlerOptions(db *gorm.DB, policy *authzinfra.PolicyEngine, defaultOrganizationValue string, argoClient *argoinfra.Client) (httpserver.CatalogHandlerOptions, error) {
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
	return catalogHandlerOptions(db, service, argoClient)
}

func catalogHandlerOptions(db *gorm.DB, service *catalogapp.Service, argoClient *argoinfra.Client) (httpserver.CatalogHandlerOptions, error) {
	status, err := argoinfra.NewStatusRepository(db)
	if err != nil {
		return httpserver.CatalogHandlerOptions{}, err
	}
	options := httpserver.CatalogHandlerOptions{Service: service, StatusReader: status}
	options.Runtime, err = newRuntimeService(db, service, argoClient)
	return options, err
}

func newRuntimeService(db *gorm.DB, catalog *catalogapp.Service, argoClient *argoinfra.Client) (*argoapp.RuntimeService, error) {
	if argoClient == nil {
		return nil, nil
	}
	auditor, err := argoinfra.NewRuntimeAuditWriter(db)
	if err != nil {
		return nil, err
	}
	return argoapp.NewRuntimeService(argoapp.RuntimeServiceOptions{Catalog: catalog, Reader: argoClient, Auditor: auditor})
}
