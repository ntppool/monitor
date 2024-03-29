package cmd

import (
	"context"
	"fmt"
	"time"

	"go.ntppool.org/common/logger"
	"go.ntppool.org/common/tracing"
	"go.ntppool.org/monitor/api"
	apitls "go.ntppool.org/monitor/api/tls"
	"go.ntppool.org/monitor/client/auth"
)

func InitTracing(name string, cauth *auth.ClientAuth) (func(), error) {
	capool, err := apitls.CAPool()
	if err != nil {
		return nil, err
	}

	ctx := context.Background()
	deployEnv, err := api.GetDeploymentEnvironmentFromName(name)
	if err != nil {
		return nil, fmt.Errorf("could not get deployment environment: %w", err)
	}
	tpShutdownFn, err := tracing.InitTracer(ctx,
		&tracing.TracerConfig{
			ServiceName:         "monitor",
			Environment:         deployEnv.String(),
			RootCAs:             capool,
			CertificateProvider: cauth.GetClientCertificate,
			Endpoint:            "otelcol.ntppool.net:443",
		},
	)
	if err != nil {
		return nil, err
	}

	return func() {
		log := logger.Setup()
		log.Debug("Shutting down trace provider")
		shutdownCtx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cancel()
		tpShutdownFn(shutdownCtx)
		log.Debug("trace provider shutdown")
	}, nil

}
