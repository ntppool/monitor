package cmd

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"net/netip"
	"strings"
	"sync"
	"time"

	"connectrpc.com/connect"
	"github.com/cenkalti/backoff/v5"
	"github.com/eclipse/paho.golang/autopaho"
	"github.com/eclipse/paho.golang/paho"
	"github.com/oklog/ulid/v2"
	"go.opentelemetry.io/otel/attribute"
	"go.opentelemetry.io/otel/metric"
	"golang.org/x/sync/errgroup"
	"google.golang.org/protobuf/types/known/timestamppb"

	"go.ntppool.org/pingtrace/traceroute"

	"go.ntppool.org/common/logger"
	"go.ntppool.org/common/tracing"
	"go.ntppool.org/common/version"

	"go.ntppool.org/monitor/api"
	"go.ntppool.org/monitor/client/config"
	"go.ntppool.org/monitor/client/config/checkconfig"
	"go.ntppool.org/monitor/client/localok"
	"go.ntppool.org/monitor/client/metrics"
	"go.ntppool.org/monitor/client/monitor"
	apiv2 "go.ntppool.org/monitor/gen/monitor/v2"
	apiv2connect "go.ntppool.org/monitor/gen/monitor/v2/monitorv2connect"

	"go.ntppool.org/monitor/mqttcm"
)

// Todo:
//   - Check status.Leap != ntp.LeapNotInSync

const MaxInterval = time.Minute * 2

const (
	DefaultConfigWait     = 3 * time.Second
	DefaultMQTTWait       = 2 * time.Second
	DefaultFetchInterval  = 60 * time.Minute
	DefaultErrorRetryBase = 10 * time.Second
)

var ErrNoWork = errors.New("no work")

// type SetConfig interface {
// 	SetConfigFromPb(cfg *pb.Config)
// 	SetConfigFromApi(cfg *apiv2.GetConfigResponse)
// }

// type ConfigStore interface {
// 	SetConfig
// 	GetConfig() *checkconfig.Config
// }

type monitorCmd struct {
	Once       bool `name:"once" help:"Only run once instead of forever"`
	SanityOnly bool `name:"sanity-only" help:"Only run the local sanity check"`
}

