package localok

import (
	"context"
	"fmt"
	"net"
	"net/netip"
	"sync"
	"time"

	"go.opentelemetry.io/otel/attribute"
	"go.opentelemetry.io/otel/codes"
	"go.opentelemetry.io/otel/trace"
	"go4.org/netipx"
	"golang.org/x/sync/errgroup"

	"go.ntppool.org/common/logger"
	"go.ntppool.org/common/tracing"
	"go.ntppool.org/monitor/client/config/checkconfig"
	"go.ntppool.org/monitor/client/metrics"
	"go.ntppool.org/monitor/client/monitor"
)

type LocalOK struct {
	conf       checkconfig.ConfigGetter
	isv4       bool
	lastCheck  time.Time
	lastStatus bool
	mu         sync.RWMutex
}

const (
	localCacheTTL = 180 * time.Second
	maxOffset     = 10 * time.Millisecond
)

func NewLocalOK(conf checkconfig.ConfigGetter) *LocalOK {
	cfg := conf.GetConfig()

	if cfg == nil || cfg.IP == nil {
		return nil
	}

	return &LocalOK{
		conf: conf,
		isv4: cfg.IP.Is4(),
	}
}

func (l *LocalOK) NextCheckIn() time.Duration {
	l.mu.RLock()
	defer l.mu.RUnlock()

	nextCheck := l.lastCheck.Add(localCacheTTL)
	wait := time.Until(nextCheck)

	if wait < 0 {
		return time.Second * 0
	}

	return wait
}

func (l *LocalOK) Check(ctx context.Context) bool {
	l.mu.Lock()
	defer l.mu.Unlock()

	if time.Now().Before(l.lastCheck.Add(localCacheTTL)) {
		return l.lastStatus
	}

	ok := l.update(ctx)
	l.lastCheck = time.Now()
	l.lastStatus = ok
	return ok
}

func (l *LocalOK) update(ctx context.Context) bool {
	ctx, span := tracing.Start(ctx,
		"localcheck-update",
		trace.WithSpanKind(trace.SpanKindInternal),
	)
	defer span.End()

	// update() is wrapped in a lock
	cfg := l.conf.GetConfig()
	log := logger.FromContext(ctx)

	ipVersion := "v6"
	if l.isv4 {
		ipVersion = "v4"
	}

	if cfg == nil || cfg.IP == nil || !cfg.IP.IsValid() || cfg.IP.Is4() != l.isv4 {
		log.WarnContext(ctx, "local check configuration does not match the monitor IP family", "ip_version", ipVersion)
		metrics.SetLocalCheckResults(ipVersion, nil)
		return false
	}
	if len(cfg.BaseChecks) == 0 {
		log.WarnContext(ctx, "did not get NTP BaseChecks from API; no localok configuration available")
		metrics.SetLocalCheckResults(ipVersion, nil)
		return false
	}

	type namedIP struct {
		Name string
		IP   *netip.Addr
	}

	hosts := []namedIP{}

	fails := 0

	for _, h := range cfg.BaseChecks {

		ips, err := net.LookupIP(h)
		if err != nil {
			log.WarnContext(ctx, "dns lookup failed", "host", h, "err", err)
			continue
		}
		var ip *netip.Addr

		// log.Printf("got IPs for %s: %s", h, ips)

		for _, dnsIP := range ips {
			i, ok := netipx.FromStdIP(dnsIP)
			if !ok {
				continue
			}
			if i.Is4() && l.isv4 {
				ip = &i
			}
			if i.Is6() && !l.isv4 {
				ip = &i
			}
			if ip != nil && ip.IsValid() {
				break
			}
		}

		if ip == nil {
			// log.Printf("No IP of appropriate protocol for '%s'", h)
			continue
		}

		hosts = append(hosts, namedIP{Name: h, IP: ip})
	}

	type hostResult struct {
		name   string
		result metrics.LocalCheckResult
	}
	hostResults := make(chan hostResult)

	g, _ := errgroup.WithContext(ctx)

	g.Go(func() error {
		allHostResults := make(map[string]metrics.LocalCheckResult, len(hosts))
		for hr := range hostResults {
			if !hr.result.OK {
				fails++
			}
			allHostResults[hr.name] = hr.result
		}

		metrics.SetLocalCheckResults(ipVersion, allHostResults)

		return nil
	})

	g.Go(func() error {
		wg := sync.WaitGroup{}

		for i, h := range hosts {
			wg.Add(1)

			if i > 0 {
				time.Sleep(20 * time.Millisecond)
			}

			go func(h namedIP) {
				checkTime := time.Now()
				ok, err := l.sanityCheckHost(ctx, cfg, h.Name, h.IP)
				if err != nil {
					log.WarnContext(ctx, "local-check failure", "server", h.Name, "ip", h.IP.String(), "err", err.Error())
					span.RecordError(err)
				}

				hostResults <- hostResult{
					name: h.Name,
					result: metrics.LocalCheckResult{
						OK:        ok,
						CheckTime: checkTime,
					},
				}
				wg.Done()
			}(h)
		}

		wg.Wait()
		close(hostResults)

		return nil
	})

	if err := g.Wait(); err != nil {
		log.ErrorContext(ctx, "local check goroutines failed", "err", err)
		span.RecordError(err)
		span.SetStatus(codes.Error, "goroutines failed")
		return false
	}

	failureThreshold := len(hosts) - ((len(hosts) + 2) / 2)
	log.InfoContext(ctx, "local-check", "failures", fails, "threshold", failureThreshold, "hosts", len(hosts))

	ok := fails <= failureThreshold

	if !ok {
		span.RecordError(fmt.Errorf("too many failures"),
			trace.WithAttributes(
				attribute.Int("failures", fails),
				attribute.Int("failure_threshold", failureThreshold),
			),
		)
		span.SetStatus(codes.Error, "too many failures")
	}

	return ok
}

func (l *LocalOK) sanityCheckHost(ctx context.Context, cfg *checkconfig.Config, name string, ip *netip.Addr) (bool, error) {
	status, _, err := monitor.CheckHost(ctx, ip, cfg, attribute.String("name", name))
	if err != nil {
		return false, err
	}

	offset := status.AbsoluteOffset()

	// log.Printf("offset for %s (%s): %s", name, ip, status.Offset.AsDuration())

	if *offset > maxOffset || *offset < maxOffset*-1 {
		return false, fmt.Errorf("offset too large: %s", status.Offset.AsDuration().String())
	}

	if status.Leap == 3 {
		return false, fmt.Errorf("NotInSync")
	}

	return true, nil
}
