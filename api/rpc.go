package api

import (
	"context"
	"crypto/tls"
	"net/http"
	"os"
	"time"

	"connectrpc.com/connect"
	"connectrpc.com/otelconnect"

	"go.ntppool.org/common/config/depenv"
	"go.ntppool.org/common/logger"
	"go.ntppool.org/common/version"
	apitls "go.ntppool.org/monitor/api/tls"
	apiv2connect "go.ntppool.org/monitor/gen/monitor/v2/monitorv2connect"
)

func httpClient(cm apitls.CertificateProvider) (*http.Client, error) {
	capool, err := apitls.CAPool()
	if err != nil {
		return nil, err
	}

	tlsConfig := &tls.Config{
		InsecureSkipVerify:   false,
		GetClientCertificate: cm.GetClientCertificate,
		RootCAs:              capool,
	}

	// tlsConfig.BuildNameToCertificate()
	transport := &http.Transport{
		TLSClientConfig:       tlsConfig,
		MaxIdleConns:          10,
		IdleConnTimeout:       120 * time.Second,
		TLSHandshakeTimeout:   5 * time.Second,
		ResponseHeaderTimeout: 40 * time.Second,
	}
	client := &http.Client{
		Transport: transport,
	}

	return client, nil
}

func getServerName(clientName string) (string, error) {
	if e := os.Getenv("DEVEL_API_SERVER"); len(e) > 0 {
		return e, nil
	}

	depEnv, err := depenv.GetDeploymentEnvironmentFromName(clientName)
	if err != nil {
		return "", err
	}

	return depEnv.MonitorAPIHost(), nil
}

func NewHeaderInterceptor() connect.UnaryInterceptorFunc {
	interceptor := func(next connect.UnaryFunc) connect.UnaryFunc {
		return connect.UnaryFunc(func(
			ctx context.Context,
			req connect.AnyRequest,
		) (connect.AnyResponse, error) {
			if req.Spec().IsClient {
				req.Header().Set("User-Agent", "ntpmon/"+version.Version())
			}
			return next(ctx, req)
		})
	}
	return connect.UnaryInterceptorFunc(interceptor)
}

func Client(ctx context.Context, clientName string, cp apitls.CertificateProvider) (context.Context, apiv2connect.MonitorServiceClient, error) {
	log := logger.FromContext(ctx)

	log.DebugContext(ctx, "setting up api client", "name", clientName)

	serverName, err := getServerName(clientName)
	if err != nil {
		return ctx, nil, err
	}

	httpClient, err := httpClient(cp)
	if err != nil {
		return ctx, nil, err
	}

	otelinter, err := otelconnect.NewInterceptor()
	if err != nil {
		return nil, nil, err
	}

	client := apiv2connect.NewMonitorServiceClient(
		httpClient,
		serverName,
		connect.WithInterceptors(otelinter),
		connect.WithInterceptors(NewHeaderInterceptor()),
	)

	return ctx, client, nil
}
