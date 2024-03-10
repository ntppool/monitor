package cmd

import (
	"context"
	"errors"
	"flag"
	"fmt"
	"net/netip"
	"os"
	"os/signal"
	"strings"
	"sync"
	"syscall"
	"time"

	"connectrpc.com/connect"
	"github.com/cenkalti/backoff"
	"github.com/eclipse/paho.golang/autopaho"
	"github.com/eclipse/paho.golang/paho"
	"github.com/oklog/ulid/v2"
	"github.com/spf13/cobra"
	"github.com/twitchtv/twirp"
	"go.opentelemetry.io/otel/attribute"
	"google.golang.org/protobuf/types/known/timestamppb"

	"go.ntppool.org/pingtrace/traceroute"

	"go.ntppool.org/common/logger"
	"go.ntppool.org/common/metricsserver"
	"go.ntppool.org/common/tracing"
	"go.ntppool.org/monitor/api"
	"go.ntppool.org/monitor/api/pb"
	"go.ntppool.org/monitor/client/auth"
	"go.ntppool.org/monitor/client/config"
	"go.ntppool.org/monitor/client/localok"
	"go.ntppool.org/monitor/client/monitor"
	apiv2 "go.ntppool.org/monitor/gen/monitor/v2"
	apiv2connect "go.ntppool.org/monitor/gen/monitor/v2/monitorv2connect"

	"go.ntppool.org/monitor/mqttcm"
)

// Todo:
//   - Check status.Leap != ntp.LeapNotInSync

var (
	onceFlag       = flag.Bool("once", false, "Only run once instead of forever")
	sanityOnlyFlag = flag.Bool("sanity-only", false, "Only run the local sanity check")
)

const MaxInterval = time.Minute * 2

type SetConfig interface {
	SetConfigFromPb(cfg *pb.Config)
	SetConfigFromApi(cfg *apiv2.GetConfigResponse)
}

type ConfigStore interface {
	SetConfig
	GetConfig() *config.Config
}

func (cli *CLI) monitorCmd() *cobra.Command {

	monitorCmd := &cobra.Command{
		Use:   "monitor",
		Short: "Run monitor",
		Long:  ``,
		RunE:  cli.Run(cli.startMonitor),
	}
	monitorCmd.PersistentFlags().AddGoFlagSet(cli.Config.Flags())

	return monitorCmd
}

func (cli *CLI) startMonitor(cmd *cobra.Command) error {
	ctx, stopMonitor := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stopMonitor()

	log := logger.Setup()
	ctx = logger.NewContext(ctx, log)

	metricssrv := metricsserver.New()
	promreg := metricssrv.Registry()

	// todo: option to enable local metrics?
	// go metricssrv.ListenAndServe(ctx, 9999)

	cauth, err := cli.ClientAuth(ctx)
	if err != nil {
		return fmt.Errorf("auth: %w", err)
	}

	deployEnv, err := api.GetDeploymentEnvironmentFromName(cli.Config.Name)
	if err != nil {
		return err
	}

	err = cauth.Login()
	if err != nil {
		var aerr auth.AuthenticationError
		if errors.As(err, &aerr) {
			var url string
			switch deployEnv {
			case api.DeployDevel:
				url = "https://manage.askdev.grundclock.com/manage/monitors"
			case api.DeployTest:
				url = "https://manage.beta.grundclock.com/manage/monitors"
			case api.DeployProd:
				url = "https://manage.ntppool.org/manage/monitors"
			default:
				url = "the management site"
			}

			log.Error("authentication error, go to manage site to rotate and download a new API secret", "err", aerr, "url", url)
			os.Exit(2)
		}

		log.Error("could not authenticate", "err", err)
		os.Exit(2)
	}

	err = cauth.LoadOrIssueCertificates()
	if err != nil {
		return fmt.Errorf("LoadOrIssueCertificates: %w", err)
	}

	go cauth.Manager(promreg)

	// block until we have a valid certificate
	err = cauth.WaitUntilReady()
	if err != nil {
		log.Error("failed waiting for authentication to be ready", "err", err)
		os.Exit(2)
	}

	tracingShutdown, err := InitTracing(cli.Config.Name, cauth)
	if err != nil {
		log.Error("tracing error", "err", err)
	}
	defer tracingShutdown()

	ctx, api, err := api.Client(ctx, cli.Config.Name, cauth)
	if err != nil {
		return fmt.Errorf("could not setup API: %w", err)
	}

	go func() {
		// this goroutine is just for logging
		<-ctx.Done()
		log.Info("Shutting down monitor", "name", cauth.Name)
	}()

	conf := config.NewConfigger(nil)

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
				log.Info("getting client configuration", "errors", errors)
			}

			cfgctx, cfgcancel := context.WithTimeout(ctx, 5*time.Second)

			cfgresp, err := api.GetConfig(cfgctx, connect.NewRequest(&apiv2.GetConfigRequest{}))
			if err != nil || cfgresp.Msg == nil {
				errors++
				if twerr, ok := err.(twirp.Error); ok {
					// if twerr.Code() == twirp.PermissionDenied {}
					log.Error("could not get config, api error: %w", "err", twerr)

				} else {
					log.Error("could not get config, http error: %w", "err", err)
				}
				if firstRun {
					wait = time.Second * 10 * time.Duration(errors)
					if wait > errorFetchInterval {
						wait = errorFetchInterval
					}
				} else {
					wait = errorFetchInterval
				}
			} else {
				log.Info("client config", "cfg", cfgresp.Msg)
				conf.SetConfigFromApi(cfgresp.Msg)
				if firstRun {
					initialConfig.Done()
					firstRun = false
				}
			}
			cfgcancel()
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

	if conf.GetConfig() == nil {
		// we were aborted before getting this far
		log.Warn("did not load remote configuration")
		os.Exit(2)
	}

	var mq *autopaho.ConnectionManager
	topics := mqttcm.NewTopics(deployEnv)
	statusChannel := topics.Status(cauth.Name)

	{
		cfg := conf.GetConfig()

		if mqcfg := cfg.MQTTConfig; mqcfg != nil && len(mqcfg.Host) > 0 {
			log := log.WithGroup("mqtt")

			mqc := monitor.NewMQClient(log, topics, conf, promreg)
			router := paho.NewSingleHandlerRouter(mqc.Handler)

			mq, err = mqttcm.Setup(
				ctx, cauth.Name, statusChannel,
				[]string{
					topics.RequestSubscription(cauth.Name),
				}, router, conf, cauth,
			)
			if err != nil {
				return fmt.Errorf("mqtt: %w", err)
			}

			err := mq.AwaitConnection(ctx)
			if err != nil {
				return fmt.Errorf("mqtt connection error: %w", err)
			}

			mqc.SetMQ(mq)
		}
	}

	localOK := localok.NewLocalOK(conf, promreg)

	if *sanityOnlyFlag {
		ok := localOK.Check(ctx)
		if ok {
			log.Info("Local clock ok")
			return nil
		}
		log.Info("Local clock not ok")
		return fmt.Errorf("health check failed")
	}

	// todo: update mqtt status with current health

	i := 0

