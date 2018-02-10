package main

import (
	"encoding/binary"
	"flag"
	"fmt"
	"log"
	"net"
	"os"
	"strings"
	"time"

	"github.com/beevik/ntp"
	"github.com/ntppool/monitor"
)

// Todo:
//   - Check status.Leap != ntp.LeapNotInSync

const sleepTime = 120

var (
	onceFlag       = flag.Bool("once", false, "Only run once instead of forever")
	sanityOnlyFlag = flag.Bool("sanity-only", false, "Only run the local sanity check")
)

func main() {
	flag.Parse()
	args := flag.Args()
	log.Printf("OS ARGS: %q", args)
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

		if !localOK.Check() {
			log.Printf("Local clock might not be okay, sleeping 90 seconds")
			time.Sleep(90 * time.Second)
			continue
		}

		if !run(api) {
			log.Printf("Got no work, sleeping.")
			time.Sleep(sleepTime * time.Second)
			continue
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

	log.Printf("CONFIG: %+v", serverlist.Config)

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

		status, err := CheckHost(s, serverlist.Config)
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

func ntpResponseToStatus(resp *ntp.Response) *monitor.ServerStatus {
	status := &monitor.ServerStatus{
		TS:         time.Now(),
		Offset:     resp.ClockOffset,
		Stratum:    resp.Stratum,
		Leap:       uint8(resp.Leap),
		RTT:        resp.RTT,
		NoResponse: false,
	}
	return status
}

func referenceIDString(refid uint32) string {
	b := make([]byte, 4)
	binary.BigEndian.PutUint32(b[0:], uint32(refid))
	return string(b)
}

func CheckHost(ip *net.IP, cfg *monitor.Config) (*monitor.ServerStatus, error) {

	if cfg.Samples == 0 {
		cfg.Samples = 1
	}

	opts := ntp.QueryOptions{
		Timeout: 3 * time.Second,
	}

	if cfg.IP != nil {
		opts.LocalAddress = cfg.IP.String()
	}

	statuses := []*monitor.ServerStatus{}

	for i := 0; i < cfg.Samples; i++ {

		if i > 0 {
			// minimum headway time is 2 seconds, https://www.eecis.udel.edu/~mills/ntp/html/rate.html
			time.Sleep(2 * time.Second)
		}

		// why lookup the IP here, just to get it deterministic? Log it?
		// maybe CheckHost should require an IP and checkLocal does the IP
		// lookup? (It'd need to have the monitor.cfg, too..)
		// ips, err := net.LookupIP(host)
		// if err != nil {
		// 	return nil, err
		// }

		resp, err := ntp.QueryWithOptions(ip.String(), opts)
		if err != nil {
			return nil, err
		}

		status := ntpResponseToStatus(resp)
		status.Server = ip

		// log.Printf("Query %d for %q: RTT: %s, Offset: %s", i, host, resp.RTT, resp.ClockOffset)

		if resp.Stratum == 0 || resp.Stratum == 16 {
			if len(resp.KissCode) > 0 {
				return status, fmt.Errorf("%s", resp.KissCode)
			}

			return status,
				fmt.Errorf("bad stratum %d (referenceID: %#x, %s)",
					resp.Stratum, resp.ReferenceID, referenceIDString(resp.ReferenceID))
		}

		if resp.Stratum > 6 {
			return status, fmt.Errorf("bad stratum %d", resp.Stratum)
		}

		statuses = append(statuses, status)
	}

	var best *monitor.ServerStatus

	for _, status := range statuses {

		if best == nil {
			best = status
			continue
		}

		if status.RTT < best.RTT {
			best = status
		}
	}

	// log.Printf("Got good response %q", best)

	return best, nil
}
