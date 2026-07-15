package observability

import (
	"errors"
	"net/http"
	"strings"

	"github.com/gin-gonic/gin"
	"go.opentelemetry.io/otel"
	"go.opentelemetry.io/otel/attribute"
	"go.opentelemetry.io/otel/codes"
	"go.opentelemetry.io/otel/propagation"
	"go.opentelemetry.io/otel/trace"
)

// GinTracing starts a server span for each Gin request.
func GinTracing(serviceName string, requestIDKey string) gin.HandlerFunc {
	serviceName = strings.TrimSpace(serviceName)
	if serviceName == "" {
		serviceName = "go-admin-kit"
	}
	tracer := otel.Tracer(serviceName)
	propagator := otel.GetTextMapPropagator()

	return func(c *gin.Context) {
		requestID := getRequestID(c, requestIDKey)
		requestPath := c.Request.URL.Path
		spanName := c.Request.Method + " " + requestPath

		ctx := propagator.Extract(c.Request.Context(), propagation.HeaderCarrier(c.Request.Header))
		ctx, span := tracer.Start(ctx, spanName,
			trace.WithSpanKind(trace.SpanKindServer),
			trace.WithAttributes(
				attribute.String("method", c.Request.Method),
				attribute.String("path", requestPath),
				attribute.String("request_id", requestID),
				attribute.String("http.request.method", c.Request.Method),
				attribute.String("url.path", requestPath),
				attribute.String("user_agent.original", c.Request.UserAgent()),
				attribute.String("client.address", c.ClientIP()),
			),
		)
		defer span.End()

		c.Request = c.Request.WithContext(ctx)
		c.Next()

		status := c.Writer.Status()
		if status == 0 {
			status = http.StatusOK
		}

		routePath := c.FullPath()
		if routePath == "" {
			routePath = requestPath
		}
		span.SetName(c.Request.Method + " " + routePath)
		span.SetAttributes(
			attribute.String("path", routePath),
			attribute.String("http.route", routePath),
			attribute.Int("status", status),
			attribute.Int("http.response.status_code", status),
			attribute.String("request_id", getRequestID(c, requestIDKey)),
		)

		if len(c.Errors) > 0 {
			err := errors.New(c.Errors.String())
			span.RecordError(err)
			span.SetStatus(codes.Error, err.Error())
			return
		}
		if status >= http.StatusInternalServerError {
			span.SetStatus(codes.Error, http.StatusText(status))
		}
	}
}

func getRequestID(c *gin.Context, requestIDKey string) string {
	if requestIDKey == "" {
		requestIDKey = "request_id"
	}
	if value, ok := c.Get(requestIDKey); ok {
		if requestID, ok := value.(string); ok {
			return requestID
		}
	}
	return ""
}
