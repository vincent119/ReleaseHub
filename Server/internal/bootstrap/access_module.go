package bootstrap

import (
	authzapp "github.com/vincent119/ReleaseHub/Server/internal/authorization/application"
	authzinfra "github.com/vincent119/ReleaseHub/Server/internal/authorization/infrastructure"
	identityapp "github.com/vincent119/ReleaseHub/Server/internal/identity/application"
	"github.com/vincent119/ReleaseHub/Server/internal/transport/httpserver"
	"gorm.io/gorm"
)

func newAccessHandlerOptions(db *gorm.DB, policy *authzinfra.PolicyEngine, local *identityapp.LocalAuthService) (httpserver.AccessHandlerOptions, error) {
	repository, err := authzinfra.NewAccessManagementRepository(db)
	if err != nil {
		return httpserver.AccessHandlerOptions{}, err
	}
	service, err := authzapp.NewAccessManagementService(repository, policy)
	return httpserver.AccessHandlerOptions{Service: service, LocalUsers: local}, err
}
