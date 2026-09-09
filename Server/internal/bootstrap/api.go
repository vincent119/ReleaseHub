package bootstrap

import (
	"context"
	"errors"
	"fmt"
	"net/http"
	"strings"

	"github.com/prometheus/client_golang/prometheus"
	"github.com/vincent119/commons/graceful"
	"github.com/vincent119/zlogger"

	"github.com/vincent119/ReleaseHub/Server/internal/config"
	"github.com/vincent119/ReleaseHub/Server/internal/lifecycle"
	"github.com/vincent119/ReleaseHub/Server/internal/observability"
	"github.com/vincent119/ReleaseHub/Server/internal/transport/httpserver"
)

type apiApplication struct {
	cfg       config.Config
	resources *processResources
	modules   *apiModules
	readiness *httpserver.Readiness
	server    *http.Server
}

type httpRouterDependencies struct {
	cfg       config.Config
	resources *processResources
	modules   *apiModules
	readiness *httpserver.Readiness
	registry  *prometheus.Registry
	metrics   *observability.HTTPMetrics
}

// RunAPI creates and runs the API process.
func RunAPI(cfg config.Config, version string) (result error) {
	if err := validateAPIConfig(cfg); err != nil {
		return err
	}
	resources, err := newProcessResources(cfg, cfg.Database.Pools.API, observability.ComponentAPI)
	if err != nil {
		return err
	}
	defer func() {
		result = errors.Join(result, resources.closeAfterRun(context.Background()))
	}()
	app, err := newAPIApplication(cfg, version, resources)
	if err != nil {
		_ = resources.runtime.closeTracer(context.Background())
		return err
	}
	return app.run()
}

func validateAPIConfig(cfg config.Config) error {
	if strings.TrimSpace(cfg.OIDC.WebRedirectURL) == "" {
		return fmt.Errorf("web redirect URL is required")
	}
	return nil
}

func newAPIApplication(cfg config.Config, version string, resources *processResources) (*apiApplication, error) {
	modules, err := newAPIModules(cfg, version, resources)
	if err != nil {
		return nil, err
	}
	readiness := httpserver.NewReadiness()
	server, err := newHTTPServer(cfg, resources, modules, readiness)
	if err != nil {
		return nil, err
	}
	return &apiApplication{cfg: cfg, resources: resources, modules: modules, readiness: readiness, server: server}, nil
}

func newHTTPServer(
	cfg config.Config,
	resources *processResources,
	modules *apiModules,
	readiness *httpserver.Readiness,
) (*http.Server, error) {
	registry, metrics, err := newHTTPObservability()
	if err != nil {
		return nil, err
	}
	router := newHTTPRouter(httpRouterDependencies{
		cfg: cfg, resources: resources, modules: modules,
		readiness: readiness, registry: registry, metrics: metrics,
	})
	return configuredHTTPServer(cfg, router), nil
}

func newHTTPRouter(dependencies httpRouterDependencies) http.Handler {
	return httpserver.NewRouter(httpserver.RouterOptions{
		Logger:   dependencies.resources.runtime.logger,
		Registry: dependencies.registry, HTTPMetrics: dependencies.metrics,
		TracerProvider: dependencies.resources.runtime.tracerProvider,
		Readiness:      dependencies.readiness,
		MetricsPath:    dependencies.cfg.Observability.MetricsPath,
	}, httpserver.NewAPIHandler(dependencies.modules.handler))
}

func configuredHTTPServer(cfg config.Config, handler http.Handler) *http.Server {
	return &http.Server{
		Addr: cfg.API.Address, Handler: handler,
		ReadTimeout: cfg.API.ReadTimeout, WriteTimeout: cfg.API.WriteTimeout,
		IdleTimeout: cfg.API.IdleTimeout,
	}
}

func newHTTPObservability() (*prometheus.Registry, *observability.HTTPMetrics, error) {
	registry := prometheus.NewRegistry()
	registry.MustRegister(prometheus.NewGoCollector(), prometheus.NewProcessCollector(prometheus.ProcessCollectorOpts{}))
	metrics, err := observability.NewHTTPMetrics(registry)
	return registry, metrics, err
}

func (a *apiApplication) run() error {
	a.resources.runtime.logger.Info("API listening", zlogger.String("address", a.cfg.API.Address))
	return lifecycle.Run(
		parallelTasks(graceful.HTTPTask(a.server), a.modules.policyListener.Run),
		a.cfg.Runtime.ShutdownTimeout,
		a.resources.runtime.logger,
		a.resources.runtime.closeTracer,
		a.shutdownServer,
	)
}

func (a *apiApplication) shutdownServer(ctx context.Context) error {
	a.readiness.BeginShutdown()
	return a.server.Shutdown(ctx)
}
