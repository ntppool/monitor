package server

import (
	"context"
	"log"
	"net/http"

	sctx "go.ntppool.org/monitor/server/context"
)

func getCertificateName(ctx context.Context) string {
	cn := ctx.Value(sctx.CertificateKey)
	if name, ok := cn.(string); ok {
		return name
	}
	log.Fatalf("certificateKey didn't return a string")
	return ""
}

func WithUserAgent(base http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		ctx := r.Context()
		ua := r.Header.Get("User-Agent")
		ctx = context.WithValue(ctx, sctx.ClientVersion, ua)
		r = r.WithContext(ctx)

		base.ServeHTTP(w, r)
	})
}
