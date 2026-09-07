// Package bootstrap provides independent composition roots for API, Worker, and Migrate.
package bootstrap

import (
	"context"
	"errors"
	"fmt"
	"syscall"

	"github.com/vincent119/zlogger"
	sdktrace "go.opentelemetry.io/otel/sdk/trace"

	"github.com/vincent119/ReleaseHub/Server/internal/config"
	"github.com/vincent119/ReleaseHub/Server/internal/observability"
)

type runtime struct {
	loggerInstance *zlogger.Instance
	logger         *zlogger.Logger
	tracerProvider *sdktrace.TracerProvider
}

func newRuntime(ctx context.Context, cfg config.Config, component string) (*runtime, error) {
	instance, logger, err := observability.NewLogger(runtimeLoggerConfig(cfg), component)
	if err != nil {
		return nil, err
	}
	provider, err := observability.NewTracerProvider(ctx, cfg.Observability.Tracing, component)
	if err != nil {
		_ = instance.Close()
		return nil, err
	}
	logger.Info("runtime configured", zlogger.String("config", cfg.String()))
	return &runtime{loggerInstance: instance, logger: logger, tracerProvider: provider}, nil
}

func runtimeLoggerConfig(cfg config.Config) zlogger.Config {
	return zlogger.Config{
		Level:         cfg.Log.Level,
		Format:        cfg.Log.Format,
		Outputs:       cfg.Log.Outputs,
		AddCaller:     cfg.Log.AddCaller,
		AddStacktrace: cfg.Log.AddStacktrace,
		Development:   cfg.Log.Development,
		ColorEnabled:  cfg.Log.ColorEnabled,
	}
}

func (r *runtime) closeLogger(context.Context) error {
	if err := r.loggerInstance.Sync(); err != nil && !errors.Is(err, syscall.EINVAL) {
		r.logger.Warn("logger sync failed", zlogger.Err(err))
	}
	if err := r.loggerInstance.Close(); err != nil {
		return fmt.Errorf("close logger: %w", err)
	}
	return nil
}

func (r *runtime) closeTracer(ctx context.Context) error {
	if err := r.tracerProvider.Shutdown(ctx); err != nil {
		return fmt.Errorf("shutdown tracer provider: %w", err)
	}
	return nil
}
