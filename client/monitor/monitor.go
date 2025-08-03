package monitor

import (
	"context"
	"encoding/binary"
	"fmt"
	"net"
	"net/netip"
	"os"
	"sync"
	"time"
	"unicode/utf8"

	"github.com/beevik/ntp"
	"go.opentelemetry.io/otel/attribute"
	"go.opentelemetry.io/otel/metric"
	"go.opentelemetry.io/otel/trace"
	"google.golang.org/protobuf/types/known/durationpb"
	"google.golang.org/protobuf/types/known/timestamppb"

	"go.ntppool.org/common/logger"
	"go.ntppool.org/common/tracing"
	"go.ntppool.org/monitor/client/config/checkconfig"
	"go.ntppool.org/monitor/client/metrics"
	apiv2 "go.ntppool.org/monitor/gen/monitor/v2"
)

var (
	debugNTPQueries     bool
	debugNTPQueriesOnce sync.Once
)

func isNTPQueryDebugEnabled() bool {
	debugNTPQueriesOnce.Do(func() {
		debugNTPQueries = os.Getenv("MONITOR_DEBUG_NTP_QUERIES") == "true"
	})
	return debugNTPQueries
}

type response struct {
	Response *ntp.Response
	Status   *apiv2.ServerStatus
	Packet   *apiv2.NTPPacket
	Error    error
}

