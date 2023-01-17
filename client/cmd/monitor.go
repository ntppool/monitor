package cmd

import (
	"context"
	"errors"
	"flag"
	"fmt"
	"log"
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
	"google.golang.org/protobuf/types/known/timestamppb"

	"go.ntppool.org/pingtrace/traceroute"

	"go.ntppool.org/monitor/api"
	"go.ntppool.org/monitor/api/pb"
	"go.ntppool.org/monitor/client/auth"
	"go.ntppool.org/monitor/client/localok"
	"go.ntppool.org/monitor/client/monitor"
	"go.ntppool.org/monitor/mqttcm"
)

// Todo:
//   - Check status.Leap != ntp.LeapNotInSync

var (
	onceFlag       = flag.Bool("once", false, "Only run once instead of forever")
	sanityOnlyFlag = flag.Bool("sanity-only", false, "Only run the local sanity check")
)

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

	cauth, err := cli.ClientAuth(ctx)
	if err != nil {
		log.Fatalf("auth error: %s", err)
	}

	deployEnv, err := api.GetDeploymentEnvironmentFromName(cauth.Name)
	if err != nil {
		log.Printf("error: %s", err)
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

			log.Printf("Authentication error: %s -- Go to %s to rotate and download new a API secret", aerr, url)
			os.Exit(2)
		}

		log.Printf("Could not authenticate: %s", err)
		os.Exit(2)
	}

	err = cauth.LoadOrIssueCertificates()
	if err != nil {
		log.Fatalf("LoadOrIssueCertificates: %s", err)
	}

	go cauth.Manager()

	// block until we have a valid certificate
	err = cauth.WaitUntilReady()
	if err != nil {
		log.Printf("Failed waiting for authentication to be ready: %s", err)
		os.Exit(2)
	}

	ctx, api, err := api.Client(ctx, cli.Config.Name, cauth)
	if err != nil {
		log.Fatalf("Could not setup API: %s", err)
	}

	cfg, err := api.GetConfig(ctx, &pb.GetConfigParams{})
	if err != nil {
		if twerr, ok := err.(twirp.Error); ok {
			if twerr.Code() == twirp.PermissionDenied {
				log.Fatalf("could not get config: %s", twerr.Msg())
			}
		}
		log.Fatalf("could not get config: %s", err)
	}

	var mq *autopaho.ConnectionManager
	topics := mqttcm.NewTopics(deployEnv)
	statusChannel := topics.Status(cauth.Name)

	if cfg.MQTTConfig != nil && len(cfg.MQTTConfig.Host) > 0 {

		router := paho.NewSingleHandlerRouter(func(m *paho.Publish) {
			log.Printf("mqtt client message on %q: %s", m.Topic, m.Payload)
		})

		// todo: once a day get a new mqtt config / JWT

		mq, err = mqttcm.Setup(
			ctx, cauth.Name, statusChannel,
			[]string{
				topics.RequestSubscription(cauth.Name),
			}, router, cfg.MQTTConfig, cauth,
		)
		if err != nil {
			log.Fatalf("mqtt: %s", err)
		}

		err := mq.AwaitConnection(ctx)
		if err != nil {
			log.Fatalf("mqtt connection error: %s", err)
		}
	}

	localOK := localok.NewLocalOK(cfg)

	if *sanityOnlyFlag {
		ok := localOK.Check()
		if ok {
			log.Printf("Local clock ok")
			return nil
		} else {
			log.Printf("Local clock not ok")
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
		boff.MaxInterval = 120 * time.Second
		boff.MaxElapsedTime = 0

		err := backoff.Retry(func() error {

			if !localOK.Check() {
				wait := localOK.NextCheckIn()
				log.Printf("Local clock might not be okay, waiting %s", wait.Round(1*time.Second).String())
				time.Sleep(wait) // todo: ctx
				return fmt.Errorf("local clock")
			}

			if ok, err := run(api); !ok || err != nil {
				if err != nil {
					log.Println(err)
					boff.MaxInterval = 20 * time.Minute
					boff.Multiplier = 5
				}
				// log.Printf("Got no work, sleeping.")
				return fmt.Errorf("no work")
			}
			boff.Reset()
			return nil
		}, boff)

		if err != nil {
			log.Printf("backoff error: %s", err)
		}

		i++
		if i > 0 && *onceFlag {
			log.Printf("Asked to only run once, so bye now.")
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

func run(api pb.Monitor) (bool, error) {

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

	if len(serverlist.Servers) == 0 {
		// no error, just no servers
		return false, nil
	}

	batchID := ulid.ULID{}
	batchID.UnmarshalText(serverlist.BatchID)

	log.Printf("Batch %s - Servers: %d", batchID.String(), len(serverlist.Servers))

	// we're testing, so limit how much work ...
	if *onceFlag {
		if len(serverlist.Servers) > 10 {
			serverlist.Servers = serverlist.Servers[0:9]
		}
	}

	statuses := []*pb.ServerStatus{}

	wg := sync.WaitGroup{}

	for _, s := range serverlist.Servers {

		wg.Add(1)

		go func(s *netip.Addr, trace bool, ticket []byte) {

			if trace {
				// todo: get psuedo lock from channel to manage parallel traceroutes

				tr, err := traceroute.New(*s)
				if err != nil {
					log.Printf("traceroute: %s", err)
				}
				tr.Start(ctx)
				x, err := tr.ReadAll()
				if err != nil {
					log.Printf("traceroute: %s", err)
				}

				log.Printf("traceroute: %+v", x)

				wg.Done()
				return
			}

			status, err := monitor.CheckHost(s, serverlist.Config)
			if status == nil {
				status = &pb.ServerStatus{
					NoResponse: true,
				}
				status.SetIP(s)
			}
			status.Ticket = ticket
			if err != nil {
				log.Printf("Error checking %q: %s", s, err)
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
			statuses = append(statuses, status)
			wg.Done()
		}(s.IP(), s.Trace, s.Ticket)
	}

	wg.Wait()

	log.Printf("Submitting %s", serverlist.BatchID)

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
