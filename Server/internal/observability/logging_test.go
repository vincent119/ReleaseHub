package observability_test

import (
	"context"
	"log/slog"
	"testing"

	"github.com/vincent119/zlogger"
	"go.uber.org/zap"
	"go.uber.org/zap/zaptest/observer"

	"github.com/vincent119/ReleaseHub/Server/internal/observability"
)

func TestComponentLoggerAddsRuntimeIdentity(t *testing.T) {
	for _, component := range []string{
		observability.ComponentAPI,
		observability.ComponentWorker,
		observability.ComponentMigrate,
	} {
		t.Run(component, func(t *testing.T) {
			core, recorded := observer.New(zap.InfoLevel)
			logger := zap.New(core)

			componentLogger, err := observability.WithComponent(logger, component)
			if err != nil {
				t.Fatalf("create component logger: %v", err)
			}
			componentLogger.Info("runtime started")

			entry := recorded.All()[0]
			if got := entry.ContextMap()["component"]; got != component {
				t.Fatalf("component field mismatch, got %#v", got)
			}
		})
	}
}

func TestComponentLoggerRejectsUnknownComponent(t *testing.T) {
	_, err := observability.WithComponent(zlogger.NewNop(), "scheduler")
	if err == nil {
		t.Fatal("unknown component should be rejected")
	}
}

func TestSlogAdapterForwardsLifecycleFieldsToZlogger(t *testing.T) {
	core, recorded := observer.New(zap.InfoLevel)
	componentLogger, err := observability.WithComponent(zap.New(core), observability.ComponentAPI)
	if err != nil {
		t.Fatalf("create component logger: %v", err)
	}

	logger := slog.New(observability.NewSlogHandler(componentLogger))
	logger.InfoContext(context.Background(), "starting task", "timeout", "30s")

	entry := recorded.All()[0]
	fields := entry.ContextMap()
	if fields["component"] != "api" || fields["timeout"] != "30s" {
		t.Fatalf("lifecycle log fields were not fully forwarded: %#v", fields)
	}
}
