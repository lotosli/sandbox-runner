package helper

import (
	"context"
	"net/http"
	"os"
	"strconv"

	"go.opentelemetry.io/otel"
	"go.opentelemetry.io/otel/attribute"
	"go.opentelemetry.io/otel/trace"
)

func StartSpan(ctx context.Context, name string, attrs ...attribute.KeyValue) (context.Context, trace.Span) {
	tracer := otel.Tracer("github.com/lotosli/sandbox-runner/pkg/helper")
	baseAttrs := RunAttrsFromEnv()
	baseAttrs = append(baseAttrs, attrs...)
	return tracer.Start(ctx, name, trace.WithAttributes(baseAttrs...))
}

func AddEvent(ctx context.Context, name string, attrs ...attribute.KeyValue) {
	trace.SpanFromContext(ctx).AddEvent(name, trace.WithAttributes(attrs...))
}

func RunAttrsFromEnv() []attribute.KeyValue {
	attrs := []attribute.KeyValue{}
	if runID := os.Getenv("RUN_ID"); runID != "" {
		attrs = append(attrs, attribute.String("run_id", runID))
	}
	if attempt := os.Getenv("ATTEMPT"); attempt != "" {
		if value, err := strconv.Atoi(attempt); err == nil {
			attrs = append(attrs, attribute.Int("attempt", value))
		}
	}
	if sandboxID := os.Getenv("SANDBOX_ID"); sandboxID != "" {
		attrs = append(attrs, attribute.String("sandbox_id", sandboxID))
	}
	return attrs
}

func WrapHTTPHandler(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		ctx, span := StartSpan(r.Context(), "http.request", attribute.String("http.method", r.Method), attribute.String("http.route", r.URL.Path))
		defer span.End()
		next.ServeHTTP(w, r.WithContext(ctx))
	})
}
