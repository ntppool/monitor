package localok

import (
	"context"
	"fmt"
	"net"
	"net/netip"
	"sync"
	"time"

	"github.com/prometheus/client_golang/prometheus"
	"go.opentelemetry.io/otel/attribute"
	"go.opentelemetry.io/otel/codes"
	"go.opentelemetry.io/otel/trace"
	"go4.org/netipx"
	"golang.org/x/sync/errgroup"

	"go.ntppool.org/common/logger"
	"go.ntppool.org/common/tracing"
	"go.ntppool.org/monitor/client/config/checkconfig"
	"go.ntppool.org/monitor/client/monitor"
)

type metrics struct {
	Ok        *prometheus.GaugeVec
	LastCheck *prometheus.GaugeVec
}

type LocalOK struct {
	cfg        *checkconfig.Config
	isv4       bool
	lastCheck  time.Time
	lastStatus bool
	seenHosts  map[string]bool
	mu         sync.RWMutex
}

const (
	localCacheTTL = 180 * time.Second
	maxOffset     = 10 * time.Millisecond
)

var (
	localMetrics         = metrics{}
	syncLocalMetricsOnce sync.Once
)

func NewLocalOK(conf checkconfig.ConfigGetter, promreg prometheus.Registerer) *LocalOK {
	var isv4 bool

	cfg := conf.GetConfig()

	if cfg == nil || cfg.IP == nil {
		return nil
	}

	if cfg.IP.Is4() {
		isv4 = true
	} else {
		isv4 = false
	}

	syncLocalMetricsOnce.Do(func() {
		localMetrics.Ok = prometheus.NewGaugeVec(prometheus.GaugeOpts{
			Name: "local_check_up",
		}, []string{"host", "ip_version"})
		promreg.MustRegister(localMetrics.Ok)

		localMetrics.LastCheck = prometheus.NewGaugeVec(prometheus.GaugeOpts{
			Name: "local_check_time",
		}, []string{"host", "ip_version"})
		promreg.MustRegister(localMetrics.LastCheck)
	})

	return &LocalOK{
		cfg:       cfg,
		isv4:      isv4,
		seenHosts: make(map[string]bool),
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

func (l *LocalOK) SetConfig(cfg *checkconfig.Config) {
	l.mu.Lock()
	defer l.mu.Unlock()
	l.cfg = cfg
}

func (l *LocalOK) Check(ctx context.Context) bool {
	l.mu.RLock()
	if time.Now().Before(l.lastCheck.Add(localCacheTTL)) {
		l.mu.RUnlock()
		return l.lastStatus
	}
	l.mu.RUnlock()

	l.mu.Lock()
	defer l.mu.Unlock()

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
	cfg := l.cfg
	log := logger.Setup()

	ipVersion := "v6"
	if l.isv4 {
		ipVersion = "v4"
	}

	if len(cfg.BaseChecks) == 0 {
		log.WarnContext(ctx, "did not get NTP BaseChecks from API; no localok configuration available")
		return false
	}

	var allHosts []string
	for _, s := range cfg.BaseChecks {
		allHosts = append(allHosts, string(s))
	}

	type namedIP struct {
		Name string
		IP   *netip.Addr
	}

	hosts := []namedIP{}

	fails := 0

	for h := range l.seenHosts {
		// mark not seen
		l.seenHosts[h] = false
	}

	for _, h := range allHosts {

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

	results := make(chan bool)

	g, _ := errgroup.WithContext(ctx)

	g.Go(func() error {
		for ok := range results {
			if !ok {
				fails++
			}
		}
		return nil
	})

	g.Go(func() error {
		wg := sync.WaitGroup{}

		for i, h := range hosts {
			wg.Add(1)

			// seen
			l.seenHosts[h.Name] = true

			if i > 0 {
				time.Sleep(20 * time.Millisecond)
			}

			go func(h namedIP) {
				ok, err := l.sanityCheckHost(ctx, cfg, h.Name, h.IP)
				if err != nil {
					log.WarnContext(ctx, "local-check failure", "server", h.Name, "ip", h.IP.String(), "err", err.Error())
					span.RecordError(err)
				}

				localMetrics.LastCheck.WithLabelValues(h.Name, ipVersion).Set(float64(time.Now().Unix()))
				m := localMetrics.Ok.WithLabelValues(h.Name, ipVersion)
				if ok {
					m.Set(1)
				} else {
					m.Set(0)
				}

				results <- ok
				wg.Done()
			}(h)
		}

		wg.Wait()
		close(results)

		for h, seen := range l.seenHosts {
			if !seen {
				localMetrics.Ok.DeleteLabelValues(h, ipVersion)
				localMetrics.LastCheck.DeleteLabelValues(h, ipVersion)
			}
		}

		return nil
	})

	g.Wait()

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
