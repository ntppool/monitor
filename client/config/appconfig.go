package config

import (
	"context"
	"crypto/tls"
	"encoding/json"
	"fmt"
	"net"
	"net/http"
	"net/netip"
	"strings"
	"sync"
	"time"

	"go.ntppool.org/common/config/depenv"
	"go.ntppool.org/common/logger"
	"go.ntppool.org/common/tracing"
	"go.ntppool.org/monitor/ntpdb"
)

// AppConfig is the configured runtime information that's stored
// on disk and used to find and authenticate with the API.
type AppConfig interface {
	TLSName() string
	ServerName() string

	Env() depenv.DeploymentEnvironment

	APIKey() string // NTP Pool API key
	SetAPIKey(apiKey string) error

	// does this API make sense?
	IPv4() IPConfig
	IPv6() IPConfig

	// Certificate providers
	GetClientCertificate(certRequestInfo *tls.CertificateRequestInfo) (*tls.Certificate, error)
	GetCertificate(hello *tls.ClientHelloInfo) (*tls.Certificate, error)
	CertificateDates() (notBefore time.Time, notAfter time.Time, remaining time.Duration, err error)

	WaitUntilReady(ctx context.Context) error
}

type IPConfig struct {
	Status ntpdb.MonitorsStatus
	IP     *netip.Addr
}

type appConfig struct {
	e    depenv.DeploymentEnvironment
	dir  string // stateDir
	lock sync.RWMutex

	API struct {
		APIKey string
	}

	Data struct {
		Name    string
		TLSName string
		IPv4    IPConfig
		IPv6    IPConfig
	}

	tlsCert *tls.Certificate
}

var apiHTTPClient *http.Client

func init() {
	apiHTTPClient = &http.Client{
		// Overall timeout for the request, including connection, redirects, reading response body, etc.
		Timeout: 30 * time.Second,
		Transport: &http.Transport{
			// DialContext controls the timeout for establishing a TCP connection
			DialContext: (&net.Dialer{
				Timeout:   10 * time.Second,
				KeepAlive: 30 * time.Second,
			}).DialContext,
			// TLSHandshakeTimeout limits the time waiting to complete the TLS handshake
			TLSHandshakeTimeout: 10 * time.Second,
			// ResponseHeaderTimeout limits the time waiting for the response headers after a request is sent
			ResponseHeaderTimeout: 10 * time.Second,
			// ExpectContinueTimeout specifies the wait time for a server's first response headers after fully writing the request headers if the request has an "Expect: 100-continue" header.
			ExpectContinueTimeout: 5 * time.Second,
		},
	}
}

func NewAppConfig(ctx context.Context, deployEnv depenv.DeploymentEnvironment, stateDir string, wait bool) (AppConfig, error) {
	log := logger.FromContext(ctx)
	if deployEnv == depenv.DeployUndefined {
		return nil, fmt.Errorf("deployment environment invalid or undefined")
	}

	log.DebugContext(ctx, "loading config", "env", deployEnv, "stateDir", stateDir)

	ac := &appConfig{
		e:   deployEnv,
		dir: stateDir,
	}

	err := ac.load(ctx)
	if err != nil {
		log.InfoContext(ctx, "load failed", "err", err)
		return nil, err
	}

	return ac, nil
}

func (ac *appConfig) WaitUntilReady(ctx context.Context) error {
	log := logger.FromContext(ctx)

	if ac.API.APIKey != "" {
		return nil
	}

	i := 0
	for {
		i++

		err := ac.load(ctx)
		if err != nil {
			log.InfoContext(ctx, "load failed", "err", err)
			return err
		}

		if ac.API.APIKey != "" {
			break
		}

		if i == 1 || i%60 == 0 {
			log.WarnContext(ctx, "no API key, please run `ntpmon setup`")
		}

		select {
		case <-ctx.Done():
			return nil
		case <-time.After(3 * time.Second):
		}
	}
	return nil
}

func (ac *appConfig) LoadAPIAppConfig(ctx context.Context) error {
	return ac.loadAPIAppConfig(ctx, false)
}

func (ac *appConfig) LoadAPIAppConfigForceCerts(ctx context.Context) error {
	return ac.loadAPIAppConfig(ctx, true)
}

