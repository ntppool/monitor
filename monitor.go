package monitor

import (
	"encoding/binary"
	"fmt"
	"net"
	"time"

	"github.com/beevik/ntp"
)

func CheckHost(ip *net.IP, cfg *Config) (*ServerStatus, error) {

	if cfg.Samples == 0 {
		cfg.Samples = 1
	}

	opts := ntp.QueryOptions{
		Timeout: 3 * time.Second,
	}

	if cfg.IP != nil {
		opts.LocalAddress = cfg.IP.String()
	}

	statuses := []*ServerStatus{}

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
			// todo: add error to status and continue instead of returning
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

	var best *ServerStatus

	for _, status := range statuses {

		if best == nil {
			best = status
			continue
		}

		// todo: ... and it's otherwise a valid response
		if status.RTT < best.RTT {
			best = status
		}
	}

	// todo: if no good responses, return the error from the last sample

	// log.Printf("Got good response %q", best)

	return best, nil
}

func ntpResponseToStatus(resp *ntp.Response) *ServerStatus {
	status := &ServerStatus{
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
