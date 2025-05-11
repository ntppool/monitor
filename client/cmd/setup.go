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

	req.Header.Set("User-Agent", "ntpmon/"+version.Version())
	req.Header.Set("Registration-ID", registrationID.String())
	req.Header.Set("Accept", "application/json, text/plain")

	if apiKey != "" {
		req.Header.Add("Authorization", "Bearer "+apiKey)
	}

	type registrationResponse struct {
		URL      string `json:"URL"`
		Status   string `json:"Status"`
		APIToken string `json:"APIToken"`
	}

	i := 0

	// todo: maybe take parameters for which IPs to use?

	var serverIP netip.Addr
	trace := &httptrace.ClientTrace{
		GotConn: func(connInfo httptrace.GotConnInfo) {
			remoteAddr := connInfo.Conn.RemoteAddr().String()
			// log.DebugContext(ctx, "conninfo",
			// 	"reused", connInfo.Reused,
			// 	"wasIdle", connInfo.WasIdle,
			// 	"idleTime", connInfo.IdleTime,
			// 	"remoteAddr", remoteAddr,
			// )
			addrport, err := netip.ParseAddrPort(remoteAddr)
			if err != nil {
				log.WarnContext(ctx, "could not parse remote address", "addr", remoteAddr, "err", err)
				return
			}
			serverIP = addrport.Addr()
		},
	}

	ctx = httptrace.WithClientTrace(ctx, trace)

	tryIPv4 := wantIPv4
	tryIPv6 := wantIPv6

	for {
		i++

		// log.InfoContext(ctx, "IP version attempts status", "tryIPv4", tryIPv4, "tryIPv6", tryIPv6)

		// wantIPv4 and wantIPv6 are the IP versions we want to
		// register for. tryIPv4 and tryIPv6 are the IP versions
		// we are trying to register for (disabled as the required
		// IP version has had a succesful request).

		switch {
		case tryIPv4 && tryIPv6:
			ctx = httpclient.NewIPVersionContext(ctx, httpclient.IPAny)

		case tryIPv4:
			ctx = httpclient.NewIPVersionContext(ctx, httpclient.IPv4Only)

		case tryIPv6:
			ctx = httpclient.NewIPVersionContext(ctx, httpclient.IPv6Only)

		default:
			// we have gotten what we need, so constrain the default
			// to what we need.
			switch {
			case wantIPv4 && wantIPv6:
				ctx = httpclient.NewIPVersionContext(ctx, httpclient.IPAny)
			case wantIPv4:
				ctx = httpclient.NewIPVersionContext(ctx, httpclient.IPv4Only)
			case wantIPv6:
				ctx = httpclient.NewIPVersionContext(ctx, httpclient.IPv6Only)
			}
		}

		req = req.WithContext(ctx)

		log.DebugContext(ctx, "registration request", "server_ip", serverIP, "tryIPv4", tryIPv4, "tryIPv6", tryIPv6, "url", req.URL.String())

		resp, err := cl.Do(req)
		if err != nil {
			return err
		}
		b, err := io.ReadAll(resp.Body)
		if err != nil {
			resp.Body.Close()
			return err
		}
		resp.Body.Close()

		serverTraceID := resp.Header.Get("TraceID")
		log := log.With("trace_id", serverTraceID)
		log.DebugContext(ctx, "registration response", "server_ip", serverIP, "serverTraceID", serverTraceID, "status_code", resp.StatusCode, "body", string(b))
		if resp.StatusCode >= http.StatusInternalServerError {
			log.ErrorContext(ctx, "server error", "status_code", resp.StatusCode, "body", string(b))
			select {
			// todo: merge with same code in the loop below
			case <-ctx.Done():
				// todo: send deletion request for the registration
				return fmt.Errorf("setup aborted")
			case <-time.After(10 * time.Second):
				continue
			}
		}

		if serverIP.Is4() {
			tryIPv4 = false
		} else if serverIP.Is6() {
			tryIPv6 = false
		}

		if tryIPv4 || tryIPv6 {
			// if we're deliberate about which IP version
			// to use, don't reuse connections
			cl.CloseIdleConnections()
		}

		contentType := resp.Header.Get("Content-Type")
		mediaType, _, err := mime.ParseMediaType(contentType)
		if err != nil {
			log.WarnContext(ctx, "could not parse content type", "content_type", contentType, "err", err)
			// Fallback or decide how to handle unparseable content types
			mediaType = contentType // Use the raw value if parsing fails
		}

		if mediaType == "text/plain" {
			log.DebugContext(ctx, "unexpected response", "body", string(b))
			fmt.Println(string(b))
			return nil
		} else if mediaType != "application/json" {
			log.ErrorContext(ctx, "unexpected content type", "content_type", contentType, "media_type", mediaType)
			return fmt.Errorf("unexpected content type: %s", contentType)
		}

		var data registrationResponse
		err = json.Unmarshal(b, &data)
		if err != nil {
			return err
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
				break
			}

			err = cli.Config.SetAPIKey(data.APIToken)
			if err != nil {
				return fmt.Errorf("could not store API key: %w", err)
			}

			break
		}

		if data.Status == "completed" {
			log.InfoContext(ctx, "registration was marked as completed")
			break
		}

		select {
		case <-ctx.Done():
			// todo: send deletion request for the registration
			return fmt.Errorf("setup aborted")
		case <-time.After(5 * time.Second):
			continue
		}

	}

	return nil
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
