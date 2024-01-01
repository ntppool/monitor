package api

import (
	"context"
	"crypto/tls"
	"fmt"
	"net/http"
	"os"
	"strings"
	"time"

	"connectrpc.com/connect"
	"connectrpc.com/otelconnect"

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

func GetDeploymentEnvironmentFromName(clientName string) (DeploymentEnvironment, error) {

	clientName = strings.ToLower(clientName)

	if !strings.HasSuffix(clientName, ".mon.ntppool.dev") {
		return DeployUndefined, fmt.Errorf("invalid client name %s", clientName)
	}

	if clientName == "api.mon.ntppool.dev" {
		return DeployProd, nil
	}

	prefix := clientName[:strings.Index(clientName, ".mon.ntppool.dev")]
	parts := strings.Split(prefix, ".")
	if len(parts) != 2 {
		return DeployUndefined, fmt.Errorf("invalid client name %s", clientName)
	}

	if d, err := DeploymentEnvironmentFromString(parts[1]); err == nil {
		return d, nil
	}

	return DeployUndefined, fmt.Errorf("invalid client name %s (unknown environment %s)", clientName, parts[1])

}

func getServerName(clientName string) (string, error) {

	if e := os.Getenv("DEVEL_API_SERVER"); len(e) > 0 {
		return e, nil
	}

	depEnv, err := GetDeploymentEnvironmentFromName(clientName)
	if err != nil {
		return "", err
	}

	return apiServers[depEnv], nil
}

func NewHeaderInterceptor() connect.UnaryInterceptorFunc {
	interceptor := func(next connect.UnaryFunc) connect.UnaryFunc {
		return connect.UnaryFunc(func(
			ctx context.Context,
			req connect.AnyRequest,
		) (connect.AnyResponse, error) {
			if req.Spec().IsClient {
				req.Header().Set("User-Agent", "ntppool-monitor/"+version.Version())
			}
			return next(ctx, req)
		})
	}
	return connect.UnaryInterceptorFunc(interceptor)
}

func Client(ctx context.Context, clientName string, cp apitls.CertificateProvider) (context.Context, apiv2connect.MonitorServiceClient, error) {

	log := logger.FromContext(ctx)

	log.Debug("setting up api client")

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