func (ac *appConfig) loadAPIAppConfig(ctx context.Context, renewCert bool) error {
	ctx, span := tracing.Start(ctx, "LoadAPIAppConfig")
	defer span.End()
	log := logger.FromContext(ctx)

	// this is separate from the monitoring API because the
	// monitoring API (currently...) requires TLS authentication
	// and we need to get the certs with our API key.
	baseURL := ac.e.APIHost()
	if !strings.HasSuffix(baseURL, "/") {
		baseURL += "/"
	}
	configURL := baseURL + "monitor/api/config"

	log.DebugContext(ctx, "loading config from api", "configURL", configURL)

	valid, remainingTime, err := ac.checkCertificateValidity(ctx)
	if err != nil {
		log.WarnContext(ctx, "check certificate validity", "err", err)
		renewCert = true
	}

	// log.DebugContext(ctx, "got cert validity", "valid", valid, "remainingTime", remainingTime)

	if !valid {
		log.InfoContext(ctx, "certificate expiring, request renewal", "remaining", remainingTime)
		renewCert = true
	}

	if renewCert {
		configURL += "?request_certificate=true"
	}

	// log.DebugContext(ctx, "sending request", "renewCert", renewCert)

	req, err := http.NewRequestWithContext(ctx, "GET", configURL, nil)
	if err != nil {
		return err
	}
	req.Header.Add("Authorization", "Bearer "+ac.APIKey())

	resp, err := apiHTTPClient.Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()
	if resp.StatusCode < 200 || resp.StatusCode > 299 {
		traceID := resp.Header.Get("Traceid")
		return fmt.Errorf("unexpected response code: %d (trace %s)", resp.StatusCode, traceID)
	}

	monStatus := MonitorStatusConfig{}
	dec := json.NewDecoder(resp.Body)
	err = dec.Decode(&monStatus)
	if err != nil {
		return err
	}

	// spew.Dump(monStatus)
	if len(monStatus.TLS.Cert) > 0 {
		err = ac.SaveCertificates([]byte(monStatus.TLS.Cert), []byte(monStatus.TLS.Key))
		if err != nil {
			return fmt.Errorf("error saving certificates: %w", err)
		}
	}

	for i, ipInput := range []MonitorStatus{monStatus.IPv4, monStatus.IPv6} {
		ipVersion := "IPv4"
		if i == 1 {
			ipVersion = "IPv6"
		}

		if ipInput.IP == "" {
			switch ipVersion {
			case "IPv4":
				ac.Data.IPv4 = IPConfig{}
			case "IPv6":
				ac.Data.IPv6 = IPConfig{}
			}
			continue
		}

		ip, err := netip.ParseAddr(ipInput.IP)
		if err != nil {
			return fmt.Errorf("error parsing %s address %q: %w", ipVersion, monStatus.IPv4.IP, err)
		}
		if !ip.IsValid() {
			return fmt.Errorf("invalid %s address %q", ipVersion, monStatus.IPv4.IP)
		}

		switch ipVersion {
		case "IPv4":
			if !ip.Is4() {
				return fmt.Errorf("expected IPv4 address, got %s", ip.String())
			}
		case "IPv6":
			if !ip.Is6() {
				return fmt.Errorf("expected IPv6 address, got %s", ip.String())
			}
		}

		ipConfig := IPConfig{
			Status: ntpdb.MonitorsStatus(ipInput.Status),
			IP:     &ip,
		}

		switch ipVersion {
		case "IPv4":
			ac.Data.IPv4 = ipConfig
		case "IPv6":
			ac.Data.IPv6 = ipConfig
		}

	}

	ac.Data.Name = monStatus.Name
	ac.Data.TLSName = monStatus.TLSName

	return nil
}

func (ac *appConfig) Env() depenv.DeploymentEnvironment {
	return ac.e
}

func (ac *appConfig) TLSName() string {
	return ac.Data.TLSName
}

func (ac *appConfig) ServerName() string {
	return ac.Data.Name
}

func (ac *appConfig) SetAPIKey(apiKey string) error {
	ac.API.APIKey = apiKey
	return ac.save()
}

func (ac *appConfig) APIKey() string {
	return ac.API.APIKey
}

func (ac *appConfig) IPv4() IPConfig {
	return ac.Data.IPv4
}

func (ac *appConfig) IPv6() IPConfig {
	return ac.Data.IPv6
}

func (ipconfig IPConfig) InUse() bool {
	if ipconfig.IP == nil || !ipconfig.IP.IsValid() {
		return false
	}
	switch ipconfig.Status {
	case ntpdb.MonitorsStatusActive:
		return true
	case ntpdb.MonitorsStatusTesting:
		return true
	default:
		return false
	}
}
