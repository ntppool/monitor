package api

import (
	"context"
	"crypto/tls"
	"fmt"
	"log"
	"net/http"
	"strings"
	"time"

	"github.com/twitchtv/twirp"
	"go.ntppool.org/monitor/api/pb"
	apitls "go.ntppool.org/monitor/api/tls"
	"go.ntppool.org/monitor/version"
)

var apiServers = map[string]string{
	"devel": "https://api.devel.mon.ntppool.dev",
	"test":  "https://api.test.mon.ntppool.dev",
	"prod":  "https://api.mon.ntppool.dev",
}

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
		TLSClientConfig: tlsConfig,
	}
	client := &http.Client{
		Transport: transport,
		Timeout:   15 * time.Second,
	}

	return client, nil
}

func getServerName(clientName string) (string, error) {

	clientName = strings.ToLower(clientName)

	if !strings.HasSuffix(clientName, ".mon.ntppool.dev") {
		return "", fmt.Errorf("invalid client name %s", clientName)
	}

	prefix := clientName[:strings.Index(clientName, ".mon.ntppool.dev")]
	parts := strings.Split(prefix, ".")
	if len(parts) != 2 {
		return "", fmt.Errorf("invalid client name %s", clientName)
	}

	if apiServer, ok := apiServers[parts[1]]; ok {
		return apiServer, nil
	}

	return "", fmt.Errorf("invalid client name %s (unknown environment %s)", clientName, parts[1])

}

func Client(ctx context.Context, clientName string, cp apitls.CertificateProvider) (pb.Monitor, error) {

	serverName, err := getServerName(clientName)
	if err != nil {
		return nil, err
	}

	httpClient, err := httpClient(cp)
	if err != nil {
		return nil, err
	}
	client := pb.NewMonitorProtobufClient(
		serverName,
		httpClient,
		twirp.WithClientPathPrefix("/api/v1"),
	)

	hdr := make(http.Header)
	hdr.Set("User-Agent", "ntppool-monitor/"+version.Version())
	ctx, err = twirp.WithHTTPRequestHeaders(ctx, hdr)
	if err != nil {
		log.Printf("twirp error setting headers: %s", err)
		return nil, err
	}

	return client, nil
}
