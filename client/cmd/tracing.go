package cmd

import (
	"context"
	"fmt"
	"os"
	"time"

	"go.ntppool.org/common/config/depenv"
	"go.ntppool.org/common/logger"
	"go.ntppool.org/common/tracing"
	apitls "go.ntppool.org/monitor/api/tls"
)

func InitTracing(ctx context.Context, deployEnv depenv.DeploymentEnvironment, tlsAuth apitls.CertificateProvider) (tracing.TpShutdownFunc, error) {
	capool, err := apitls.CAPool()
	if err != nil {
		return nil, err
	}

	if tlsAuth == nil {
		return nil, apitls.ErrNoCertificateProvider
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

	return func(ctx context.Context) error {
		log := logger.Setup()
		log.Debug("shutting down trace provider")
		shutdownCtx, cancel := context.WithTimeout(ctx, 5*time.Second)
		defer cancel()
		err := tpShutdownFn(shutdownCtx)
		// log.Debug("trace provider shutdown", "err", err)
		return err
	}, nil
}
