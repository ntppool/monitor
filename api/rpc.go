package api

import (
	"context"
	"crypto/tls"
	"fmt"
	"net/http"
	"os"
	"strings"
	"time"

	"github.com/twitchtv/twirp"

	"go.ntppool.org/common/logger"
	"go.ntppool.org/common/version"
	"go.ntppool.org/monitor/api/pb"
	apitls "go.ntppool.org/monitor/api/tls"
	"go.ntppool.org/monitor/server/twirptrace"
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
		IdleConnTimeout:       90 * time.Second,
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

func Client(ctx context.Context, clientName string, cp apitls.CertificateProvider) (context.Context, pb.Monitor, error) {

	log := logger.FromContext(ctx)

	serverName, err := getServerName(clientName)
	if err != nil {
		return ctx, nil, err
	}

	httpClient, err := httpClient(cp)
	if err != nil {
		return ctx, nil, err
	}

	client := pb.NewMonitorProtobufClient(
		serverName,
		twirptrace.NewTraceHTTPClient(httpClient),
		twirp.WithClientPathPrefix("/api/v1"),
	)

	hdr := make(http.Header)
	hdr.Set("User-Agent", "ntppool-monitor/"+version.Version())
	ctx, err = twirp.WithHTTPRequestHeaders(ctx, hdr)
	if err != nil {
		log.Error("twirp error setting headers", "err", err)
		return ctx, nil, err
	}

	return ctx, client, nil
}
