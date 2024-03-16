package server

import (
	"context"
	"net/http"

	"go.ntppool.org/common/logger"
	sctx "go.ntppool.org/monitor/server/context"
	"go.opentelemetry.io/otel/attribute"
	otrace "go.opentelemetry.io/otel/trace"
)

func getCertificateName(ctx context.Context) string {
	cn := ctx.Value(sctx.CertificateKey)
	if name, ok := cn.(string); ok {
		return name
	}
	logger.Setup().Error("certificateKey didn't return a string")
	return ""
}

func WithUserAgent(base http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		ctx := r.Context()
		ua := r.Header.Get("User-Agent")
		ctx = context.WithValue(ctx, sctx.ClientVersionKey, ua)
		r = r.WithContext(ctx)

		span := otrace.SpanFromContext(ctx)
		span.SetAttributes(attribute.String("User-Agent", ua))

		// logger.Setup().InfoContext(ctx, "got user agent", "ua", ua, "traceX", span.SpanContext().TraceID())

		base.ServeHTTP(w, r)
	})
}