func (cmd *monitorCmd) Run(ctx context.Context, cli *ClientCmd) error {
	ctx, stopMonitor := context.WithCancel(ctx)
	defer stopMonitor()

	log := logger.FromContext(ctx).With("env", cli.DeployEnv.String())
	ctx = logger.NewContext(ctx, log)

	log.InfoContext(ctx, "starting ntppool-agent", "version", version.Version())

	g, ctx := errgroup.WithContext(ctx)

	// Start the config manager early so it can watch for file changes
	// during WaitUntilLive

	if !cmd.SanityOnly && !cmd.Once {
		g.Go(func() error {
			log.InfoContext(ctx, "starting AppConfig manager early for file watching", "name", cli.Config.TLSName())
			return cli.Config.Manager(ctx)
		})
	}

	err := cli.Config.WaitUntilLive(ctx)
	if err != nil {
		return fmt.Errorf("waiting for config: %w", err)
	}

	log.DebugContext(ctx, "WaitUntilLive done. continuing work", "haveCertificate", cli.Config.HaveCertificate())

	if checkDone(ctx) {
		log.DebugContext(ctx, "skipping monitor, context done")
		return nil
	}

	// Metrics are now handled via OpenTelemetry OTLP export

	// Wait for certificates to be loaded before setting up API client
	// This ensures we don't try to use the API without proper TLS authentication
	log.DebugContext(ctx, "waiting for certificates to be loaded before API setup")
	err = cli.Config.WaitUntilCertificatesLoaded(ctx)
	if err != nil {
		log.WarnContext(ctx, "error waiting for certificates", "err", err)
		// Continue anyway - the API client will handle certificate errors
	}

	tracingShutdown, err := InitTracing(ctx, cli.DeployEnv, cli.Config)
	if err != nil {
		log.WarnContext(ctx, "tracing initialization failed", "err", err)
	} else {
		defer func() {
			if cli.Config.HaveCertificate() {
				// if we don't have a certificate, don't try
				// to flush the buffered tracing data
				shutdownCtx, cancel := context.WithTimeout(context.Background(), 15*time.Second)
				defer cancel()
				if err := tracingShutdown(shutdownCtx); err != nil {
					log.WarnContext(ctx, "failed to shutdown tracing", "err", err)
				}
			}
		}()
	}

	ctx, api, err := api.Client(ctx, cli.Config.TLSName(), cli.Config)
	if err != nil {
		return fmt.Errorf("could not setup API: %w", err)
	}

	go func() {
		// this goroutine is just for logging; it's not in
		// the errgroup, so it won't block shutdown when the
		// other goroutines are done
		<-ctx.Done()
		log.Info("shutting down monitor", "name", cli.Config.TLSName())
	}()

	mqconfigger := checkconfig.NewConfigger(nil)

	for ix, ipc := range []config.IPConfig{cli.Config.IPv4(), cli.Config.IPv6()} {
		var ipVersion string
		switch ix {
		case 0:
			ipVersion = "v4"
			if !cli.IPv4 {
				log.DebugContext(ctx, "skipping IPv4 monitor")
				continue
			}
		case 1:
			ipVersion = "v6"
			if !cli.IPv6 {
				log.DebugContext(ctx, "skipping IPv6 monitor")
				continue
			}
		}

		g.Go(func() error {
			// Create a new logger instance for this goroutine to avoid shared state
			ipLog := log.With("ip_version", ipVersion)
			ctx := logger.NewContext(ctx, ipLog)

			if ipc.IP == nil {
				ipLog.ErrorContext(ctx, "not configured")
				return nil
			}

			// Check if protocol is live according to AppConfig before starting monitor
			if !ipc.IsLive() {
				ipLog.InfoContext(ctx, "protocol not active, waiting for activation")
				// Wait for config change
			outerLoop:
				for {
					configChangeWaiter := cli.Config.WaitForConfigChange(ctx)
					select {
					case <-configChangeWaiter.Done():
						// Get fresh status
						if ipc.IP.Is4() {
							ipc = cli.Config.IPv4()
						} else {
							ipc = cli.Config.IPv6()
						}
						if ipc.IsLive() {
							ipLog.InfoContext(ctx, "protocol is now active, starting monitor")
							break outerLoop
						}
					case <-ctx.Done():
						configChangeWaiter.Cancel() // Clean up on exit
						return nil
					}
				}
			}

			err := cmd.runMonitor(ctx, ipc, api, mqconfigger, cli.Config)
			ipLog.DebugContext(ctx, "monitor done", "err", err)
			return err
		})
	}

	g.Go(func() error {
		if cmd.SanityOnly || cmd.Once {
			log.DebugContext(ctx, "skipping mqtt client for once/sanity-only")
			return nil
		}
		// wait for the config to be loaded
		for mqconfigger.GetMQTTConfig() == nil {
			select {
			case <-ctx.Done():
				return nil
			case <-time.After(DefaultConfigWait):
			}
		}
		log.InfoContext(ctx, "starting mqtt client", "name", cli.Config.TLSName())
		return runMQTTClient(ctx, cli, mqconfigger)
	})

	err = g.Wait()
	if err != nil {
		log.Error("monitor error", "err", err)
		return err
	}

	return nil
}

func runMQTTClient(ctx context.Context, cli *ClientCmd, mqconfigger checkconfig.ConfigGetter) error {
	log := logger.FromContext(ctx)

	var mq *autopaho.ConnectionManager
	topics := mqttcm.NewTopics(cli.Config.Env())
	statusChannel := topics.Status(cli.Config.TLSName())

	{
		if mqcfg := mqconfigger.GetMQTTConfig(); mqcfg != nil && len(mqcfg.Host) > 0 {
			log := log.WithGroup("mqtt")

			mqc := monitor.NewMQClient(log, topics, mqconfigger)
			router := paho.NewStandardRouterWithDefault(func(m *paho.Publish) {
				log.Debug("mqtt message (unhandled)", "topic", m.Topic, "payload", m.Payload)
			})

			// Register explicit handler for request topics
			requestSubscription := topics.RequestSubscription(cli.Config.TLSName())
			router.RegisterHandler(requestSubscription, mqc.Handler)

			var err error
			mq, err = mqttcm.Setup(
				ctx, cli.Config.TLSName(), statusChannel,
				[]string{
					topics.RequestSubscription(cli.Config.TLSName()),
				}, router, mqconfigger, cli.Config,
			)
			if err != nil {
				return fmt.Errorf("mqtt: %w", err)
			}

			err = mq.AwaitConnection(ctx)
			if err != nil {
				return fmt.Errorf("mqtt connection error: %w", err)
			}

			mqc.SetMQ(mq)
		}
	}

	// Wait for context cancellation before disconnecting
	// This keeps the MQTT connection alive to receive and process messages
	<-ctx.Done()
	log.InfoContext(ctx, "MQTT client shutting down")

	if mq != nil {
		if err := mq.Disconnect(ctx); err != nil {
			log.WarnContext(ctx, "Failed to disconnect MQTT", "err", err)
		}
	}

	if mq != nil {
		// wait until the mqtt connection is done; or two seconds
		select {
		case <-mq.Done():
		case <-time.After(DefaultMQTTWait):
		}
	}

	return nil
}