// CheckHost runs the configured queries to the IP and returns one ServerStatus
func CheckHost(ctx context.Context, ip *netip.Addr, cfg *checkconfig.Config, traceAttributes ...attribute.KeyValue) (*apiv2.ServerStatus, *ntp.Response, error) {
	log := logger.FromContext(ctx)

	ipVersion := "ipv4"
	if ip.Is6() {
		ipVersion = "ipv6"
	}

	// Increment servers checked counter
	if metrics.ServersChecked != nil {
		metrics.ServersChecked.Add(ctx, 1, metric.WithAttributes(attribute.String("ip_version", ipVersion)))
	}

	traceAttributes = append(traceAttributes,
		attribute.String("ip", ip.String()),
		attribute.String("ip_version", ipVersion),
	)

	ctx, span := tracing.Start(ctx,
		"monitor.CheckHost",
		trace.WithAttributes(traceAttributes...),
	)
	defer span.End()

	if cfg.Samples == 0 {
		cfg.Samples = 3
	}

	span.SetAttributes(attribute.Int("samples", int(cfg.Samples)))

	var localAddress string

	configIP := cfg.IP
	if configIP != nil && configIP.IsValid() {
		localAddress = configIP.String()
		if natIP := cfg.IPNat; natIP != nil && natIP.IsValid() {
			localAddress = natIP.String()
		}
	} else {
		log.Error("Did not get valid local configuration IP", "configIP", configIP)
	}

	var err error
	if ip.IsLoopback() {
		err = fmt.Errorf("loopback address")
	}
	if ip.IsPrivate() {
		err = fmt.Errorf("private address")
	}
	if ip.IsMulticast() {
		err = fmt.Errorf("multicast address")
	}
	if !ip.IsValid() {
		err = fmt.Errorf("invalid IP")
	}
	if err != nil {
		span.RecordError(err)
		return nil, nil, err
	}

	ntpCaptureBuffer := NewCaptureBuffer(ip, configIP)
	responses := []*response{}

	opts := ntp.QueryOptions{
		Timeout:      3 * time.Second,
		Extensions:   []ntp.Extension{ntpCaptureBuffer},
		LocalAddress: localAddress,
	}

	for i := int32(0); i < cfg.Samples; i++ {

		if i > 0 {
			// minimum headway time is 2 seconds, https://www.eecis.udel.edu/~mills/ntp/html/rate.html
			time.Sleep(2 * time.Second)
		}

		ipStr := ip.String()
		if ip.Is6() {
			ipStr = "[" + ipStr + "]:123"
		}

		resp, err := ntp.QueryWithOptions(ipStr, opts)

		// Increment NTP queries sent counter
		if metrics.NTPQueriesSent != nil {
			metrics.NTPQueriesSent.Add(ctx, 1, metric.WithAttributes(attribute.String("ip_version", ipVersion)))
		}

		if err != nil {
			r := &response{
				Status: &apiv2.ServerStatus{
					NoResponse: true,
				},
			}
			r.Status.SetIP(ip)
			if resp != nil {
				r.Response = resp
				r.Status = ntpResponseToApiStatus(ip, resp)
				// ignore the offset if there also was an error
				r.Status.Offset = nil
			}

			if netErr, ok := err.(*net.OpError); ok {
				// drop the protocol and addresses
				r.Error = fmt.Errorf("network: %w", netErr.Err)
			} else {
				r.Error = err
			}
			r.Status.Error = r.Error.Error()

			responses = append(responses, r)

			// span.RecordError(err) // errors are expected, so don't consider them such
			if isNTPQueryDebugEnabled() {
				log.DebugContext(ctx, "ntp query error", "host", ip.String(), "iteration", i, "error", err)
			}

			continue
		}

		status := ntpResponseToApiStatus(ip, resp)

		if isNTPQueryDebugEnabled() {
			log.DebugContext(ctx, "ntp query", "host", ip.String(), "iteration", i, "rtt", resp.RTT.String(), "offset", resp.ClockOffset, "error", err)
		}

		// if we get an explicit bad response in any of the samples, we error out
		if resp.Stratum == 0 || resp.Stratum == 16 {
			if len(resp.KissCode) > 0 {
				if resp.KissCode == "RATE" {
					status.Offset = nil
				}
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

		if resp.Stratum > 10 {
			return status, resp, fmt.Errorf("bad stratum %d", resp.Stratum)
		}

		packets := ntpCaptureBuffer.ResponsePackets()
		if len(packets) > 1 {
			log.WarnContext(ctx, "got more than one packet for a response")
		}
		var packet *apiv2.NTPPacket
		if len(packets) > 0 {
			packet = packets[0]
		}

		ntpCaptureBuffer.Clear()

		responses = append(responses, &response{
			Status:   status,
			Response: resp,
			Packet:   packet,
		})
	}

	var best *response

	// log.Debug("collection done, now find the best result", "ip", ip.String(), "count", len(responses))

	// todo: if there are more than 2 (3?) samples with an offset, throw
	// away the offset outlier(s)

	for _, r := range responses {

		// log.Printf("status for %s / %d: offset: %s rtt: %s err: %q", ip.String(), i, status.Offset.AsDuration(), status.RTT.AsDuration(), status.Error)

		if best == nil {
			best = r
			continue
		}

		// Priority 1: Always prefer responses without errors
		if r.Error == nil && best.Error != nil {
			best = r
			continue
		}

		// Priority 2: Among responses with errors, prefer partial responses over complete timeouts
		if r.Error != nil && best.Error != nil {
			if !r.Status.NoResponse && best.Status.NoResponse {
				best = r
				continue
			}
		}

		// Priority 3: Among equivalent response types, compare RTT (only if both have valid RTT)
		if r.Error == nil && best.Error == nil {
			// Both are valid responses - compare RTT
			if r.Status.Rtt != nil && best.Status.Rtt != nil &&
				r.Status.Rtt.AsDuration() < best.Status.Rtt.AsDuration() {
				best = r
			}
		} else if r.Error != nil && best.Error != nil &&
			r.Status.NoResponse == best.Status.NoResponse {
			// Both have same error type - compare RTT if available
			if r.Status.Rtt != nil && best.Status.Rtt != nil &&
				r.Status.Rtt.AsDuration() < best.Status.Rtt.AsDuration() {
				best = r
			}
		}
	}

	// errLog := ""
	// if len(best.Error) > 0 {
	// 	errLog = fmt.Sprintf(" err: %q", best.Error)
	// }
	// log.Printf("best result for %s - offset: %s rtt: %s%s",
	// 	ip.String(), best.Offset.AsDuration(), best.RTT.AsDuration(), errLog)

	return best.Status, best.Response, best.Error
}

func ntpResponseToApiStatus(ip *netip.Addr, resp *ntp.Response) *apiv2.ServerStatus {
	// log.Printf("Leap: %d", resp.Leap)
	status := &apiv2.ServerStatus{
		Ts:         timestamppb.Now(),
		Offset:     durationpb.New(resp.ClockOffset),
		Stratum:    int32(resp.Stratum),
		Leap:       int32(resp.Leap),
		Rtt:        durationpb.New(resp.RTT),
		NoResponse: false,
		// Responses:
	}
	status.SetIP(ip)
	return status
}

func referenceIDString(refid uint32) string {
	b := make([]byte, 4)
	binary.BigEndian.PutUint32(b[0:], uint32(refid))
	return string(b)
}
