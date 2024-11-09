package cmd

import (
	"context"
	"fmt"
	"os"
	"time"

	"go.ntppool.org/common/logger"
	"go.ntppool.org/common/tracing"
	"go.ntppool.org/monitor/api"
	apitls "go.ntppool.org/monitor/api/tls"
	"go.ntppool.org/monitor/client/auth"
)

func InitTracing(name string, cauth *auth.ClientAuth) (tracing.TpShutdownFunc, error) {
	capool, err := apitls.CAPool()
	if err != nil {
		return nil, err
	}

	ctx := context.Background()
	deployEnv, err := api.GetDeploymentEnvironmentFromName(name)
	if err != nil {
		return nil, fmt.Errorf("could not get deployment environment: %w", err)
	}

	endpoint := "https://api-buzz.mon.ntppool.dev/"

	if ep := os.Getenv("OTEL_EXPORTER_OTLP_ENDPOINT"); len(ep) > 0 {
		endpoint = ""
	}

	tpShutdownFn, err := tracing.InitTracer(ctx,
		&tracing.TracerConfig{
			ServiceName:         "monitor",
			Environment:         deployEnv.String(),
			RootCAs:             capool,
			CertificateProvider: cauth.GetClientCertificate,
			EndpointURL:         endpoint,
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
