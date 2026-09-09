package bootstrap

import (
	"context"
	"errors"
	"fmt"
	"strings"

	argoinfra "github.com/vincent119/ReleaseHub/Server/internal/argocd/infrastructure"
	"github.com/vincent119/ReleaseHub/Server/internal/config"
	"github.com/vincent119/ReleaseHub/Server/internal/infrastructure/database"
	"github.com/vincent119/ReleaseHub/Server/internal/observability"
	"gorm.io/gorm"
)

type processResources struct {
	runtime    *runtime
	db         *gorm.DB
	argoClient *argoinfra.Client
}

func newProcessResources(cfg config.Config, pool config.PoolConfig, component string) (*processResources, error) {
	runtime, err := newRuntime(context.Background(), cfg, component)
	if err != nil {
		return nil, fmt.Errorf("initialize %s runtime: %w", component, err)
	}
	resources := &processResources{runtime: runtime}
	resources.db, err = database.Open(cfg.Database, pool)
	if err != nil {
		return nil, resources.fail(fmt.Errorf("open %s database: %w", component, err))
	}
	if shouldCreateArgoCDClient(component, cfg.ArgoCD) {
		resources.argoClient, err = argoinfra.NewClient(cfg.ArgoCD)
		if err != nil {
			return nil, resources.fail(fmt.Errorf("create Argo CD client: %w", err))
		}
	}
	return resources, nil
}

func shouldCreateArgoCDClient(component string, cfg config.ArgoCDConfig) bool {
	if component != observability.ComponentAPI {
		return true
	}
	return strings.TrimSpace(cfg.Address) != "" || strings.TrimSpace(cfg.Token) != ""
}

func (r *processResources) fail(cause error) error {
	return errors.Join(cause, r.closeArgo(), database.Close(r.db), r.runtime.closeTracer(context.Background()), r.runtime.closeLogger(context.Background()))
}

func (r *processResources) closeAfterRun(ctx context.Context) error {
	return errors.Join(r.closeArgo(), database.Close(r.db), r.runtime.closeLogger(ctx))
}

func (r *processResources) closeArgo() error {
	if r.argoClient == nil {
		return nil
	}
	return r.argoClient.Close()
}
