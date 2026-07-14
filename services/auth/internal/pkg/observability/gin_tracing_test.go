package observability

import (
	"context"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/gin-gonic/gin"
	"go.opentelemetry.io/otel"
	"go.opentelemetry.io/otel/attribute"
	"go.opentelemetry.io/otel/propagation"
	sdktrace "go.opentelemetry.io/otel/sdk/trace"
	"go.opentelemetry.io/otel/sdk/trace/tracetest"
)

func TestGinTracingCreatesRequestSpan(t *testing.T) {
	gin.SetMode(gin.TestMode)

	exporter := tracetest.NewInMemoryExporter()
	provider := sdktrace.NewTracerProvider(
		sdktrace.WithSyncer(exporter),
		sdktrace.WithSampler(sdktrace.AlwaysSample()),
	)
	previousProvider := otel.GetTracerProvider()
	previousPropagator := otel.GetTextMapPropagator()
	otel.SetTracerProvider(provider)
	otel.SetTextMapPropagator(propagation.TraceContext{})
	defer func() {
		_ = provider.Shutdown(context.Background())
		otel.SetTracerProvider(previousProvider)
		otel.SetTextMapPropagator(previousPropagator)
	}()

	router := gin.New()
	router.Use(func(c *gin.Context) {
		c.Set("request_id", "req-123")
		c.Next()
	})
	router.Use(GinTracing("test-service", "request_id"))
	router.GET("/users/:id", func(c *gin.Context) {
		c.Status(http.StatusAccepted)
	})

	req := httptest.NewRequest(http.MethodGet, "/users/42", nil)
	recorder := httptest.NewRecorder()

	router.ServeHTTP(recorder, req)

	if recorder.Code != http.StatusAccepted {
		t.Fatalf("expected status %d, got %d", http.StatusAccepted, recorder.Code)
	}

	spans := exporter.GetSpans()
	if len(spans) != 1 {
		t.Fatalf("expected one span, got %d", len(spans))
	}

	span := spans[0]
	if span.Name != "GET /users/:id" {
		t.Fatalf("expected route span name, got %q", span.Name)
	}

	attrs := attrsByKey(span.Attributes)
	assertAttr(t, attrs, "method", http.MethodGet)
	assertAttr(t, attrs, "path", "/users/:id")
	assertAttr(t, attrs, "request_id", "req-123")
	assertAttr(t, attrs, "status", int64(http.StatusAccepted))
	assertAttr(t, attrs, "http.route", "/users/:id")
}

func attrsByKey(attrs []attribute.KeyValue) map[string]attribute.Value {
	result := make(map[string]attribute.Value, len(attrs))
	for _, attr := range attrs {
		result[string(attr.Key)] = attr.Value
	}
	return result
}

func assertAttr(t *testing.T, attrs map[string]attribute.Value, key string, want any) {
	t.Helper()

	value, ok := attrs[key]
	if !ok {
		t.Fatalf("expected attribute %q", key)
	}

	switch want := want.(type) {
	case string:
		if value.AsString() != want {
			t.Fatalf("expected attribute %q=%q, got %q", key, want, value.AsString())
		}
	case int64:
		if value.AsInt64() != want {
			t.Fatalf("expected attribute %q=%d, got %d", key, want, value.AsInt64())
		}
	default:
		t.Fatalf("unsupported assertion type %T", want)
	}
}
