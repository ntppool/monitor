package cmd

import (
	"context"
	"flag"
	"fmt"
	"log"
	"os"
	"strings"
	"sync"
	"time"

	"github.com/cenkalti/backoff"
	"github.com/oklog/ulid/v2"
	"github.com/spf13/cobra"
	"go.ntppool.org/monitor/api"
	"go.ntppool.org/monitor/api/pb"
	"go.ntppool.org/monitor/client/localok"
	"go.ntppool.org/monitor/client/monitor"
	"google.golang.org/protobuf/types/known/timestamppb"
	"inet.af/netaddr"
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

	// ignoring errors because the Manager will issue missing certs
	// we just want to load them from disk right away
	cauth.LoadCertificates(ctx)

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
		log.Fatalf("Could not get config: %s", err)
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

	i := 0

	for {

		boff := backoff.NewExponentialBackOff()
		boff.RandomizationFactor = 0.2
		boff.InitialInterval = 2 * time.Second
		boff.MaxInterval = 120 * time.Second
		boff.MaxElapsedTime = 60 * time.Minute

		err := backoff.Retry(func() error {

			if !localOK.Check() {
				wait := localOK.NextCheckIn()
				log.Printf("Local clock might not be okay, waiting %s", wait.Round(1*time.Second).String())
				time.Sleep(wait) // todo: ctx
				return fmt.Errorf("local clock")
			}

			if !run(api) {
				log.Printf("Got no work, sleeping.")
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

	return nil

}

func run(api pb.Monitor) bool {

	ctx := context.Background()

	serverlist, err := api.GetServers(ctx, &pb.GetServersParams{})

	if err != nil {
		log.Printf("getting server list: %s", err)
		return false
	}

	if len(serverlist.Servers) == 0 {
		return false
	}

	batchID := ulid.ULID{}
	batchID.UnmarshalText(serverlist.BatchID)

	log.Printf("Batch %s - Servers: %d, Config: %+v", batchID.String(), len(serverlist.Servers), serverlist.Config)

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

		go func(s *netaddr.IP, ticket []byte) {
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
		}(s.IP(), s.Ticket)
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
		log.Printf("SubmitResults error: %s", err)
		return false
	}
	if !r.Ok {
		log.Printf("SubmitResults did not return okay")
		return false
	}

	return true

}
