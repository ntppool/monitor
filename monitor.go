package monitor

import (
	"encoding/binary"
	"fmt"
	"net"
	"time"

	"github.com/beevik/ntp"
)

// VERSION is the current version of the software
const VERSION = "2.2"

// CheckHost runs the configured queries to the IP and returns one ServerStatus
func CheckHost(ip *net.IP, cfg *Config) (*ServerStatus, error) {

	if cfg.Samples == 0 {
		cfg.Samples = 3
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

		resp, err := ntp.QueryWithOptions(ip.String(), opts)
		if err != nil {
			status := &ServerStatus{Server: ip}
			if resp != nil {
				status = ntpResponseToStatus(ip, resp)
			}
			status.Error = err.Error()
			statuses = append(statuses, status)
			continue
		}

		status := ntpResponseToStatus(ip, resp)

		// log.Printf("Query %d for %q: RTT: %s, Offset: %s", i, host, resp.RTT, resp.ClockOffset)

		// if we get an explicit bad response in any of the samples, we error out
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

	if len(best.Error) > 0 {
		return best, fmt.Errorf("%s", best.Error)
	}

	return best, nil
}

func ntpResponseToStatus(ip *net.IP, resp *ntp.Response) *ServerStatus {
	status := &ServerStatus{
		TS:         time.Now(),
		Offset:     resp.ClockOffset,
		Stratum:    resp.Stratum,
		Leap:       uint8(resp.Leap),
		RTT:        resp.RTT,
		NoResponse: false,
	}
	status.Server = ip
	return status
}

func referenceIDString(refid uint32) string {
	b := make([]byte, 4)
	binary.BigEndian.PutUint32(b[0:], uint32(refid))
	return string(b)
}