runLoop:
	for {
		boff := backoff.NewExponentialBackOff()
		boff.RandomizationFactor = 0.3
		boff.InitialInterval = 3 * time.Second
		boff.MaxInterval = MaxInterval
		boff.MaxElapsedTime = 0

		err := backoff.Retry(func() error {

			if checkDone(ctx) {
				return nil
			}

			if !localOK.Check(ctx) {
				wait := localOK.NextCheckIn()
				log.Info("local clock might not be okay", "waiting", wait.Round(1*time.Second).String())
				select {
				case <-ctx.Done():
					log.Info("localOK context done")
					return nil
				case <-time.After(wait):
					return fmt.Errorf("local clock")
				}
			}

			if ok, err := run(ctx, api, conf); !ok || err != nil {
				if err != nil {
					log.Error("batch processing", "err", err)
					boff.MaxInterval = 10 * time.Minute
					boff.Multiplier = 5
				}
				return fmt.Errorf("no work")
			}
			boff.Reset()
			return nil
		}, boff)

		if err != nil {
			log.Error("backoff error", "err", err)
		}

		i++
		if i > 0 && *onceFlag {
			log.Info("Asked to only run once")
			break
		}

		if checkDone(ctx) {
			break runLoop
		}
	}

	mq.Disconnect(ctx)
	stopMonitor()

	if mq != nil {
		// wait until the mqtt connection is done; or two seconds
		select {
		case <-mq.Done():
		case <-time.After(2 * time.Second):
		}
	}

	return nil

}

func run(ctx context.Context, api apiv2connect.MonitorServiceClient, cfgStore ConfigStore) (bool, error) {

	log := logger.FromContext(ctx)

	ctx, span := tracing.Start(ctx, "monitor-run")
	defer span.End()

	log.InfoContext(ctx, "monitor-run execution")

	serverresp, err := api.GetServers(ctx, connect.NewRequest(&apiv2.GetServersRequest{}))
	if err != nil {
		if twerr, ok := err.(twirp.Error); ok {
			if twerr.Code() == twirp.PermissionDenied {
				return false, fmt.Errorf("getting server list: %s", twerr.Msg())
			}
		}
		return false, fmt.Errorf("getting server list: %s", err)
	}

	serverlist := serverresp.Msg

	if serverlist.Config != nil {
		cfgStore.SetConfigFromApi(serverlist.Config)
	}

	if len(serverlist.Servers) == 0 {
		// no error, just no servers
		return false, nil
	}

	batchID := ulid.ULID{}
	batchID.UnmarshalText(serverlist.BatchId)

	log = log.With("batchID", batchID.String())

	span.SetAttributes(attribute.String("batchID", batchID.String()))

	log.Debug("processing", "server_count", len(serverlist.Servers))

	// we're testing, so limit how much work ...
	if *onceFlag {
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

	log.Info("submitting")

	list := &apiv2.SubmitResultsRequest{
		Version: 4,
		List:    statuses,
		BatchId: serverlist.BatchId,
	}

	r, err := api.SubmitResults(ctx, connect.NewRequest(list))
	if err != nil {
		return false, fmt.Errorf("SubmitResults: %s", err)
	}
	if !r.Msg.Ok {
		return false, fmt.Errorf("SubmitResults not okay")
	}

	return true, nil

}

func checkDone(ctx context.Context) bool {
	select {
	case <-ctx.Done():
		return true
	default:
		return false
	}
}
