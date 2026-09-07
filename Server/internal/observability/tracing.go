package observability

import (
	"context"
	"fmt"

	"go.opentelemetry.io/otel/exporters/otlp/otlptrace/otlptracegrpc"
	"go.opentelemetry.io/otel/sdk/resource"
	sdktrace "go.opentelemetry.io/otel/sdk/trace"
	semconv "go.opentelemetry.io/otel/semconv/v1.39.0"

	"github.com/vincent119/ReleaseHub/Server/internal/config"
)

// NewTracerProvider creates a process-scoped tracer provider without an exporter when disabled.
func NewTracerProvider(
	ctx context.Context,
	cfg config.TracingConfig,
	component string,
) (*sdktrace.TracerProvider, error) {
	res, err := resource.Merge(
		resource.Default(),
		resource.NewWithAttributes(
			semconv.SchemaURL,
			semconv.ServiceName("releasehub-"+component),
			semconv.ServiceNamespace("releasehub"),
		),
	)
	if err != nil {
		return nil, fmt.Errorf("create OpenTelemetry resource: %w", err)
	}

	options := []sdktrace.TracerProviderOption{
		sdktrace.WithResource(res),
		sdktrace.WithSampler(sdktrace.ParentBased(sdktrace.NeverSample())),
	}
	if cfg.Enabled {
		exporterOptions := []otlptracegrpc.Option{otlptracegrpc.WithEndpoint(cfg.OTLPEndpoint)}
		if cfg.Insecure {
			exporterOptions = append(exporterOptions, otlptracegrpc.WithInsecure())
		}
		exporter, exportErr := otlptracegrpc.New(ctx, exporterOptions...)
		if exportErr != nil {
			return nil, fmt.Errorf("create OTLP trace exporter: %w", exportErr)
		}
		options = append(options,
			sdktrace.WithBatcher(exporter),
			sdktrace.WithSampler(sdktrace.ParentBased(sdktrace.TraceIDRatioBased(cfg.SampleRatio))),
		)
	}
	return sdktrace.NewTracerProvider(options...), nil
}
