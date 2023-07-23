package monitor

import (
	"encoding/binary"
	"fmt"
	"net/netip"
	"time"
	"unicode/utf8"

	"github.com/beevik/ntp"
	"go.ntppool.org/common/logger"
	"go.ntppool.org/monitor/api/pb"
	"google.golang.org/protobuf/types/known/durationpb"
	"google.golang.org/protobuf/types/known/timestamppb"
)

type response struct {
	Response *ntp.Response
	Status   *pb.ServerStatus
	Error    error
}

// CheckHost runs the configured queries to the IP and returns one ServerStatus
func CheckHost(ip *netip.Addr, cfg *pb.Config) (*pb.ServerStatus, *ntp.Response, error) {

	log := logger.Setup()

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
		log.Error("Did not get valid local configuration IP", "configIP", configIP)
	}

	if ip.IsLoopback() {
		return nil, nil, fmt.Errorf("loopback address")
	}
	if ip.IsPrivate() {
		return nil, nil, fmt.Errorf("private address")
	}
	if ip.IsMulticast() {
		return nil, nil, fmt.Errorf("multicast address")
	}
	if !ip.IsValid() {
		return nil, nil, fmt.Errorf("invalid IP")
	}

	responses := []*response{}

	for i := int32(0); i < cfg.Samples; i++ {

		if i > 0 {
			// minimum headway time is 2 seconds, https://www.eecis.udel.edu/~mills/ntp/html/rate.html
			time.Sleep(2 * time.Second)
		}

		resp, err := ntp.QueryWithOptions(ip.String(), opts)
		if err != nil {
			r := &response{
				Status: &pb.ServerStatus{},
			}
			r.Status.SetIP(ip)
			if resp != nil {
				r.Response = resp
				r.Status = ntpResponseToStatus(ip, resp)
			}
			r.Error = err
			responses = append(responses, r)

			log.Debug("ntp query error", "host", ip.String(), "iteration", i, "error", err)

			continue
		}

		status := ntpResponseToStatus(ip, resp)

		log.Debug("ntp query", "host", ip.String(), "iteration", i, "rtt", resp.RTT.String(), "offset", resp.ClockOffset, "error", err)

		// if we get an explicit bad response in any of the samples, we error out
		if resp.Stratum == 0 || resp.Stratum == 16 {
			if len(resp.KissCode) > 0 {
				return status, resp, fmt.Errorf("%s", resp.KissCode)
			}

			refText := fmt.Sprintf("%#x", resp.ReferenceID)

			refIDStr := referenceIDString(resp.ReferenceID)
			if utf8.Valid([]byte(refIDStr)) {
				refText = refText + ", " + refIDStr
			}

			return status, resp,
				fmt.Errorf("bad stratum %d (referenceID: %s)",
					resp.Stratum, refText)
		}

		if resp.Stratum > 6 {
			return status, resp, fmt.Errorf("bad stratum %d", resp.Stratum)
		}

		responses = append(responses, &response{
			Status:   status,
			Response: resp,
		})
	}

	var best *response

	// log.Printf("for %s we collected %d samples, now find the best result", ip.String(), len(statuses))

	// todo: if there are more than 2 (3?) samples with an offset, throw
	// away the offset outlier(s)

	for _, r := range responses {

		// log.Printf("status for %s / %d: offset: %s rtt: %s err: %q", ip.String(), i, status.Offset.AsDuration(), status.RTT.AsDuration(), status.Error)

		if best == nil {
			best = r
			continue
		}

		// todo: ... and it's otherwise a valid response?
		if (r.Error == nil && best.Error != nil) || (r.Status.RTT.AsDuration() < best.Status.RTT.AsDuration()) {
			best = r
		}
	}

	// errLog := ""
	// if len(best.Error) > 0 {
	// 	errLog = fmt.Sprintf(" err: %q", best.Error)
	// }
	// log.Printf("best result for %s - offset: %s rtt: %s%s",
	// 	ip.String(), best.Offset.AsDuration(), best.RTT.AsDuration(), errLog)

	if best.Error != nil {
		return best.Status, best.Response, fmt.Errorf("%s", best.Error)
	}

	return best.Status, best.Response, nil
}

func ntpResponseToStatus(ip *netip.Addr, resp *ntp.Response) *pb.ServerStatus {
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
