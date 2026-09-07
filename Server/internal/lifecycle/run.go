// Package lifecycle centralizes ReleaseHub process startup and shutdown.
package lifecycle

import (
	"log/slog"
	"time"

	"github.com/vincent119/commons/graceful"
	"github.com/vincent119/zlogger"

	"github.com/vincent119/ReleaseHub/Server/internal/observability"
)

// Run executes a process task and LIFO cleanup through commons/graceful.
func Run(
	task graceful.Task,
	shutdownTimeout time.Duration,
	logger *zlogger.Logger,
	cleanups ...graceful.Cleaner,
) error {
	options := []graceful.Option{
		graceful.WithTimeout(shutdownTimeout),
		graceful.WithLogger(slog.New(observability.NewSlogHandler(logger))),
	}
	for _, cleanup := range cleanups {
		options = append(options, graceful.WithCleanup(cleanup))
	}
	return graceful.Run(task, options...)
}
