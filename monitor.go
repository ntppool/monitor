package monitor

import (
	"encoding/binary"
	"fmt"
	"log"
	"time"
	"unicode/utf8"

	"github.com/beevik/ntp"
	"go.ntppool.org/monitor/api/pb"
	"google.golang.org/protobuf/types/known/durationpb"
	"google.golang.org/protobuf/types/known/timestamppb"
	"inet.af/netaddr"
)

// CheckHost runs the configured queries to the IP and returns one ServerStatus
func CheckHost(ip *netaddr.IP, cfg *pb.Config) (*pb.ServerStatus, error) {

	if cfg.Samples == 0 {
		cfg.Samples = 3
	}

	opts := ntp.QueryOptions{
		Timeout: 3 * time.Second,
	}

	configIP := cfg.GetIP()
	if configIP != nil && configIP.IsValid() {
		opts.LocalAddress = configIP.String()
		if natIP := cfg.GetNatIP(); natIP != nil && natIP.IsValid() {
			opts.LocalAddress = natIP.String()
		}
	} else {
		log.Printf("Did not get valid local configuration IP: %+v", configIP)
	}

	statuses := []*pb.ServerStatus{}

	for i := int32(0); i < cfg.Samples; i++ {

		if i > 0 {
			// minimum headway time is 2 seconds, https://www.eecis.udel.edu/~mills/ntp/html/rate.html
			time.Sleep(2 * time.Second)
		}

		resp, err := ntp.QueryWithOptions(ip.String(), opts)
		if err != nil {
			status := &pb.ServerStatus{}
			status.SetIP(ip)
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

			refText := fmt.Sprintf("%#x", resp.ReferenceID)

			refIDStr := referenceIDString(resp.ReferenceID)
			if utf8.Valid([]byte(refIDStr)) {
				refText = refText + ", " + refIDStr
			}

			return status,
				fmt.Errorf("bad stratum %d (referenceID: %s)",
					resp.Stratum, refText)
		}

		if resp.Stratum > 6 {
			return status, fmt.Errorf("bad stratum %d", resp.Stratum)
		}

		statuses = append(statuses, status)
	}

	var best *pb.ServerStatus

	// log.Printf("for %s we collected %d samples, now find the best result", ip.String(), len(statuses))

	// todo: if there are more than 2 (3?) samples with an offset, throw
	// away the offset outlier(s)

	for _, status := range statuses {

		// log.Printf("status for %s / %d: offset: %s rtt: %s err: %q", ip.String(), i, status.Offset.AsDuration(), status.RTT.AsDuration(), status.Error)

		if best == nil {
			best = status
			continue
		}

		// todo: ... and it's otherwise a valid response

		if len(status.Error) == 0 && (len(best.Error) > 0 || (status.RTT.AsDuration() < best.RTT.AsDuration())) {
			best = status
		}
	}

	errLog := ""
	if len(best.Error) > 0 {
		errLog = fmt.Sprintf(" err: %q", best.Error)
	}
	log.Printf("best result for %s - offset: %s rtt: %s%s",
		ip.String(), best.Offset.AsDuration(), best.RTT.AsDuration(), errLog)

	if len(best.Error) > 0 {
		return best, fmt.Errorf("%s", best.Error)
	}

	return best, nil
}

func ntpResponseToStatus(ip *netaddr.IP, resp *ntp.Response) *pb.ServerStatus {
	//log.Printf("Leap: %d", resp.Leap)
	status := &pb.ServerStatus{
		TS:         timestamppb.Now(),
		Offset:     durationpb.New(resp.ClockOffset),
		Stratum:    int32(resp.Stratum),
		Leap:       int32(resp.Leap),
		RTT:        durationpb.New(resp.RTT),
		NoResponse: false,
	}
	status.SetIP(ip)
	return status
}

func referenceIDString(refid uint32) string {
	b := make([]byte, 4)
	binary.BigEndian.PutUint32(b[0:], uint32(refid))
	return string(b)
}
