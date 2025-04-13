package cmd

import (
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/http/httptrace"
	"net/netip"
	"time"

	"github.com/MakeNowJust/heredoc"
	"github.com/spf13/cobra"

	"go.ntppool.org/common/logger"
	"go.ntppool.org/common/tracing"
	"go.ntppool.org/common/ulid"
	"go.ntppool.org/common/version"
	"go.ntppool.org/monitor/client/httpclient"
)

func (cli *CLI) setupCmd() *cobra.Command {
	setupCmd := &cobra.Command{
		Use:   "setup",
		Short: "initial authentication and configuration",
		Long:  ``,
		RunE:  cli.Run(cli.setupRun),
		Args:  cobra.MatchAll(cobra.NoArgs),
	}
	setupCmd.PersistentFlags().AddGoFlagSet(cli.Flags())

	return setupCmd
}

func (cli *CLI) setupRun(cmd *cobra.Command, args []string) error {
	ctx := cmd.Context()
	log := logger.FromContext(ctx)

	ctx, span := tracing.Start(ctx, "monitor.setup")
	defer span.End()

	apiKey := cli.Config.APIKey()
	if apiKey != "" {
		log.InfoContext(ctx, "API key already set, skipping setup")
		return nil
	}

	// todo: load config, require --reset to overwrite if one exists?

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

	req.Header.Set("User-Agent", "ntppool-monitor/"+version.Version())
	req.Header.Set("Registration-ID", registrationID.String())

	type registrationResponse struct {
		URL      string `json:"URL"`
		Status   string `json:"Status"`
		APIToken string `json:"APIToken"`
	}

	tryIPv4 := true
	tryIPv6 := true
	i := 0

	// todo: maybe take parameters for which IPs to use?

	var serverIP netip.Addr
	trace := &httptrace.ClientTrace{
		GotConn: func(connInfo httptrace.GotConnInfo) {
			remoteAddr := connInfo.Conn.RemoteAddr().String()
			log.DebugContext(ctx, "conninfo",
				"reused", connInfo.Reused,
				"wasIdle", connInfo.WasIdle,
				"idleTime", connInfo.IdleTime,
				"remoteAddr", remoteAddr,
			)
			addrport, err := netip.ParseAddrPort(remoteAddr)
			if err != nil {
				log.WarnContext(ctx, "could not parse remote address", "addr", remoteAddr, "err", err)
				return
			}
			serverIP = addrport.Addr()
		},
	}

	ctx = httptrace.WithClientTrace(ctx, trace)

	for {
		i++

		// log.InfoContext(ctx, "IP version attempts status", "tryIPv4", tryIPv4, "tryIPv6", tryIPv6)

		if i > 1 {
			if i%2 == 0 {
				if tryIPv4 {
					ctx = httpclient.NewIPVersionContext(ctx, httpclient.IPv4Only)
				} else if tryIPv6 {
					ctx = httpclient.NewIPVersionContext(ctx, httpclient.IPv6Only)
				} else {
					ctx = httpclient.NewIPVersionContext(ctx, httpclient.IPAny)
				}
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
			return err
		}
		resp.Body.Close()

		serverTraceID := resp.Header.Get("TraceID")
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
