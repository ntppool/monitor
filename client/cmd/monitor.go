package cmd

import (
	"context"
	"errors"
	"flag"
	"fmt"
	"net/netip"
	"os"
	"strings"
	"sync"
	"time"

	"github.com/cenkalti/backoff"
	"github.com/eclipse/paho.golang/autopaho"
	"github.com/eclipse/paho.golang/paho"
	"github.com/oklog/ulid/v2"
	"github.com/spf13/cobra"
	"github.com/twitchtv/twirp"
	"golang.org/x/exp/slog"
	"google.golang.org/protobuf/types/known/timestamppb"

	"go.ntppool.org/pingtrace/traceroute"

	"go.ntppool.org/monitor/api"
	"go.ntppool.org/monitor/api/pb"
	"go.ntppool.org/monitor/client/auth"
	"go.ntppool.org/monitor/client/localok"
	"go.ntppool.org/monitor/client/monitor"
	"go.ntppool.org/monitor/logger"
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
	SetConfig(cfg *pb.Config)
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
	ctx, cancelMonitor := context.WithCancel(context.Background())
	defer cancelMonitor()

	log := logger.Setup()
	ctx = logger.NewContext(ctx, log)

	cauth, err := cli.ClientAuth(ctx)
	if err != nil {
		return fmt.Errorf("auth: %w", err)
	}

	deployEnv, err := api.GetDeploymentEnvironmentFromName(cauth.Name)
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

	go cauth.Manager()

	// block until we have a valid certificate
	err = cauth.WaitUntilReady()
	if err != nil {
		log.Error("failed waiting for authentication to be ready", "err", err)
		os.Exit(2)
	}

	ctx, api, err := api.Client(ctx, cli.Config.Name, cauth)
	if err != nil {
		return fmt.Errorf("could not setup API: %w", err)
	}

	cfg, err := api.GetConfig(ctx, &pb.GetConfigParams{})
	if err != nil {
		if twerr, ok := err.(twirp.Error); ok {
			if twerr.Code() == twirp.PermissionDenied {
				return fmt.Errorf("could not get config: %w", twerr)
			}
		}
		return fmt.Errorf("could not get config: %s", err)
	}

	var mq *autopaho.ConnectionManager
	topics := mqttcm.NewTopics(deployEnv)
	statusChannel := topics.Status(cauth.Name)

	if cfg.MQTTConfig != nil && len(cfg.MQTTConfig.Host) > 0 {

		log := log.WithGroup("mqtt")

		mqc := monitor.NewMQClient(log, topics, cfg)
		router := paho.NewSingleHandlerRouter(mqc.Handler)

		// todo: once a day get a new mqtt config / JWT

		mq, err = mqttcm.Setup(
			ctx, cauth.Name, statusChannel,
			[]string{
				topics.RequestSubscription(cauth.Name),
			}, router, cfg.MQTTConfig, cauth,
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

	localOK := localok.NewLocalOK(cfg)

	if *sanityOnlyFlag {
		ok := localOK.Check()
		if ok {
			log.Info("Local clock ok")
			return nil
		} else {
			log.Info("Local clock not ok")
			return fmt.Errorf("health check failed")
		}
	}

	if mq != nil {
		msg, _ := mqttcm.StatusMessageJSON(true)
		mq.Publish(ctx, &paho.Publish{
			Topic:   statusChannel,
			Payload: msg,
			QoS:     1,
			Retain:  true,
		})

		// old, clear retained message
		// oldChannel := fmt.Sprintf("%s/status/%s/online", cfg.MQTTConfig.Prefix, cauth.Name)
		// for _, qos := range []byte{0, 1, 2} {
		// 	mq.Publish(ctx, &paho.Publish{
		// 		Topic:   oldChannel,
		// 		Payload: []byte{},
		// 		QoS:     qos,
		// 		Retain:  true,
		// 	})
		// }
	}

	i := 0

	for {
		boff := backoff.NewExponentialBackOff()
		boff.RandomizationFactor = 0.3
		boff.InitialInterval = 3 * time.Second
		boff.MaxInterval = MaxInterval
		boff.MaxElapsedTime = 0

		err := backoff.Retry(func() error {

			if !localOK.Check() {
				wait := localOK.NextCheckIn()
				log.Info("local clock might not be okay", "waiting", wait.Round(1*time.Second).String())
				time.Sleep(wait) // todo: ctx
				return fmt.Errorf("local clock")
			}

			if ok, err := run(api, localOK); !ok || err != nil {
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
	}

	mq.Disconnect(ctx)

	cancelMonitor()

	if mq != nil {
		// wait until the mqtt connection is done; or two seconds
		select {
		case <-mq.Done():
		case <-time.After(2 * time.Second):
		}
	}

	return nil

}

func run(api pb.Monitor, cfgStore SetConfig) (bool, error) {

	ctx := context.Background()

	serverlist, err := api.GetServers(ctx, &pb.GetServersParams{})
	if err != nil {
		if twerr, ok := err.(twirp.Error); ok {
			if twerr.Code() == twirp.PermissionDenied {
				return false, fmt.Errorf("getting server list: %s", twerr.Msg())
			}
		}
		return false, fmt.Errorf("getting server list: %s", err)
	}

	if serverlist.Config != nil {
		cfgStore.SetConfig(serverlist.Config)
	}

	if len(serverlist.Servers) == 0 {
		// no error, just no servers
		return false, nil
	}

	batchID := ulid.ULID{}
	batchID.UnmarshalText(serverlist.BatchID)

	log := slog.With("batchID", batchID.String())

	log.Debug("processing", "server_count", len(serverlist.Servers))

	// we're testing, so limit how much work ...
	if *onceFlag {
		if len(serverlist.Servers) > 10 {
			serverlist.Servers = serverlist.Servers[0:9]
		}
	}

	statuses := []*pb.ServerStatus{}

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

			status, _, err := monitor.CheckHost(s, serverlist.Config)
			if status == nil {
				status = &pb.ServerStatus{
					NoResponse: true,
				}
				status.SetIP(s)
			}
			status.Ticket = ticket
			if err != nil {
				log.Info("ntp error", "server", s, "err", err)
				status.Error = err.Error()
				if strings.HasPrefix(status.Error, "read udp") {
					idx := strings.LastIndex(status.Error, ":")
					// ": " == two characters
					if len(status.Error) > idx+2 {
						idx = idx + 2
					}
					status.Error = status.Error[idx:]
				}
			}
			status.TS = timestamppb.Now()

			mu.Lock()
			defer mu.Unlock()
			statuses = append(statuses, status)
			wg.Done()
		}(s.IP(), s.Trace, s.Ticket)
	}

	wg.Wait()

	log.Info("submitting")

	list := &pb.ServerStatusList{
		Version: 3,
		List:    statuses,
		BatchID: serverlist.BatchID,
	}

	r, err := api.SubmitResults(ctx, list)
	if err != nil {
		return false, fmt.Errorf("SubmitResults: %s", err)
	}
	if !r.Ok {
		return false, fmt.Errorf("SubmitResults not okay")
	}

	return true, nil

}
