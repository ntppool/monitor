package cmd

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"mime"
	"net/http"
	"net/http/httptrace"
	"net/netip"
	"time"

	"github.com/MakeNowJust/heredoc"

	"go.ntppool.org/common/logger"
	"go.ntppool.org/common/tracing"
	"go.ntppool.org/common/ulid"
	"go.ntppool.org/common/version"
	"go.ntppool.org/monitor/client/httpclient"
)

type setupCmd struct{}

func (cmd *setupCmd) Run(ctx context.Context, cli *ClientCmd) error {
	log := logger.FromContext(ctx)
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

	cl := httpclient.CreateIPVersionAwareClient()
	apiHost := cli.Config.Env().APIHost()

	req, err := http.NewRequestWithContext(ctx, "POST",
		fmt.Sprintf("%s/monitor/api/registration", apiHost),
		nil,
	)
	if err != nil {
		return err
	}

	req.Header.Set("User-Agent", "ntppool-agent/"+version.Version())
	req.Header.Set("Registration-ID", registrationID.String())
	req.Header.Set("Accept", "application/json, text/plain")

	if apiKey != "" {
		req.Header.Add("Authorization", "Bearer "+apiKey)
	}

	rs := &registrationState{
		cl:       cl,
		req:      req,
		serverIP: netip.Addr{},
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

	// wantIPv4 and wantIPv6 are the IP versions we want to
	// register for. tryIPv4 and tryIPv6 are the IP versions
	// we are trying to register for (disabled as the required
	// IP version has had a succesful request).
	tryIPv4  bool
	tryIPv6  bool
	wantIPv4 bool
	wantIPv6 bool
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

	rs.req = rs.req.WithContext(ctx)

	log.DebugContext(ctx, "registration request", "server_ip", rs.serverIP, "tryIPv4", rs.tryIPv4, "tryIPv6", rs.tryIPv6, "url", rs.req.URL.String())

	resp, err := rs.cl.Do(rs.req)
	if err != nil {
		return false, err
	}
	b, err := io.ReadAll(resp.Body)
	if err != nil {
		resp.Body.Close()
		return false, err
	}
	resp.Body.Close()

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
		fmt.Print(heredoc.Docf(`

				Please visit the following URL to complete the monitor registration:

					%s

				`, data.URL))
	}

	log.DebugContext(ctx, "status", "status", data.Status)

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

		return true, nil
	}

	if data.Status == "completed" {
		log.InfoContext(ctx, "registration was marked as completed")
		return true, nil
	}

	select {
	case <-ctx.Done():
		return true, fmt.Errorf("setup aborted")
	case <-time.After(5 * time.Second):
		return false, nil
	}
}

func (cmd *setupCmd) checkCurrentConfig(ctx context.Context, cli *ClientCmd) (bool, bool, error) {
	log := logger.FromContext(ctx)
	checkLivenessCtx, cancel := context.WithTimeout(ctx, 5*time.Second)
	defer cancel()
	err := cli.Config.WaitUntilConfigured(checkLivenessCtx)
	if err != nil {
		log.ErrorContext(checkLivenessCtx, "could not check config (reset keys to create a new monitor)", "err", err)
		return false, false, err
	} else {
		log.InfoContext(checkLivenessCtx, "API key already set")
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
