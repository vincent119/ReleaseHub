package bootstrap

import (
	"context"
	"errors"
	"fmt"

	"github.com/vincent119/zlogger"

	"github.com/vincent119/ReleaseHub/Server/internal/config"
	"github.com/vincent119/ReleaseHub/Server/internal/infrastructure/database"
	"github.com/vincent119/ReleaseHub/Server/internal/observability"
	"github.com/vincent119/ReleaseHub/Server/migrations"
)

// RunMigrate creates an independent Migrate composition root that API and Worker never invoke implicitly.
func RunMigrate(cfg config.Config) (result error) {
	runtime, err := newRuntime(context.Background(), cfg, observability.ComponentMigrate)
	if err != nil {
		return fmt.Errorf("initialize Migrate runtime: %w", err)
	}
	defer func() {
		result = errors.Join(result, runtime.closeLogger(context.Background()))
	}()
	defer func() {
		result = errors.Join(result, runtime.closeTracer(context.Background()))
	}()
	runtime.logger.Info("starting database migrations")
	if err := migrations.Up(database.URL(cfg.Database)); err != nil {
		return fmt.Errorf("apply database migrations: %w", err)
	}
	runtime.logger.Info("database migrations completed", zlogger.String("database", cfg.Database.Database))
	return nil
}