func (cmd *monitorCmd) runMonitor(ctx context.Context, ipc config.IPConfig, api apiv2connect.MonitorServiceClient, mqconfigger checkconfig.ConfigUpdater, appConfig config.AppConfig) error {
	log := logger.FromContext(ctx).With("monitor_ip", ipc.IP.String())

	log.InfoContext(ctx, "starting monitor")

	monconf := checkconfig.NewConfigger(nil)

	initialConfig := sync.WaitGroup{}
	initialConfig.Add(1)

	go func() {
		firstRun := true

		fetchInterval := DefaultFetchInterval
		errorFetchInterval := fetchInterval / 3
		errors := 0

		for {
			wait := fetchInterval
			configChangeWaiter := appConfig.WaitForConfigChange(ctx)

			if firstRun {
				log.InfoContext(ctx, "getting client configuration with BaseChecks for LocalOK", "errors", errors)
			}

			cfgresp, err := fetchConfig(ctx, ipc, api)
			if err != nil {
				log.WarnContext(ctx, "fetching config", "err", err, "monitor_ip", ipc.IP.String())
				errors++
				if firstRun {
					wait = DefaultErrorRetryBase * time.Duration(errors)
					if wait > errorFetchInterval {
						wait = errorFetchInterval
					}
				} else {
					wait = errorFetchInterval
				}
			} else {
				log.DebugContext(ctx, "client config", "cfg", cfgresp)
				mqconfigger.SetConfigFromApi(cfgresp)
				monconf.SetConfigFromApi(cfgresp)

				// Only signal completion on first run if we have valid BaseChecks for LocalOK
				if firstRun {
					if cfgresp != nil && len(cfgresp.BaseChecks) > 0 {
						log.InfoContext(ctx, "received valid configuration with BaseChecks, LocalOK can be initialized", "basecheck_count", len(cfgresp.BaseChecks))
						initialConfig.Done()
						firstRun = false
					} else {
						log.WarnContext(ctx, "configuration received but no BaseChecks available, waiting for complete config")
						// Continue as first run until we get BaseChecks
					}
				}
			}

			select {
			case <-time.After(wait):
				configChangeWaiter.Cancel() // Clean up when timer triggers
			case <-configChangeWaiter.Done():
				log.InfoContext(ctx, "HTTP config changed, triggering immediate gRPC config fetch")
				// Config changed, fetch immediately (continue loop with no wait)
			case <-ctx.Done():
				configChangeWaiter.Cancel() // Clean up on exit
				if firstRun {
					initialConfig.Done()
				}
				return
			}
		}
	}()

	initialConfig.Wait()

	if checkDone(ctx) {
		// config loading was cancelled, so we don't have a config with BaseChecks
		// yet which LocalOK and the monitoring loops need
		return nil
	}

	// LocalOK can now be safely created since we're guaranteed to have valid BaseChecks
	log.DebugContext(ctx, "setting up localok.NewLocalOK with validated configuration")
	localOK := localok.NewLocalOK(monconf)

	if cmd.SanityOnly {
		ok := localOK.Check(ctx)
		if ok {
			log.InfoContext(ctx, "local clock ok")
			return nil
		}
		log.WarnContext(ctx, "local clock not ok")
		return fmt.Errorf("health check failed")
	}

runLoop:
	for i := 1; true; i++ {
		// Check if protocol is still live, wait for reactivation if not
		if !cmd.waitForProtocolActivation(ctx, ipc, appConfig, log) {
			return nil // Context was cancelled
		}

		boff := backoff.NewExponentialBackOff()
		boff.RandomizationFactor = 0.3
		boff.InitialInterval = DefaultConfigWait
		boff.MaxInterval = MaxInterval

		doBatch := func() (int, error) {
			if checkDone(ctx) {
				return 0, nil
			}

			if !localOK.Check(ctx) {
				wait := localOK.NextCheckIn()
				log.InfoContext(ctx, "local clock might not be okay", "waiting", wait.Round(1*time.Second).String())
				select {
				case <-ctx.Done():
					log.DebugContext(ctx, "localOK context done")
					return 0, nil
				case <-time.After(wait):
					return 0, nil
				}
			}

			// todo: proxy monconf so we also set mqconfigger

			if count, err := cmd.doMonitorBatch(ctx, ipc, api, monconf); count == 0 || err != nil {
				if err != nil {
					log.ErrorContext(ctx, "batch processing", "err", err)
					return 0, err
				}
				if cmd.Once {
					// just once, don't retry
					return 0, nil
				}
				return 0, ErrNoWork
			} else {
				return count, nil
			}
		}

		count, err := backoff.Retry(ctx,
			doBatch,
			backoff.WithBackOff(boff),
		)
		if err != nil {
			if !errors.Is(err, ErrNoWork) {
				log.ErrorContext(ctx, "backoff error", "err", err)
			}
		}

		if count > 0 {
			log.InfoContext(ctx, "batch processing", "count", count)
		}

		if i > 0 && cmd.Once {
			log.InfoContext(ctx, "asked to only run once")
			break
		}

		if checkDone(ctx) {
			break runLoop
		}
	}

	return nil
}

