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

	"github.com/cenkalti/backoff/v5"
	"go.ntppool.org/common/config/depenv"
	"go.ntppool.org/common/logger"
	"go.ntppool.org/common/tracing"
	apitls "go.ntppool.org/monitor/api/tls"
	"go.ntppool.org/monitor/ntpdb"
)

// newConfigBackoff creates an exponential backoff with the specified intervals
func newConfigBackoff(initial, max time.Duration) *backoff.ExponentialBackOff {
	b := backoff.NewExponentialBackOff()
	b.InitialInterval = initial
	b.MaxInterval = max
	b.RandomizationFactor = 0.1 // Add jitter to prevent thundering herd
	return b
}

// ErrAuthorization is returned when the API key is unauthorized or invalid.
var ErrAuthorization = errors.New("api key unauthorized")

// AppConfig is the configured runtime information that's stored
// on disk and used to find and authenticate with the API.
type AppConfig interface {
	TLSName() string
	ServerName() string

	Env() depenv.DeploymentEnvironment

	APIKey() string    // NTP Pool API key
	GetAPIKey() string // Alias for APIKey for AuthProvider interface
	SetAPIKey(apiKey string) error

	// does this API make sense?
	IPv4() IPConfig
	IPv6() IPConfig

	// Certificate providers
	GetClientCertificate(certRequestInfo *tls.CertificateRequestInfo) (*tls.Certificate, error)
	GetCertificate(hello *tls.ClientHelloInfo) (*tls.Certificate, error)

	// JWT token provider for API authentication (replaces mTLS)
	GetJWTToken(ctx context.Context) (string, error)

	HaveCertificate() bool
	CertificateDates() (notBefore time.Time, notAfter time.Time, remaining time.Duration, err error)

	// Certificate renewal methods
	LoadAPIAppConfig(ctx context.Context) (bool, error)
	LoadAPIAppConfigWithCertificateRequest(ctx context.Context) (bool, error)
	CheckCertificateValidity(ctx context.Context) (valid bool, nextCheck time.Duration, err error)

	WaitUntilConfigured(ctx context.Context) error
	WaitUntilLive(ctx context.Context) error
	WaitUntilCertificatesLoaded(ctx context.Context) error

	// Hot reloading from disk and API
	load(ctx context.Context) error

	// AppConfig manager for hot reloading
	Manager(ctx context.Context) error

	// Configuration change notifications
	WaitForConfigChange(ctx context.Context) *ConfigChangeWaiter
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

	// JWT token management
	jwtMutex  sync.RWMutex
	jwtToken  string
	jwtExpiry time.Time

	// For configuration change notifications
	configChangeMu      sync.RWMutex
	configChangeWaiters map[uint64]configChangeWaiter
	configChangeNextID  uint64
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

	log.DebugContext(ctx, "creating new AppConfig",
		"env", deployEnv,
		"stateDir", stateDir,
		"wait", wait,
		"MONITOR_STATE_DIR", os.Getenv("MONITOR_STATE_DIR"),
		"STATE_DIRECTORY", os.Getenv("STATE_DIRECTORY"),
		"RUNTIME_DIRECTORY", os.Getenv("RUNTIME_DIRECTORY"))

	ac := &appConfig{
		e:   deployEnv,
		dir: stateDir,
	}

	err := ac.load(ctx)
	if err != nil {
		if errors.Is(err, ErrAuthorization) {
			log.InfoContext(ctx, "API key unauthorized, clearing it")
			ac.lock.Lock()
			ac.API.APIKey = ""
			ac.lock.Unlock()
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

	// Check if already live
	ac.lock.RLock()
	isLive := ac.Data.IPv4.IsLive() || ac.Data.IPv6.IsLive()
	ac.lock.RUnlock()

	if isLive {
		return nil
	}

	// Backoff for waiting for monitor activation
	activationBackoff := newConfigBackoff(1*time.Minute, 20*time.Minute)

	log.InfoContext(ctx, "waiting for monitor status to be testing or active")

	for {
		// Wait for config change notification or backoff timeout
		waiter := ac.WaitForConfigChange(ctx)
		waitTime := activationBackoff.NextBackOff()
		if waitTime == backoff.Stop {
			waitTime = activationBackoff.MaxInterval
		}

		timer := time.NewTimer(waitTime)

		select {
		case <-ctx.Done():
			timer.Stop()
			waiter.Cancel()
			log.InfoContext(ctx, "WaitUntilLive context done, exiting")
			return nil
		case <-waiter.Done():
			// Config changed, check immediately
			log.DebugContext(ctx, "config change detected, checking live status")
			timer.Stop()
			waiter.Cancel()
			// Reset backoff on config change
			activationBackoff.Reset()
		case <-timer.C:
			// Backoff timeout - reload config manually as fallback
			log.DebugContext(ctx, "activation check timeout, reloading config",
				"wait_time", waitTime.Round(time.Minute).String())
			waiter.Cancel()
			err := ac.load(ctx)
			if err != nil {
				if errors.Is(err, ErrAuthorization) {
					// Authorization error - let waitUntilAPIKey handle it
					log.WarnContext(ctx, "authorization error during live check, will retry", "err", err)
					continue
				}
				log.InfoContext(ctx, "load failed", "err", err)
				return err
			}
		}

		// Check if any IP is live now
		ac.lock.RLock()
		isLive := ac.Data.IPv4.IsLive() || ac.Data.IPv6.IsLive()
		ac.lock.RUnlock()

		if isLive {
			log.InfoContext(ctx, "monitor is now live")
			return nil
		}
	}
}

func (ac *appConfig) waitUntilAPIKey(ctx context.Context) error {
	ctx, span := tracing.Start(ctx, "monitor.WaitUntilAPIKey")
	defer span.End()
	log := logger.FromContext(ctx)

	ac.lock.RLock()
	hasKey := ac.API.APIKey != ""
	ac.lock.RUnlock()

	if hasKey {
		return nil
	}

	// Backoff for authorization errors (expired/revoked keys)
	authErrorBackoff := newConfigBackoff(10*time.Minute, 4*time.Hour)

	// Backoff for missing API keys (normal case)
	missingKeyBackoff := newConfigBackoff(30*time.Second, 5*time.Minute)

	var currentBackoff backoff.BackOff = missingKeyBackoff
	var lastAuthError time.Time

	i := 0
	for {
		i++

		// Try to load from disk/API
		err := ac.load(ctx)
		if err != nil {
			if errors.Is(err, ErrAuthorization) {
				// API key is unauthorized - could be expired or temporary server issue
				currentBackoff = authErrorBackoff
				lastAuthError = time.Now()
				// Don't return error - continue waiting
			} else {
				// Other errors should be propagated
				log.InfoContext(ctx, "load failed", "err", err)
				return err
			}
		} else {
			// Successful load - check if we have an API key now
			ac.lock.RLock()
			hasKey := ac.API.APIKey != ""
			ac.lock.RUnlock()

			if hasKey {
				// Reset backoff on success
				authErrorBackoff.Reset()
				missingKeyBackoff.Reset()
				return nil
			}

			// No API key after successful load - back to missing key backoff
			if time.Since(lastAuthError) > time.Hour {
				currentBackoff = missingKeyBackoff
			}
		}

		// Determine wait time and log appropriate message
		waitTime := currentBackoff.NextBackOff()
		if waitTime == backoff.Stop {
			waitTime = currentBackoff.(*backoff.ExponentialBackOff).MaxInterval
		}

		if currentBackoff == authErrorBackoff {
			log.DebugContext(ctx, "API key unauthorized, waiting before retry",
				"wait_time", waitTime.Round(time.Minute).String(),
				"next_retry", time.Now().Add(waitTime).Format("15:04:05"))
		} else {
			if i == 1 || waitTime >= 2*time.Minute {
				cmdName := fmt.Sprintf(
					"ntppool-agent setup --env %s --state-dir '%s'",
					ac.e.String(),
					ac.dir,
				)
				log.WarnContext(ctx, "no API key, please run ntppool-agent setup",
					"cmd", cmdName,
					"wait_time", waitTime.Round(time.Second).String())

				if i == 1 {
					// Check if stdin is a terminal
					fileInfo, err := os.Stdin.Stat()
					if err == nil && (fileInfo.Mode()&os.ModeCharDevice) != 0 {
						fmt.Printf("\nSetup API key with:\n\n    %s\n\n", cmdName)
					}
				}
			}
		}

		// Wait for config change notification or timeout
		waiter := ac.WaitForConfigChange(ctx)
		timer := time.NewTimer(waitTime)

		select {
		case <-ctx.Done():
			timer.Stop()
			waiter.Cancel()
			return nil
		case <-waiter.Done():
			// Config changed, check immediately
			log.DebugContext(ctx, "config change detected, checking for API key")
			timer.Stop()
			waiter.Cancel()
			// Reset backoff on config change (new API key might have been added)
			if currentBackoff == missingKeyBackoff {
				missingKeyBackoff.Reset()
			} else {
				// For auth errors, also reset to try again immediately
				authErrorBackoff.Reset()
			}
		case <-timer.C:
			// Backoff timeout reached - will retry on next iteration
			waiter.Cancel()
		}
	}
}

func (ac *appConfig) WaitUntilConfigured(ctx context.Context) error {
	ctx, span := tracing.Start(ctx, "monitor.WaitUntilConfigured")
	defer span.End()

	// First wait for API key
	if err := ac.waitUntilAPIKey(ctx); err != nil {
		return err
	}

	// Then wait for certificates
	if err := ac.WaitUntilCertificatesLoaded(ctx); err != nil {
		return err
	}

	// Finally wait for JWT token
	return ac.waitForJWTToken(ctx)
}

func (ac *appConfig) WaitUntilCertificatesLoaded(ctx context.Context) error {
	ctx, span := tracing.Start(ctx, "monitor.WaitUntilCertificatesLoaded")
	defer span.End()
	log := logger.FromContext(ctx)

	// Check if we already have valid certificates
	if ac.HaveCertificate() {
		valid, _, err := ac.checkCertificateValidity(ctx)
		if err == nil && valid {
			log.DebugContext(ctx, "certificates are already loaded and valid")
			return nil
		}
	}

	// Backoff for certificate loading failures
	certBackoff := newConfigBackoff(4*time.Second, 2*time.Minute)

	log.InfoContext(ctx, "waiting for certificates to be loaded")

	for {
		// Try to load certificates from API if we don't have them
		err := ac.load(ctx)
		if err != nil {
			if errors.Is(err, ErrAuthorization) {
				// Authorization error - certificate loading will fail
				log.WarnContext(ctx, "authorization error while loading certificates", "err", err)
				return err // Propagate auth errors up to waitUntilAPIKey
			}
			log.InfoContext(ctx, "load failed while waiting for certificates", "err", err)
			// Continue waiting for other errors
		}

		// Check certificate status
		if ac.HaveCertificate() {
			// Verify certificate is valid and not expired
			valid, _, err := ac.checkCertificateValidity(ctx)
			if err == nil && valid {
				log.DebugContext(ctx, "certificates are loaded and valid")
				return nil
			}
			if err != nil {
				log.DebugContext(ctx, "certificate validity check failed, will retry", "err", err)
			} else {
				log.DebugContext(ctx, "certificate is loaded but not valid, will retry")
			}
		}

		// Determine wait time
		waitTime := certBackoff.NextBackOff()
		if waitTime == backoff.Stop {
			waitTime = certBackoff.MaxInterval
		}

		// Log periodically when waiting longer
		if waitTime >= 30*time.Second {
			log.InfoContext(ctx, "still waiting for certificates to be loaded",
				"wait_time", waitTime.Round(time.Second).String())
		}

		select {
		case <-ctx.Done():
			log.DebugContext(ctx, "WaitUntilCertificatesLoaded context done, exiting")
			return ctx.Err()
		case <-time.After(waitTime):
			// Continue to next iteration
		}
	}
}

func (ac *appConfig) LoadAPIAppConfig(ctx context.Context) (bool, error) {
	return ac.loadAPIAppConfig(ctx, false)
}

func (ac *appConfig) LoadAPIAppConfigWithCertificateRequest(ctx context.Context) (bool, error) {
	return ac.loadAPIAppConfig(ctx, true)
}

func (ac *appConfig) loadAPIAppConfig(ctx context.Context, renewCert bool) (bool, error) {
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
		return false, err
	}
	req.Header.Add("Authorization", "Bearer "+ac.APIKey())

	resp, err := apiHTTPClient.Do(req)
	if err != nil {
		return false, err
	}
	defer func() {
		if err := resp.Body.Close(); err != nil {
			log.WarnContext(ctx, "Failed to close response body", "err", err)
		}
	}()
	traceID := resp.Header.Get("Traceid")
	if resp.StatusCode == http.StatusUnauthorized {
		log.WarnContext(ctx, "API authorization failed", "trace", traceID)
		// Clear JWT token on authorization error as it might be invalid
		ac.clearJWTToken()
		return false, ErrAuthorization
	} else if resp.StatusCode < 200 || resp.StatusCode > 299 {
		return false, fmt.Errorf("unexpected response code: %d (trace %s)", resp.StatusCode, traceID)
	}

	monStatus := MonitorStatusConfig{}
	dec := json.NewDecoder(resp.Body)
	err = dec.Decode(&monStatus)
	if err != nil {
		return false, err
	}

	// spew.Dump(monStatus)
	if len(monStatus.TLS.Cert) > 0 {
		err = ac.SaveCertificates(ctx, []byte(monStatus.TLS.Cert), []byte(monStatus.TLS.Key))
		if err != nil {
			return false, fmt.Errorf("error saving certificates: %w", err)
		}
	}

	// Build new data configuration before acquiring lock
	var newIPv4, newIPv6 IPConfig
	var newName, newTLSName string

	for i, ipInput := range []MonitorStatus{monStatus.IPv4, monStatus.IPv6} {
		ipVersion := "IPv4"
		if i == 1 {
			ipVersion = "IPv6"
		}

		if ipInput.IP == "" {
			// Leave as zero value
			continue
		}

		ip, err := netip.ParseAddr(ipInput.IP)
		if err != nil {
			return false, fmt.Errorf("error parsing %s address %q: %w", ipVersion, monStatus.IPv4.IP, err)
		}
		if !ip.IsValid() {
			return false, fmt.Errorf("invalid %s address %q", ipVersion, monStatus.IPv4.IP)
		}

		switch ipVersion {
		case "IPv4":
			if !ip.Is4() {
				return false, fmt.Errorf("expected IPv4 address, got %s", ip.String())
			}
			newIPv4 = IPConfig{
				Status: ntpdb.MonitorsStatus(ipInput.Status),
				IP:     &ip,
			}
		case "IPv6":
			if !ip.Is6() {
				return false, fmt.Errorf("expected IPv6 address, got %s", ip.String())
			}
			newIPv6 = IPConfig{
				Status: ntpdb.MonitorsStatus(ipInput.Status),
				IP:     &ip,
			}
		}
	}

	newName = monStatus.Name
	newTLSName = monStatus.TLSName

	// Update fields and check for changes under lock
	ac.lock.Lock()
	ac.Data.IPv4 = newIPv4
	ac.Data.IPv6 = newIPv6
	ac.Data.Name = newName
	ac.Data.TLSName = newTLSName

	// Marshal while holding lock to ensure consistency
	js, err := json.Marshal(ac.Data)
	if err != nil {
		ac.lock.Unlock()
		log.WarnContext(ctx, "error marshalling config", "err", err)
		return false, nil
	}

	sum := sha256.Sum256(js)
	sha := hex.EncodeToString(sum[:])
	dataChanged := sha != ac.DataSha

	// Copy data for logging outside the lock
	var logData struct {
		Name    string
		TLSName string
		IPv4    IPConfig
		IPv6    IPConfig
	}
	if dataChanged {
		ac.DataSha = sha
		logData.Name = ac.Data.Name
		logData.TLSName = ac.Data.TLSName
		logData.IPv4 = ac.Data.IPv4
		logData.IPv6 = ac.Data.IPv6
	}
	ac.lock.Unlock()

	// Log and notify outside the lock
	if dataChanged {
		fields := []any{
			"name", logData.Name,
			"tls_name", logData.TLSName,
		}
		if logData.IPv4.IP != nil {
			fields = append(fields,
				"ipv4.ip", logData.IPv4.IP.String(),
				"ipv4.status", logData.IPv4.Status,
			)
		}
		if logData.IPv6.IP != nil {
			fields = append(fields,
				"ipv6.ip", logData.IPv6.IP.String(),
				"ipv6.status", logData.IPv6.Status,
			)
		}
		log.InfoContext(ctx, "config changed", fields...)
		ac.notifyConfigChange()
	}

	return dataChanged, nil
}

func (ac *appConfig) Env() depenv.DeploymentEnvironment {
	return ac.e
}

func (ac *appConfig) TLSName() string {
	ac.lock.RLock()
	defer ac.lock.RUnlock()
	return ac.Data.TLSName
}

func (ac *appConfig) ServerName() string {
	ac.lock.RLock()
	defer ac.lock.RUnlock()
	return ac.Data.Name
}

func (ac *appConfig) SetAPIKey(apiKey string) error {
	// Check if API key is actually changing
	ac.lock.Lock()
	prevAPIKey := ac.API.APIKey
	ac.API.APIKey = apiKey
	ac.lock.Unlock()

	err := ac.save()
	if err != nil {
		return err
	}

	// Immediately notify waiters if API key changed
	if prevAPIKey != apiKey {
		ac.notifyConfigChange()
	}

	return nil
}

func (ac *appConfig) APIKey() string {
	ac.lock.RLock()
	defer ac.lock.RUnlock()
	return ac.API.APIKey
}

func (ac *appConfig) IPv4() IPConfig {
	ac.lock.RLock()
	defer ac.lock.RUnlock()
	return ac.Data.IPv4
}

func (ac *appConfig) IPv6() IPConfig {
	ac.lock.RLock()
	defer ac.lock.RUnlock()
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

// configChangeWaiter represents a single waiter for config changes
type configChangeWaiter struct {
	id     uint64
	cancel context.CancelFunc
}

// ConfigChangeWaiter holds a context and its cleanup function
type ConfigChangeWaiter struct {
	ctx context.Context
	id  uint64
	ac  *appConfig
}

// Done returns the context's Done channel
func (w *ConfigChangeWaiter) Done() <-chan struct{} {
	return w.ctx.Done()
}

// Cancel cleans up this waiter and removes it from the notification list
func (w *ConfigChangeWaiter) Cancel() {
	w.ac.removeConfigChangeWaiter(w.id)
}

// WaitForConfigChange returns a waiter that will be cancelled when configuration changes
func (ac *appConfig) WaitForConfigChange(ctx context.Context) *ConfigChangeWaiter {
	ac.configChangeMu.Lock()
	defer ac.configChangeMu.Unlock()

	childCtx, cancel := context.WithCancel(ctx)

	// Generate unique ID for this waiter
	ac.configChangeNextID++
	id := ac.configChangeNextID

	waiter := configChangeWaiter{
		id:     id,
		cancel: cancel,
	}

	if ac.configChangeWaiters == nil {
		ac.configChangeWaiters = make(map[uint64]configChangeWaiter)
	}
	ac.configChangeWaiters[id] = waiter

	return &ConfigChangeWaiter{
		ctx: childCtx,
		id:  id,
		ac:  ac,
	}
}

// removeConfigChangeWaiter removes a specific waiter by ID
func (ac *appConfig) removeConfigChangeWaiter(id uint64) {
	ac.configChangeMu.Lock()
	defer ac.configChangeMu.Unlock()

	if waiter, exists := ac.configChangeWaiters[id]; exists {
		waiter.cancel()
		delete(ac.configChangeWaiters, id)
	}
}

// notifyConfigChange cancels all waiting contexts and clears the map
func (ac *appConfig) notifyConfigChange() {
	ac.configChangeMu.Lock()
	defer ac.configChangeMu.Unlock()

	for _, waiter := range ac.configChangeWaiters {
		waiter.cancel()
	}
	ac.configChangeWaiters = make(map[uint64]configChangeWaiter) // Clear the map
}
