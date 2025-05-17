package config

import (
	"context"
	"crypto/sha256"
	"crypto/tls"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"net"
	"net/http"
	"net/netip"
	"os"
	"strings"
	"sync"
	"time"

	"go.ntppool.org/common/config/depenv"
	"go.ntppool.org/common/logger"
	"go.ntppool.org/common/tracing"
	apitls "go.ntppool.org/monitor/api/tls"
	"go.ntppool.org/monitor/ntpdb"
)

var ErrAuthorization = errors.New("api key unauthorized")

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

	HaveCertificate() bool
	CertificateDates() (notBefore time.Time, notAfter time.Time, remaining time.Duration, err error)

	WaitUntilConfigured(ctx context.Context) error
	WaitUntilLive(ctx context.Context) error
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
	DataSha string

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
		if errors.Is(err, ErrAuthorization) {
			ac.API.APIKey = ""
			return ac, nil
		}
		log.InfoContext(ctx, "load failed", "err", err)
		return nil, err
	}

	return ac, nil
}

func (ac *appConfig) WaitUntilLive(ctx context.Context) error {
	ctx, span := tracing.Start(ctx, "monitor.WaitUntilLive")
	defer span.End()

	err := ac.WaitUntilConfigured(ctx)
	if err != nil {
		return err
	}

	log := logger.FromContext(ctx)

	for i := 0; true; i++ {

		if ac.Data.IPv4.IsLive() || ac.Data.IPv6.IsLive() {
			break
		}

		if i == 0 {
			log.InfoContext(ctx, "waiting for monitor status to be testing or active")
		}

		select {
		case <-ctx.Done():
			return nil
		case <-time.After(60 * time.Second):
		}

		// do this last in the loop because WaitUntilConfigured just
		// loaded the configuration anyway
		err := ac.load(ctx)
		if err != nil {
			log.InfoContext(ctx, "load failed", "err", err)
			return err
		}
	}

	return nil
}

func (ac *appConfig) WaitUntilConfigured(ctx context.Context) error {
	ctx, span := tracing.Start(ctx, "monitor.WaitUntilConfigured")
	defer span.End()
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
			cmdName := fmt.Sprintf(
				"ntppool-agent setup --env %s --state-dir '%s'",
				ac.e.String(),
				ac.dir,
			)
			log.WarnContext(ctx, "no API key, please run ntppool-agent setup", "cmd", cmdName)
			if i == 1 {
				// Check if stdin is a terminal
				fileInfo, err := os.Stdin.Stat()
				if err == nil && (fileInfo.Mode()&os.ModeCharDevice) != 0 {
					fmt.Printf("\nSetup API key with:\n\n    %s\n\n", cmdName)
				}
			}
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

	valid, nextCheck, err := ac.checkCertificateValidity(ctx)
	if err != nil {
		renewCert = true
		if !errors.Is(err, apitls.ErrNoCertificate) {
			log.WarnContext(ctx, "check certificate validity", "err", err)
		}
	}

	// log.DebugContext(ctx, "got cert validity", "valid", valid, "nextCheck", nextCheck)

	if !valid && !renewCert {
		log.InfoContext(ctx, "certificate expiring, request renewal", "nextCheck", nextCheck)
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
	traceID := resp.Header.Get("Traceid")
	if resp.StatusCode == http.StatusUnauthorized {
		log.InfoContext(ctx, "unauthorized, please run setup", "trace", traceID)
		return ErrAuthorization
	} else if resp.StatusCode < 200 || resp.StatusCode > 299 {
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

	js, err := json.Marshal(ac.Data)
	if err != nil {
		log.WarnContext(ctx, "error marshalling config", "err", err)
	} else {
		sum := sha256.Sum256(js)
		sha := hex.EncodeToString(sum[:])
		if sha != ac.DataSha {
			fields := []any{
				"name", ac.Data.Name,
				"tls_name", ac.Data.TLSName,
			}
			if ac.Data.IPv4.IP != nil {
				fields = append(fields,
					"ipv4.ip", ac.Data.IPv4.IP.String(),
					"ipv4.status", ac.Data.IPv4.Status,
				)
			}

			if ac.Data.IPv6.IP != nil {
				fields = append(fields,
					"ipv6.ip", ac.Data.IPv6.IP.String(),
					"ipv6.status", ac.Data.IPv6.Status,
				)
			}

			log.InfoContext(ctx, "config changed", fields...)
			ac.DataSha = sha
		}
	}

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

func (ipconfig IPConfig) IsLive() bool {
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
