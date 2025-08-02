package cmd

import (
	"context"
	"fmt"
	"os"
	"time"

	"go.ntppool.org/common/config/depenv"
	"go.ntppool.org/common/logger"
	"go.ntppool.org/common/metrics"
	"go.ntppool.org/common/tracing"
	apitls "go.ntppool.org/monitor/api/tls"
	clientmetrics "go.ntppool.org/monitor/client/metrics"
)

func InitTracing(ctx context.Context, deployEnv depenv.DeploymentEnvironment, tlsAuth apitls.AuthProvider) (tracing.TpShutdownFunc, error) {
	capool, err := apitls.CAPool()
	if err != nil {
		return nil, err
	}

	if tlsAuth == nil {
		return nil, apitls.ErrNoAuthProvider
	}

	endpoint := "api-buzz.mon.ntppool.dev"

	if ep := os.Getenv("OTEL_EXPORTER_OTLP_ENDPOINT"); len(ep) > 0 {
		endpoint = ""
	} else {
		// Set the env var so autoexport log exporter uses our endpoint
		if err := os.Setenv("OTEL_EXPORTER_OTLP_ENDPOINT", "https://"+endpoint+":4318"); err != nil {
			return nil, fmt.Errorf("failed to set OTEL_EXPORTER_OTLP_ENDPOINT: %w", err)
		}
	}

	tpShutdownFn, err := tracing.InitTracer(ctx,
		&tracing.TracerConfig{
			ServiceName:         "monitor",
			Environment:         deployEnv.String(),
			RootCAs:             capool,
			CertificateProvider: tlsAuth.GetClientCertificate,
			Endpoint:            endpoint,
		},
	)
	if err != nil {
		return nil, err
	}

	// Initialize OpenTelemetry metrics
	if err := metrics.Setup(ctx); err != nil {
		log := logger.FromContext(ctx)
		log.WarnContext(ctx, "metrics setup failed", "err", err)
		// Continue anyway - metrics are not critical for operation
	}

	// Initialize client-specific metric instruments
	if err := clientmetrics.InitInstruments(); err != nil {
		log := logger.FromContext(ctx)
		log.WarnContext(ctx, "client metrics instruments setup failed", "err", err)
		// Continue anyway - metrics are not critical for operation
	}

	return func(ctx context.Context) error {
		log := logger.Setup()
		log.Debug("shutting down trace and metrics providers")
		shutdownCtx, cancel := context.WithTimeout(ctx, 5*time.Second)
		defer cancel()

		// Shutdown metrics first
		if err := metrics.Shutdown(shutdownCtx); err != nil {
			log.WarnContext(shutdownCtx, "failed to shutdown metrics", "err", err)
		}

		// Then shutdown tracing
		err := tpShutdownFn(shutdownCtx)
		// log.Debug("trace provider shutdown", "err", err)
		return err
	}, nil
}
