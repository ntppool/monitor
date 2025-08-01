package cmd

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"mime"
	"net/http"
	"net/http/httptrace"
	"net/netip"
	"net/url"
	"os"
	"strings"
	"time"

	"github.com/MakeNowJust/heredoc"

	"go.ntppool.org/common/logger"
	"go.ntppool.org/common/tracing"
	"go.ntppool.org/common/ulid"
	"go.ntppool.org/common/version"
	"go.ntppool.org/monitor/client/config"
	"go.ntppool.org/monitor/client/httpclient"
)

type setupCmd struct {
	Hostname string `name:"hostname" help:"Hostname to register (defaults to system hostname)"`
	Account  string `name:"account" short:"a" help:"Account identifier for registration"`
}

func (cmd *setupCmd) Run(ctx context.Context, cli *ClientCmd) error {
	log := logger.FromContext(ctx).With("env", cli.DeployEnv.String())
	ctx = logger.NewContext(ctx, log)
	ctx, span := tracing.Start(ctx, "monitor.setup")
	defer span.End()

	wantIPv4 := cli.IPv4
	wantIPv6 := cli.IPv6

	apiKey := cli.Config.APIKey()
	if apiKey != "" {
		// todo: require --reset to overwrite if one exists?
		_, _, err := cmd.checkCurrentConfig(ctx, cli)
		if err != nil {
			log.ErrorContext(ctx, "could not check config", "err", err)
		}
	}

	if !wantIPv4 && !wantIPv6 {
		log.InfoContext(ctx, "registration for both IP versions disabled")
		return nil
	}

	registrationID, err := ulid.MakeULID(time.Now())
	if err != nil {
		return fmt.Errorf("could not make registration ID: %w", err)
	}

	log.DebugContext(ctx, "registration ID", "id", registrationID)

	// Get hostname for registration
	hostname := cmd.Hostname
	if hostname == "" {
		hostname, err = os.Hostname()
		if err != nil {
			log.WarnContext(ctx, "could not get system hostname", "err", err)
			hostname = "" // Use empty hostname as fallback
		}
	}

	if hostname != "" {
		log.InfoContext(ctx, "using hostname for registration", "hostname", hostname)
	} else {
		log.InfoContext(ctx, "registering without hostname")
	}

	cl := httpclient.CreateIPVersionAwareClient()
	apiHost := cli.Config.Env().APIHost()

	// Prepare form data for hostname and account
	formData := url.Values{}
	if hostname != "" {
		formData.Set("hostname", hostname)
	}
	if cmd.Account != "" {
		formData.Set("a", cmd.Account)
	}

	var reqBody io.Reader
	if len(formData) > 0 {
		reqBody = strings.NewReader(formData.Encode())
	}

	req, err := http.NewRequestWithContext(ctx, "POST",
		fmt.Sprintf("%s/monitor/api/registration", apiHost),
		reqBody,
	)
	if err != nil {
		return err
	}

	req.Header.Set("User-Agent", "ntppool-agent/"+version.Version())
	req.Header.Set("Registration-ID", registrationID.String())
	req.Header.Set("Accept", "application/json, text/plain")

	if len(formData) > 0 {
		req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	}

	if apiKey != "" {
		req.Header.Add("Authorization", "Bearer "+apiKey)
	}

	rs := &registrationState{
		cl:       cl,
		req:      req,
		serverIP: netip.Addr{},
		hostname: hostname,
		account:  cmd.Account,
		tryIPv4:  wantIPv4,
		tryIPv6:  wantIPv6,
		wantIPv4: wantIPv4,
		wantIPv6: wantIPv6,
		cli:      cli,
	}

	trace := &httptrace.ClientTrace{
		GotConn: func(connInfo httptrace.GotConnInfo) {
			remoteAddr := connInfo.Conn.RemoteAddr().String()
			addrport, err := netip.ParseAddrPort(remoteAddr)
			if err != nil {
				log.WarnContext(ctx, "could not parse remote address", "addr", remoteAddr, "err", err)
				return
			}
			rs.serverIP = addrport.Addr()
		},
	}

	ctx = httptrace.WithClientTrace(ctx, trace)

	for {
		done, err := rs.registrationStep(ctx)
		if err != nil {
			return err
		}
		if done {
			break
		}
	}

	return nil
}

