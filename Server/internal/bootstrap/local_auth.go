package bootstrap

import (
	"context"

	"gorm.io/gorm"

	"github.com/vincent119/ReleaseHub/Server/internal/config"
	identityapp "github.com/vincent119/ReleaseHub/Server/internal/identity/application"
	identityinfra "github.com/vincent119/ReleaseHub/Server/internal/identity/infrastructure"
)

func newAPIModules(cfg config.Config, version string, resources *processResources) (*apiModules, error) {
	local, err := bootstrapLocalManager(cfg, resources.db)
	if err != nil {
		return nil, err
	}
	return newAPIModulesWithLocal(cfg, version, resources, local)
}

func bootstrapLocalManager(cfg config.Config, db *gorm.DB) (*identityapp.LocalAuthService, error) {
	service, err := newLocalAuthService(cfg, db)
	if err != nil {
		return nil, err
	}
	if err := service.Bootstrap(context.Background(), cfg.ManagerPassword); err != nil {
		return nil, err
	}
	return service, nil
}

func newLocalAuthService(cfg config.Config, db *gorm.DB) (*identityapp.LocalAuthService, error) {
	repository, err := identityinfra.NewLocalAuthRepository(db)
	if err != nil {
		return nil, err
	}
	sessions, err := identityinfra.NewSessionRepository(db)
	if err != nil {
		return nil, err
	}
	service, err := identityapp.NewSessionService(sessions, identityapp.SystemClock{}, cfg.Session.IdleTimeout, cfg.Session.AbsoluteTTL)
	if err != nil {
		return nil, err
	}
	return identityapp.NewLocalAuthService(repository, service)
}
