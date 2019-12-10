package main

import (
	"flag"
	"fmt"
	"log"
	"os"
	"strings"
	"time"

	"github.com/cenkalti/backoff"
	"go.ntppool.org/monitor"
)

// Todo:
//   - Check status.Leap != ntp.LeapNotInSync

var (
	onceFlag       = flag.Bool("once", false, "Only run once instead of forever")
	sanityOnlyFlag = flag.Bool("sanity-only", false, "Only run the local sanity check")
)

func main() {
	flag.Parse()
	args := flag.Args()
	if len(args) < 1 {
		exe, err := os.Executable()
		if err != nil {
			log.Printf("Could not get executable name: %s", err)
		}
		fmt.Printf("%s [api_key] [pool-server]\n", exe)
		os.Exit(2)
	}

	apiKey := args[0]
	apiURLStr := "http://www.pool.ntp.org/monitor"
	if len(args) > 1 {
		apiURLStr = args[1]
	}

	api, err := monitor.NewAPI(apiURLStr, apiKey)
	if err != nil {
		log.Fatalf("creating API: %s", err)
	}

	cfg, err := api.GetConfig()
	if err != nil {
		log.Fatalf("Could not get config: %s", err)
	}

	log.Printf("Config: %+v", cfg)

	localOK := NewLocalOK(cfg)

	if *sanityOnlyFlag {
		ok := localOK.Check()
		if ok {
			log.Printf("Local clock ok")
			os.Exit(0)
		} else {
			log.Printf("Local clock not ok")
			os.Exit(2)
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

}

func run(api *monitor.API) bool {

	serverlist, err := api.GetServerList()

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

	statuses := []*monitor.ServerStatus{}

	for _, s := range servers {

		status, err := monitor.CheckHost(s, serverlist.Config)
		if status == nil {
			status = &monitor.ServerStatus{
				Server:     s,
				NoResponse: true,
			}
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
		status.TS = time.Now()
		statuses = append(statuses, status)
	}

	err = api.PostStatuses(statuses)
	if err != nil {
		log.Printf("Post status error: %s", err)
		return false
	}

	return true

}
