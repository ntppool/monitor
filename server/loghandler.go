package server

import (
	"log/slog"
	"net/http"
	"time"
)

func WithLogger(h http.Handler, l *slog.Logger) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		ctx := r.Context()
		if !l.Enabled(ctx, slog.LevelInfo) {
			h.ServeHTTP(w, r)
			return
		}
		start := time.Now()
		h.ServeHTTP(w, r)
		dur := time.Since(start)

		// Extract monitor name from authentication context
		monitorName := getCertificateName(ctx)
		if monitorName == "" {
			monitorName = "unknown"
		}

		l.InfoContext(ctx, "",
			"monitor", monitorName,
			"path", r.URL.RequestURI(),
			"method", r.Method,
			"url", r.URL.String(),
			"client", r.RemoteAddr,
			"user_agent", r.UserAgent(),
			"host", r.Host,
			"duration", dur,
		)
	})
}