func fetchConfig(ctx context.Context, ipc config.IPConfig, api apiv2connect.MonitorServiceClient) (*apiv2.GetConfigResponse, error) {
	cfgctx, cfgcancel := context.WithTimeout(ctx, 35*time.Second)
	defer cfgcancel()
	log := logger.FromContext(ctx)

	if ipc.IP == nil {
		log.ErrorContext(ctx, "not configured")
		return nil, fmt.Errorf("not configured")
	}

	cfgresp, err := api.GetConfig(cfgctx,
		connect.NewRequest(
			&apiv2.GetConfigRequest{MonId: ipc.IP.String()},
		))

	// Record RPC request metric
	if metrics.RPCRequests != nil {
		statusCode := "success"
		if err != nil {
			statusCode = "error"
		}
		metrics.RPCRequests.Add(ctx, 1,
			metric.WithAttributes(
				attribute.String("type", "config"),
				attribute.String("status_code", statusCode)))
	}

	if err != nil || cfgresp.Msg == nil {
		if strings.Contains(err.Error(), "tls: expired certificate") {
			log.ErrorContext(ctx, "TLS certificate error - check server certificate validity", "err", err, "monitor_ip", ipc.IP.String())
		} else {
			log.ErrorContext(ctx, "could not get config, http error", "err", err, "monitor_ip", ipc.IP.String())
		}
		return nil, err
	}
	return cfgresp.Msg, nil
}

