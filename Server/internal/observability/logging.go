// Package observability centralizes ReleaseHub logging, metrics, and tracing boundaries.
package observability

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"strings"

	"github.com/vincent119/zlogger"
)

const (
	ComponentAPI     = "api"
	ComponentWorker  = "worker"
	ComponentMigrate = "migrate"
)

// NewLogger creates a zlogger instance and a component logger owned by the caller.
func NewLogger(cfg zlogger.Config, component string) (*zlogger.Instance, *zlogger.Logger, error) {
	instance, err := zlogger.New(&cfg)
	if err != nil {
		return nil, nil, fmt.Errorf("create zlogger: %w", err)
	}
	logger, err := WithComponent(instance.Logger(), component)
	if err != nil {
		_ = instance.Close()
		return nil, nil, err
	}
	return instance, logger, nil
}

// WithComponent creates a logger with a fixed runtime component field.
func WithComponent(logger *zlogger.Logger, component string) (*zlogger.Logger, error) {
	if logger == nil {
		return nil, errors.New("logger must not be nil")
	}
	switch component {
	case ComponentAPI, ComponentWorker, ComponentMigrate:
		return logger.With(zlogger.String("component", component)), nil
	default:
		return nil, fmt.Errorf("unsupported component %q", component)
	}
}

// NewSlogHandler forwards the commons/graceful slog contract to zlogger.
func NewSlogHandler(logger *zlogger.Logger) slog.Handler {
	return &slogHandler{logger: logger}
}

type slogHandler struct {
	logger *zlogger.Logger
	attrs  []slog.Attr
	groups []string
}

func (h *slogHandler) Enabled(_ context.Context, level slog.Level) bool {
	return h.logger.Core().Enabled(toZloggerLevel(level))
}

func (h *slogHandler) Handle(_ context.Context, record slog.Record) error {
	fields := make([]zlogger.Field, 0, len(h.attrs)+record.NumAttrs())
	for _, attr := range h.attrs {
		fields = append(fields, h.field(attr))
	}
	record.Attrs(func(attr slog.Attr) bool {
		fields = append(fields, h.field(attr))
		return true
	})

	switch {
	case record.Level >= slog.LevelError:
		h.logger.Error(record.Message, fields...)
	case record.Level >= slog.LevelWarn:
		h.logger.Warn(record.Message, fields...)
	case record.Level >= slog.LevelInfo:
		h.logger.Info(record.Message, fields...)
	default:
		h.logger.Debug(record.Message, fields...)
	}
	return nil
}

func (h *slogHandler) WithAttrs(attrs []slog.Attr) slog.Handler {
	cloned := *h
	cloned.attrs = append(append([]slog.Attr(nil), h.attrs...), attrs...)
	cloned.groups = append([]string(nil), h.groups...)
	return &cloned
}

func (h *slogHandler) WithGroup(name string) slog.Handler {
	if name == "" {
		return h
	}
	cloned := *h
	cloned.attrs = append([]slog.Attr(nil), h.attrs...)
	cloned.groups = append(append([]string(nil), h.groups...), name)
	return &cloned
}

func (h *slogHandler) field(attr slog.Attr) zlogger.Field {
	key := attr.Key
	if len(h.groups) > 0 {
		key = strings.Join(append(append([]string(nil), h.groups...), key), ".")
	}
	return zlogger.Any(key, attr.Value.Any())
}

func toZloggerLevel(level slog.Level) zlogger.Level {
	switch {
	case level >= slog.LevelError:
		return zlogger.ErrorLevel
	case level >= slog.LevelWarn:
		return zlogger.WarnLevel
	case level >= slog.LevelInfo:
		return zlogger.InfoLevel
	default:
		return zlogger.DebugLevel
	}
}