type registrationState struct {
	cl       *http.Client
	req      *http.Request
	serverIP netip.Addr
	cli      *ClientCmd
	hostname string
	account  string

	// wantIPv4 and wantIPv6 are the IP versions we want to
	// register for. tryIPv4 and tryIPv6 are the IP versions
	// we are trying to register for (disabled as the required
	// IP version has had a succesful request).
	tryIPv4  bool
	tryIPv6  bool
	wantIPv4 bool
	wantIPv6 bool

	// registrationURL stores the URL until both protocols complete
	registrationURL string
	// urlPrinted tracks whether we've already printed the registration URL
	urlPrinted bool
}

func (rs *registrationState) registrationStep(ctx context.Context) (done bool, err error) {
	log := logger.FromContext(ctx)

	switch {
	case rs.tryIPv4 && rs.tryIPv6:
		ctx = httpclient.NewIPVersionContext(ctx, httpclient.IPAny)
	case rs.tryIPv4:
		ctx = httpclient.NewIPVersionContext(ctx, httpclient.IPv4Only)
	case rs.tryIPv6:
		ctx = httpclient.NewIPVersionContext(ctx, httpclient.IPv6Only)
	default:
		switch {
		case rs.wantIPv4 && rs.wantIPv6:
			ctx = httpclient.NewIPVersionContext(ctx, httpclient.IPAny)
		case rs.wantIPv4:
			ctx = httpclient.NewIPVersionContext(ctx, httpclient.IPv4Only)
		case rs.wantIPv6:
			ctx = httpclient.NewIPVersionContext(ctx, httpclient.IPv6Only)
		}
	}

	// Update request with hostname and account form data for this iteration
	formData := url.Values{}
	if rs.hostname != "" {
		formData.Set("hostname", rs.hostname)
	}
	if rs.account != "" {
		formData.Set("a", rs.account)
	}
	if len(formData) > 0 {
		encodedData := formData.Encode()
		reqBody := strings.NewReader(encodedData)
		rs.req.Body = io.NopCloser(reqBody)
		rs.req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
		rs.req.ContentLength = int64(len(encodedData))
	}

	rs.req = rs.req.WithContext(ctx)

	log.DebugContext(ctx, "registration request", "server_ip", rs.serverIP, "tryIPv4", rs.tryIPv4, "tryIPv6", rs.tryIPv6, "hostname", rs.hostname, "url", rs.req.URL.String())

	resp, err := rs.cl.Do(rs.req)
	if err != nil {
		return false, err
	}
	b, err := io.ReadAll(resp.Body)
	if err != nil {
		_ = resp.Body.Close()
		return false, err
	}
	_ = resp.Body.Close()

	serverTraceID := resp.Header.Get("TraceID")
	log = log.With("trace_id", serverTraceID)

	log.DebugContext(ctx, "registration response", "server_ip", rs.serverIP, "serverTraceID", serverTraceID, "status_code", resp.StatusCode, "body", string(b))
	if resp.StatusCode >= http.StatusInternalServerError {
		log.ErrorContext(ctx, "server error", "status_code", resp.StatusCode, "body", string(b))
		select {
		case <-ctx.Done():
			return true, fmt.Errorf("setup aborted")
		case <-time.After(10 * time.Second):
			return false, nil
		}
	}

	if rs.serverIP.Is4() {
		rs.tryIPv4 = false
	} else if rs.serverIP.Is6() {
		rs.tryIPv6 = false
	}

	if rs.tryIPv4 || rs.tryIPv6 {
		rs.cl.CloseIdleConnections()
	}

	contentType := resp.Header.Get("Content-Type")
	mediaType, _, err := mime.ParseMediaType(contentType)
	if err != nil {
		log.WarnContext(ctx, "could not parse content type", "content_type", contentType, "err", err)
		mediaType = contentType // Use the raw value if parsing fails
	}

	if mediaType == "text/plain" {
		log.DebugContext(ctx, "unexpected response", "body", string(b))
		fmt.Println(string(b))
		return true, nil
	} else if mediaType != "application/json" {
		log.ErrorContext(ctx, "unexpected content type", "content_type", contentType, "media_type", mediaType)
		return true, fmt.Errorf("unexpected content type: %s", contentType)
	}

	var data struct {
		URL      string `json:"URL"`
		Status   string `json:"Status"`
		APIToken string `json:"APIToken"`
	}
	err = json.Unmarshal(b, &data)
	if err != nil {
		return true, err
	}
	if data.URL != "" {
		rs.registrationURL = data.URL
		log.DebugContext(ctx, "received registration URL", "url", data.URL)
	}

	log.DebugContext(ctx, "status", "status", data.Status)

	// Print registration URL only once after both protocol requests complete (unless one is disabled)
	if rs.registrationURL != "" && !rs.tryIPv4 && !rs.tryIPv6 && !rs.urlPrinted {
		fmt.Print(heredoc.Docf(`

				Please visit the following URL to complete the monitor registration:

					%s

				`, rs.registrationURL))
		rs.urlPrinted = true
	}

	if data.Status == "accepted" {
		log.InfoContext(ctx, "monitor registration was accepted")
		if data.APIToken == "" {
			log.ErrorContext(ctx, "no API token provided")
			return true, nil
		}

		err = rs.cli.Config.SetAPIKey(data.APIToken)
		if err != nil {
			return true, fmt.Errorf("could not store API key: %w", err)
		}

		log.InfoContext(ctx, "API key stored successfully")
		log.InfoContext(ctx, "Setup complete. The monitor must be activated by an administrator before it can start.")
		log.InfoContext(ctx, "Once activated, start monitoring with:")

		// Build command with same flags used for setup
		monitorCmd := fmt.Sprintf("ntppool-agent monitor -e %s", rs.cli.DeployEnv.String())
		if rs.cli.StateDir != "" {
			monitorCmd += fmt.Sprintf(" --state-dir '%s'", rs.cli.StateDir)
		}
		log.InfoContext(ctx, "  "+monitorCmd)

		log.InfoContext(ctx, "See documentation for systemd service setup and other deployment options.")

		return true, nil
	}

	if data.Status == "completed" {
		log.InfoContext(ctx, "registration was marked as completed")
		return true, nil
	}

	// Conservative 200ms sleep between all requests for safety
	// Fast enough to feel interactive, but prevents runaway request loops
	sleepDuration := 200 * time.Millisecond

	// Use longer sleep for status polling after URL is shown
	if rs.urlPrinted {
		sleepDuration = 5 * time.Second
	}

	select {
	case <-ctx.Done():
		return true, fmt.Errorf("setup aborted")
	case <-time.After(sleepDuration):
		return false, nil
	}
}

