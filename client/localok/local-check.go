package localok

import (
	"context"
	"fmt"
	"net"
	"net/netip"
	"sync"
	"time"

	"go.ntppool.org/common/logger"
	"go.ntppool.org/monitor/api/pb"
	"go4.org/netipx"
	"golang.org/x/exp/slog"
	"golang.org/x/sync/errgroup"

	"go.ntppool.org/monitor/client/config"
	"go.ntppool.org/monitor/client/monitor"
)

type LocalOK struct {
	cfg        *pb.Config
	isv4       bool
	lastCheck  time.Time
	lastStatus bool
	mu         sync.RWMutex
}

const localCacheTTL = 180 * time.Second
const maxOffset = 10 * time.Millisecond

func NewLocalOK(conf config.ConfigUpdater) *LocalOK {
	var isv4 bool

	cfg := conf.GetConfig()

	if cfg.GetIP().Is4() {
		isv4 = true
	} else {
		isv4 = false
	}

	return &LocalOK{cfg: cfg, isv4: isv4}
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

func (l *LocalOK) SetConfig(cfg *pb.Config) {
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

	// update() is wrapped in a lock
	cfg := l.cfg

	// overridden by server config
	allHosts := []string{
		"time.apple.com",
		// "ntp1.net.berkeley.edu",
		"uslax1-ntp-001.aaplimg.com",
		"defra1-ntp-002.aaplimg.com",
		"uklon5-ntp-001.aaplimg.com",
		// "ntp.stupi.se",
		// "ntp.nict.jp",
		// "ntp.ripe.net",
		"time.fu-berlin.de",
		"ntp.se",
	}

	if len(cfg.BaseChecks) > 0 {
		allHosts = []string{}
		for _, s := range cfg.BaseChecks {
			allHosts = append(allHosts, string(s))
		}
	} else {
		slog.Info("did not get NTP BaseChecks from API, using built-in defaults")
	}

	type namedIP struct {
		Name string
		IP   *netip.Addr
	}

	hosts := []namedIP{}

	fails := 0

	// log.Printf("Looking for ipv4: %t", l.isv4)

	for _, h := range allHosts {

		ips, err := net.LookupIP(h)
		if err != nil {
			logger.Setup().Warn("dns lookup failed", "host", h, "err", err)
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

			if i > 0 {
				time.Sleep(10 * time.Millisecond)
			}

			go func(h namedIP) {
				ok, err := l.sanityCheckHost(cfg, h.Name, h.IP)
				if err != nil {
					slog.Warn("local-check failure", "server", h.Name, "ip", h.IP.String(), "err", err)
				}
				results <- ok
				wg.Done()
			}(h)
		}

		wg.Wait()
		close(results)

		return nil
	})

	g.Wait()

	failureThreshold := len(hosts) - ((len(hosts) + 2) / 2)
	slog.Info("local-check", "failures", fails, "threshold", failureThreshold, "hosts", len(hosts))

	return fails <= failureThreshold
}

func (l *LocalOK) sanityCheckHost(cfg *pb.Config, name string, ip *netip.Addr) (bool, error) {
	status, _, err := monitor.CheckHost(ip, cfg)
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
