package observability

import (
	"context"
	"net/url"
	"strings"

	"github.com/go-admin-kit/services/audit/internal/config"
	"go.opentelemetry.io/otel"
	"go.opentelemetry.io/otel/attribute"
	"go.opentelemetry.io/otel/exporters/otlp/otlptrace/otlptracegrpc"
	"go.opentelemetry.io/otel/propagation"
	"go.opentelemetry.io/otel/sdk/resource"
	sdktrace "go.opentelemetry.io/otel/sdk/trace"
)

const defaultOTLPEndpoint = "localhost:4317"

// InitTracer configures the process-wide OpenTelemetry tracer provider.
func InitTracer(ctx context.Context, cfg config.TracingConfig) (func(context.Context) error, error) {
	if !cfg.Enabled {
		return func(context.Context) error { return nil }, nil
	}

	endpoint, insecure := normalizeOTLPEndpoint(cfg.OTLPEndpoint)
	exporterOpts := []otlptracegrpc.Option{
		otlptracegrpc.WithEndpoint(endpoint),
	}
	if insecure {
		exporterOpts = append(exporterOpts, otlptracegrpc.WithInsecure())
	}

	exporter, err := otlptracegrpc.New(ctx, exporterOpts...)
	if err != nil {
		return nil, err
	}

	res, err := resource.New(ctx,
		resource.WithAttributes(
			attribute.String("service.name", fallbackString(cfg.ServiceName, "go-admin-kit")),
			attribute.String("deployment.environment", fallbackString(cfg.Environment, "development")),
			attribute.String("deployment.environment.name", fallbackString(cfg.Environment, "development")),
		),
	)
	if err != nil {
		_ = exporter.Shutdown(ctx)
		return nil, err
	}

	provider := sdktrace.NewTracerProvider(
		sdktrace.WithBatcher(exporter),
		sdktrace.WithResource(res),
		sdktrace.WithSampler(sdktrace.ParentBased(sdktrace.TraceIDRatioBased(normalizeSampleRatio(cfg.SampleRatio)))),
	)
	otel.SetTracerProvider(provider)
	otel.SetTextMapPropagator(
		propagation.NewCompositeTextMapPropagator(
			propagation.TraceContext{},
			propagation.Baggage{},
		),
	)

	return provider.Shutdown, nil
}

func normalizeSampleRatio(ratio float64) float64 {
	switch {
	case ratio < 0:
		return 0
	case ratio > 1:
		return 1
	default:
		return ratio
	}
}

func normalizeOTLPEndpoint(endpoint string) (string, bool) {
	endpoint = strings.TrimSpace(endpoint)
	if endpoint == "" {
		return defaultOTLPEndpoint, true
	}

	parsed, err := url.Parse(endpoint)
	if err == nil && parsed.Scheme != "" && parsed.Host != "" {
		return parsed.Host, parsed.Scheme != "https"
	}

	endpoint = strings.TrimPrefix(endpoint, "http://")
	endpoint = strings.TrimPrefix(endpoint, "https://")
	if slash := strings.Index(endpoint, "/"); slash >= 0 {
		endpoint = endpoint[:slash]
	}
	if endpoint == "" {
		return defaultOTLPEndpoint, true
	}
	return endpoint, true
}

func fallbackString(value string, fallback string) string {
	value = strings.TrimSpace(value)
	if value == "" {
		return fallback
	}
	return value
}
