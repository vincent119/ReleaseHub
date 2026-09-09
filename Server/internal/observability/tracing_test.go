package observability

import (
	"context"
	"testing"

	"github.com/vincent119/ReleaseHub/Server/internal/config"
)

func TestTracerProviderStartsWhenTracingIsDisabled(t *testing.T) {
	provider, err := NewTracerProvider(
		context.Background(),
		config.TracingConfig{},
		ComponentMigrate,
	)
	if err != nil {
		t.Fatalf("create disabled tracer provider: %v", err)
	}
	t.Cleanup(func() {
		if err := provider.Shutdown(context.Background()); err != nil {
			t.Fatalf("shut down tracer provider: %v", err)
		}
	})
}

func TestResourceIncludesRuntimeIdentity(t *testing.T) {
	res, err := newResource(ComponentMigrate)
	if err != nil {
		t.Fatalf("create resource: %v", err)
	}

	attributes := make(map[string]string)
	for _, attr := range res.Attributes() {
		attributes[string(attr.Key)] = attr.Value.AsString()
	}
	if got := attributes["service.name"]; got != "releasehub-migrate" {
		t.Fatalf("service name mismatch, got %q", got)
	}
	if got := attributes["service.namespace"]; got != "releasehub" {
		t.Fatalf("service namespace mismatch, got %q", got)
	}
}
