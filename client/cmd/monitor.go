package cmd

import (
	"context"
	"fmt"
	"net/netip"
	"strings"
	"sync"
	"time"

	"connectrpc.com/connect"
	"github.com/cenkalti/backoff/v5"
	"github.com/eclipse/paho.golang/autopaho"
	"github.com/eclipse/paho.golang/paho"
	"github.com/oklog/ulid/v2"
	"github.com/prometheus/client_golang/prometheus"
	"go.opentelemetry.io/otel/attribute"
	"golang.org/x/sync/errgroup"
	"google.golang.org/protobuf/types/known/timestamppb"

	"go.ntppool.org/pingtrace/traceroute"

	"go.ntppool.org/common/logger"
	"go.ntppool.org/common/metricsserver"
	"go.ntppool.org/common/tracing"

	"go.ntppool.org/monitor/api"
	"go.ntppool.org/monitor/client/config"
	"go.ntppool.org/monitor/client/config/checkconfig"
	"go.ntppool.org/monitor/client/localok"
	"go.ntppool.org/monitor/client/monitor"
	apiv2 "go.ntppool.org/monitor/gen/monitor/v2"
	apiv2connect "go.ntppool.org/monitor/gen/monitor/v2/monitorv2connect"

	"go.ntppool.org/monitor/mqttcm"
)

// Todo:
//   - Check status.Leap != ntp.LeapNotInSync

const MaxInterval = time.Minute * 2

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

	log := logger.FromContext(ctx)

	err := cli.Config.WaitUntilLive(ctx)
	if err != nil {
		return fmt.Errorf("waiting for config: %w", err)
	}

	if checkDone(ctx) {
		log.DebugContext(ctx, "skipping monitor, context done")
		return nil
	}

	g, ctx := errgroup.WithContext(ctx)

	// todo: switch to pushing metrics over oltp
	metricssrv := metricsserver.New()
	promreg := metricssrv.Registry()

	// todo: option to enable local metrics?
	// go metricssrv.ListenAndServe(ctx, 9999)

	// todo: do we need to wait on a certificate here? It should
	// have been taken care of earlier in the config setup.
	// (previously we waited for the vault cert here)

	tracingShutdown, err := InitTracing(ctx, cli.DeployEnv, cli.Config)
	if err != nil {
		log.Error("tracing error", "err", err)
	}
	defer func() {
		if cli.Config.HaveCertificate() {
			// if we don't have a certificate, don't try
			// to flush the buffered tracing data
			tracingShutdown(context.Background())
		}
	}()

	ctx, api, err := api.Client(ctx, cli.Config.TLSName(), cli.Config)
	if err != nil {
		return fmt.Errorf("could not setup API: %w", err)
	}

	go func() {
		// this goroutine is just for logging; it's not in
		// the errgroup, so it won't block shutdown
		<-ctx.Done()
		log.Info("shutting down monitor", "name", cli.Config.TLSName())
	}()

	mqconfigger := checkconfig.NewConfigger(nil)

	for ix, ipc := range []config.IPConfig{cli.Config.IPv4(), cli.Config.IPv6()} {
		var ip_version string
		switch ix {
		case 0:
			ip_version = "v4"
			if !cli.IPv4 {
				log.DebugContext(ctx, "skipping IPv4 monitor")
				continue
			}
		case 1:
			ip_version = "v6"
			if !cli.IPv6 {
				log.DebugContext(ctx, "skipping IPv6 monitor")
				continue
			}
		}

		g.Go(func() error {
			log = log.With("ip_version", ip_version)
			ctx := logger.NewContext(ctx, log)

			if ipc.IP == nil {
				log.ErrorContext(ctx, "not configured")
				return nil
			}
			err := cmd.runMonitor(ctx, ipc, api, mqconfigger, promreg)
			log.DebugContext(ctx, "monitor done", "err", err)
			return err
		})
	}

	g.Go(func() error {
		if cmd.SanityOnly || cmd.Once {
			log.DebugContext(ctx, "skipping mqtt client for once/sanity-only")
			return nil
		}
		// wait for the config to be loaded
		for {
			if mqconfigger.GetMQTTConfig() != nil {
				break
			}
			select {
			case <-ctx.Done():
				return nil
			case <-time.After(3 * time.Second):
			}
		}
		log.InfoContext(ctx, "starting mqtt client", "name", cli.Config.TLSName())
		return runMQTTClient(ctx, cli, mqconfigger, promreg)
	})

	err = g.Wait()
	if err != nil {
		log.Error("monitor error", "err", err)
		return err
	}

	return nil
}

