package main

import (
	"encoding/binary"
	"flag"
	"fmt"
	"log"
	"net"
	"os"
	"time"

	"github.com/beevik/ntp"
	"github.com/ntppool/monitor"
)

// Todo:
//   - Check status.Leap != ntp.LeapNotInSync

const SLEEP_TIME = 120

var (
	onceFlag = flag.Bool("once", false, "Only run once instead of forever")
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

	localOK := LocalOK{}

	i := 0

	for {

		if !localOK.Check() {
			log.Printf("Local clock might not be okay, sleeping 90 seconds")
			time.Sleep(90 * time.Second)
			continue
		}

		if !run(api) {
			log.Printf("Got no work, sleeping.")
			time.Sleep(SLEEP_TIME * time.Second)
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

	servers, err := api.GetServerList()
	if err != nil {
		log.Printf("getting server list: %s", err)
		return false
	}

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

		status, err := CheckHost(s.String(), 2)
		if err != nil {
			log.Printf("Error checking %q: %s", s, err)
		}
		if status == nil {
			status = &monitor.ServerStatus{Server: s, NoResponse: true}
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

func CheckHost(host string, samples int) (*monitor.ServerStatus, error) {

	statuses := []*monitor.ServerStatus{}

	for i := 0; i < 4; i++ {

		ips, err := net.LookupIP(host)
		if err != nil {
			return nil, err
		}

		resp, err := ntp.Query(ips[0].String())
		if err != nil {
			return nil, err
		}

		status := ntpResponseToStatus(resp)
		status.Server = ips[0]

		// log.Printf("Query %d for %q: RTT: %s, Offset: %s", i, host, resp.RTT, resp.ClockOffset)

		if resp.Stratum == 0 || resp.Stratum == 16 {
			// todo: interpret reference ID
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