func (cmd *setupCmd) checkCurrentConfig(ctx context.Context, cli *ClientCmd) (bool, bool, error) {
	log := logger.FromContext(ctx)
	checkLivenessCtx, cancel := context.WithTimeout(ctx, 5*time.Second)
	defer cancel()

	// Check if we have an API key
	if cli.Config.APIKey() == "" {
		log.DebugContext(checkLivenessCtx, "No API key set")
		return false, false, nil
	}

	log.InfoContext(checkLivenessCtx, "API key already set")

	// If API key exists but no certificates, try to load config and certs
	if !cli.Config.HaveCertificate() {
		log.InfoContext(checkLivenessCtx, "API key exists but no certificates, attempting to load config")
		_, err := cli.Config.LoadAPIAppConfigWithCertificateRequest(checkLivenessCtx)
		if err != nil {
			if errors.Is(err, config.ErrAuthorization) {
				// Invalid API key - log and continue as if no API key
				log.WarnContext(checkLivenessCtx, "API key is invalid or unauthorized, continuing with setup", "err", err)
				return false, false, nil
			}
			// Other errors (network, etc) - log but don't fail
			log.DebugContext(checkLivenessCtx, "Could not load config", "err", err)
		}
	}

	var hasIPv4 bool
	var hasIPv6 bool

	if cli.Config.IPv4().IP != nil && cli.Config.IPv4().IP.IsValid() {
		log.InfoContext(checkLivenessCtx, "IPv4 address already set", "addr", cli.Config.IPv4().IP.String())
		hasIPv4 = true
	} else {
		log.InfoContext(checkLivenessCtx, "IPv4 address not set")
	}
	if cli.Config.IPv6().IP != nil && cli.Config.IPv6().IP.IsValid() {
		log.InfoContext(checkLivenessCtx, "IPv6 address already set", "addr", cli.Config.IPv6().IP.String())
		hasIPv6 = true
	} else {
		log.InfoContext(checkLivenessCtx, "IPv6 address not set")
	}
	return hasIPv4, hasIPv6, nil
}