func runMQTTClient(ctx context.Context, cli *ClientCmd, mqconfigger checkconfig.ConfigGetter, promreg prometheus.Gatherer) error {
	log := logger.FromContext(ctx)

	var mq *autopaho.ConnectionManager
	topics := mqttcm.NewTopics(cli.Config.Env())
	statusChannel := topics.Status(cli.Config.TLSName())

	{
		if mqcfg := mqconfigger.GetMQTTConfig(); mqcfg != nil && len(mqcfg.Host) > 0 {
			log := log.WithGroup("mqtt")

			mqc := monitor.NewMQClient(log, topics, mqconfigger, promreg)
			router := paho.NewSingleHandlerRouter(mqc.Handler)

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

	mq.Disconnect(ctx)

	if mq != nil {
		// wait until the mqtt connection is done; or two seconds
		select {
		case <-mq.Done():
		case <-time.After(2 * time.Second):
		}
	}

	return nil
}

func (cmd *monitorCmd) runMonitor(ctx context.Context, ipc config.IPConfig, api apiv2connect.MonitorServiceClient, mqconfigger checkconfig.ConfigUpdater, promreg prometheus.Registerer) error {
	log := logger.FromContext(ctx).With("monitor_ip", ipc.IP.String())

	log.InfoContext(ctx, "starting monitor")

	monconf := checkconfig.NewConfigger(nil)

	initialConfig := sync.WaitGroup{}
	initialConfig.Add(1)

	go func() {
		firstRun := true

		fetchInterval := 60 * time.Minute
		errorFetchInterval := fetchInterval / 3
		errors := 0

		for {
			wait := fetchInterval

			if firstRun {
				log.InfoContext(ctx, "getting client configuration", "errors", errors)
			}

			log.WarnContext(ctx, "ipc", "ip", ipc.IP.String())

			cfgresp, err := fetchConfig(ctx, ipc, api)
			if err != nil {
				errors++
				if firstRun {
					wait = time.Second * 10 * time.Duration(errors)
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
				if firstRun {
					initialConfig.Done()
					firstRun = false
				}
			}

			select {
			case <-time.After(wait):
			case <-ctx.Done():
				if firstRun {
					initialConfig.Done()
				}
				return
			}
		}
	}()

	initialConfig.Wait()

	if checkDone(ctx) {
		// config loading was cancelled, so we don't have a config
		// yet which everything below needs
		return nil
	}

	// todo: update mqtt status with current health from localok?
	localOK := localok.NewLocalOK(monconf, promreg)

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

		boff := backoff.NewExponentialBackOff()
		boff.RandomizationFactor = 0.3
		boff.InitialInterval = 3 * time.Second
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
				return 0, fmt.Errorf("no work")
			} else {
				return count, nil
			}
		}

		count, err := backoff.Retry(ctx,
			doBatch,
			backoff.WithBackOff(boff),
		)
		if err != nil {
			log.Error("backoff error", "err", err)
		}

		if count > 0 {
			log.InfoContext(ctx, "batch processing", "count", count)
		}

		if i > 0 && cmd.Once {
			log.InfoContext(ctx, "asked to only run once")
			break
		} else {
			log.InfoContext(ctx, "not once?", "once", cmd.Once, "i", i)
		}

		if checkDone(ctx) {
			break runLoop
		}
	}

	return nil
}

func fetchConfig(ctx context.Context, ipc config.IPConfig, api apiv2connect.MonitorServiceClient) (*apiv2.GetConfigResponse, error) {
	cfgctx, cfgcancel := context.WithTimeout(ctx, 5*time.Second)
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
	if err != nil || cfgresp.Msg == nil {
		log.ErrorContext(ctx, "could not get config, http error", "err", err)
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
	batchID.UnmarshalText(serverlist.BatchId)

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
				tr.Start(ctx)
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

	log.InfoContext(ctx, "submitting")

	list := &apiv2.SubmitResultsRequest{
		MonId:   ipc.IP.String(),
		Version: 4,
		List:    statuses,
		BatchId: serverlist.BatchId,
	}

	r, err := api.SubmitResults(ctx, connect.NewRequest(list))
	if err != nil {
		return 0, fmt.Errorf("SubmitResults: %s", err)
	}
	if !r.Msg.Ok {
		return 0, fmt.Errorf("SubmitResults not okay")
	}

	return len(statuses), nil
}

func checkDone(ctx context.Context) bool {
	select {
	case <-ctx.Done():
		return true
	default:
		return false
	}
}
