package api

import (
	"context"
	"crypto/tls"
	"log"
	"net/http"
	"time"

	"github.com/twitchtv/twirp"
	"go.ntppool.org/monitor/api/pb"
	apitls "go.ntppool.org/monitor/api/tls"
)

// VERSION is the current version of the software
var VERSION = "3.0-dev"

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

func Client(ctx context.Context, cp apitls.CertificateProvider) (pb.Monitor, error) {

	httpClient, err := httpClient(cp)
	if err != nil {
		return nil, err
	}
	client := pb.NewMonitorProtobufClient(
//		"https://monitor-api-dev.ntppool.net:8000",
		"https://api.devel.mon.ntppool.dev",
		httpClient,
		twirp.WithClientPathPrefix("/api/v1"),
	)

	hdr := make(http.Header)
	hdr.Set("User-Agent", "ntppool-monitor/"+VERSION)
	ctx, err = twirp.WithHTTPRequestHeaders(ctx, hdr)
	if err != nil {
		log.Printf("twirp error setting headers: %s", err)
		return nil, err
	}

	return client, nil
}