func (cmd *monitorCmd) doMonitorBatch(ctx context.Context, ipc config.IPConfig, api apiv2connect.MonitorServiceClient, cfgStore checkconfig.ConfigProvider) (int, error) {
	log := logger.FromContext(ctx)

	ctx, span := tracing.Start(ctx, "monitor-run")
	defer span.End()

	serverresp, err := api.GetServers(ctx,
		connect.NewRequest(
			&apiv2.GetServersRequest{
				MonId: ipc.IP.String(),
			},
		),
	)

	// Record RPC request metric
	if metrics.RPCRequests != nil {
		statusCode := "success"
		if err != nil {
			statusCode = "error"
		}
		metrics.RPCRequests.Add(ctx, 1,
			metric.WithAttributes(
				attribute.String("type", "getServers"),
				attribute.String("status_code", statusCode)))
	}

	if err != nil {
		return 0, fmt.Errorf("getting server list: %s", err)
	}

	serverlist := serverresp.Msg

	if serverlist.Config != nil {
		cfgStore.SetConfigFromApi(serverlist.Config)
	}

	if len(serverlist.Servers) == 0 {
		// no error, just no servers
		return 0, nil
	}

	batchID := ulid.ULID{}
	if err := batchID.UnmarshalText(serverlist.BatchId); err != nil {
		return 0, fmt.Errorf("failed to unmarshal batch ID: %w", err)
	}

	log = log.With("batchID", batchID.String())

	span.SetAttributes(attribute.String("batchID", batchID.String()))

	log.DebugContext(ctx, "processing", "server_count", len(serverlist.Servers))

	// we're testing, so limit how much work ...
	if cmd.Once {
		if len(serverlist.Servers) > 10 {
			serverlist.Servers = serverlist.Servers[0:9]
		}
	}

	statuses := []*apiv2.ServerStatus{}

	wg := sync.WaitGroup{}

	mu := sync.Mutex{}

	for _, s := range serverlist.Servers {

		wg.Add(1)

		go func(s *netip.Addr, trace bool, ticket []byte) {
			if trace {
				// todo: get psuedo lock from channel to manage parallel traceroutes

				tr, err := traceroute.New(*s)
				if err != nil {
					log.Error("traceroute", "err", err)
				}
				if err := tr.Start(ctx); err != nil {
					log.Error("traceroute start failed", "err", err)
				}
				x, err := tr.ReadAll()
				if err != nil {
					log.Error("traceroute", "err", err)
				}

				log.Info("traceroute", "output", x)

				wg.Done()
				return
			}

			status, _, err := monitor.CheckHost(ctx, s, cfgStore.GetConfig())
			if status == nil {
				status = &apiv2.ServerStatus{
					NoResponse: true,
				}
				status.SetIP(s)
			}
			status.Ticket = ticket
			if err != nil {
				log.Info("ntp error", "server", s, "err", err)
				if strings.HasPrefix(err.Error(), "network:") {
					status.NoResponse = true
				}
				status.Error = err.Error()
			}
			status.Ts = timestamppb.Now()

			mu.Lock()
			defer mu.Unlock()
			statuses = append(statuses, status)
			wg.Done()
		}(s.IP(), s.Trace, s.Ticket)
	}

	wg.Wait()

	list := &apiv2.SubmitResultsRequest{
		MonId:   ipc.IP.String(),
		Version: 4,
		List:    statuses,
		BatchId: serverlist.BatchId,
	}

	r, err := api.SubmitResults(ctx, connect.NewRequest(list))

	// Record RPC request metric
	if metrics.RPCRequests != nil {
		statusCode := "success"
		if err != nil || (r != nil && !r.Msg.Ok) {
			statusCode = "error"
		}
		metrics.RPCRequests.Add(ctx, 1,
			metric.WithAttributes(
				attribute.String("type", "submitResults"),
				attribute.String("status_code", statusCode)))
	}

	if err != nil {
		return 0, fmt.Errorf("SubmitResults: %s", err)
	}
	if !r.Msg.Ok {
		return 0, fmt.Errorf("SubmitResults not okay")
	}

	return len(statuses), nil
}

func (cmd *monitorCmd) waitForProtocolActivation(ctx context.Context, ipc config.IPConfig, appConfig config.AppConfig, log *slog.Logger) bool {
	// Check current status
	var currentIpc config.IPConfig
	if ipc.IP.Is4() {
		currentIpc = appConfig.IPv4()
	} else {
		currentIpc = appConfig.IPv6()
	}

	// If already live, no need to wait
	if currentIpc.IsLive() {
		log.DebugContext(ctx, "protocol is already active")
		return true
	}

	log.InfoContext(ctx, "protocol is inactive, waiting for activation")

	// Wait for config changes instead of polling
	for {
		configChangeCtx := appConfig.WaitForConfigChange(ctx)
		select {
		case <-configChangeCtx.Done():
			log.DebugContext(ctx, "config changed, checking protocol status")
			// Get fresh status
			if ipc.IP.Is4() {
				currentIpc = appConfig.IPv4()
			} else {
				currentIpc = appConfig.IPv6()
			}
			if currentIpc.IsLive() {
				log.InfoContext(ctx, "protocol is now active")
				return true
			}
			log.DebugContext(ctx, "protocol still inactive, continuing to wait")
		case <-ctx.Done():
			log.DebugContext(ctx, "context cancelled while waiting for activation")
			return false
		}
	}
}

func checkDone(ctx context.Context) bool {
	select {
	case <-ctx.Done():
		return true
	default:
		return false
	}
}
