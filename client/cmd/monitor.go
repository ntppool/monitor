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
	"github.com/spf13/cobra"
	"go.ntppool.org/monitor"
	"go.ntppool.org/monitor/api"
	"go.ntppool.org/monitor/api/pb"
	"go.ntppool.org/monitor/client/localok"
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

	go cauth.Manager()

	// block until we have a valid certificate
	err = cauth.WaitUntilReady()
	if err != nil {
		log.Printf("Failed waiting for authentication to be ready: %s", err)
		os.Exit(2)
	}

	api, err := api.Client(ctx, cli.Config.Name, cauth)
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
		boff.InitialInterval = 5 * time.Second
		boff.MaxInterval = 90 * time.Second

		err := backoff.Retry(func() error {

			if !localOK.Check() {
				log.Printf("Local clock might not be okay, waiting a bit")
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

	servers, err := serverlist.IPs()
	if err != nil {
		log.Printf("bad IPs in server list: %s", err)
		return false
	}

	log.Printf("Servers: %d, Config: %+v", len(servers), serverlist.Config)

	if len(servers) == 0 {
		return false
	}

	// we're testing, so limit how much work ...
	if *onceFlag {
		if len(servers) > 10 {
			servers = servers[0:9]
		}
	}

	statuses := []*pb.ServerStatus{}

	wg := sync.WaitGroup{}

	for _, ip := range servers {

		wg.Add(1)

		go func(s *netaddr.IP) {
			status, err := monitor.CheckHost(s, serverlist.Config)
			if status == nil {
				status = &pb.ServerStatus{
					NoResponse: true,
				}
				status.SetIP(s)
			}
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
		}(ip)
	}

	wg.Wait()

	list := &pb.ServerStatusList{
		Version: 2,
		List:    statuses,
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
