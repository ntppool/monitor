package cmd

import (
	"context"
	"os"
	"time"

	"go.ntppool.org/common/config/depenv"
	"go.ntppool.org/common/logger"
	"go.ntppool.org/common/tracing"
	apitls "go.ntppool.org/monitor/api/tls"
)

func InitTracing(ctx context.Context, deployEnv depenv.DeploymentEnvironment, tlsAuth any) (tracing.TpShutdownFunc, error) {
	capool, err := apitls.CAPool()
	if err != nil {
		return nil, err
	}

	endpoint := "https://api-buzz.mon.ntppool.dev/"

	if ep := os.Getenv("OTEL_EXPORTER_OTLP_ENDPOINT"); len(ep) > 0 {
		endpoint = ""
	}

	tpShutdownFn, err := tracing.InitTracer(ctx,
		&tracing.TracerConfig{
			ServiceName: "monitor",
			Environment: deployEnv.String(),
			RootCAs:     capool,
			// todo: get certificates from configuration
			// CertificateProvider: cauth.GetClientCertificate,
			EndpointURL: endpoint,
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
