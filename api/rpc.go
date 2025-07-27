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
	"go.ntppool.org/monitor/client/httpclient"
	apiv2connect "go.ntppool.org/monitor/gen/monitor/v2/monitorv2connect"
)

func httpClient(cm apitls.AuthProvider) (*http.Client, error) {
	capool, err := apitls.CAPool()
	if err != nil {
		return nil, err
	}

	tlsConfig := &tls.Config{
		InsecureSkipVerify:   false,
		GetClientCertificate: cm.GetClientCertificate,
		RootCAs:              capool,
		// Add debug logging for certificate issues
		VerifyConnection: func(cs tls.ConnectionState) error {
			log := logger.Setup()
			for _, cert := range cs.PeerCertificates {
				log.Debug("server certificate verification",
					"subject", cert.Subject.String(),
					"issuer", cert.Issuer.String(),
					"notBefore", cert.NotBefore,
					"notAfter", cert.NotAfter,
					"expired", time.Now().After(cert.NotAfter),
					"serverName", cs.ServerName,
				)
			}
			return nil
		},
	}

	// tlsConfig.BuildNameToCertificate()
	transport := &http.Transport{
		TLSClientConfig:       tlsConfig,
		MaxIdleConns:          10,
		IdleConnTimeout:       120 * time.Second,
		TLSHandshakeTimeout:   10 * time.Second,
		ResponseHeaderTimeout: 10 * time.Second,
	}

	// Wrap transport with pool flusher to handle certificate errors
	poolFlusherTransport := httpclient.NewPoolFlusherTransport(transport)

	client := &http.Client{
		Transport: poolFlusherTransport,
		Timeout:   30 * time.Second,
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

func NewHeaderInterceptor(ap apitls.AuthProvider) connect.UnaryInterceptorFunc {
	interceptor := func(next connect.UnaryFunc) connect.UnaryFunc {
		return connect.UnaryFunc(func(
			ctx context.Context,
			req connect.AnyRequest,
		) (connect.AnyResponse, error) {
			if req.Spec().IsClient {
				req.Header().Set("User-Agent", "ntppool-agent/"+version.Version())

				// Prefer JWT token over API key for authentication
				if jwtToken, err := ap.GetJWTToken(ctx); err == nil && jwtToken != "" {
					req.Header().Set("Authorization", "Bearer "+jwtToken)
				} else if apiKey := ap.GetAPIKey(); apiKey != "" {
					// Fallback to API key during migration period
					req.Header().Set("Authorization", "Bearer "+apiKey)
				}
			}
			return next(ctx, req)
		})
	}
	return connect.UnaryInterceptorFunc(interceptor)
}

func Client(ctx context.Context, clientName string, cp apitls.AuthProvider) (context.Context, apiv2connect.MonitorServiceClient, error) {
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
		connect.WithInterceptors(NewHeaderInterceptor(cp)),
	)

	return ctx, client, nil
}
