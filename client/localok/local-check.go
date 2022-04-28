package localok

import (
	"fmt"
	"log"
	"net"
	"sync"
	"time"

	"go.ntppool.org/monitor/api/pb"
	"inet.af/netaddr"

	"go.ntppool.org/monitor"
)

type LocalOK struct {
	cfg        *pb.Config
	isv4       bool
	lastCheck  time.Time
	lastStatus bool
	mu         sync.RWMutex
}

const localCacheTTL = 120 * time.Second
const maxOffset = 3500 * time.Microsecond

func NewLocalOK(cfg *pb.Config) *LocalOK {
	var isv4 bool

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
	wait := nextCheck.Sub(time.Now())

	if wait < 0 {
		return time.Second * 0
	}

	return wait
}

func (l *LocalOK) Check() bool {
	l.mu.RLock()
	if time.Now().Before(l.lastCheck.Add(localCacheTTL)) {
		l.mu.RUnlock()
		return l.lastStatus
	}
	l.mu.RUnlock()

	l.mu.Lock()
	defer l.mu.Unlock()

	ok := l.update()
	l.lastCheck = time.Now()
	l.lastStatus = ok
	return ok
}

func (l *LocalOK) update() bool {

	// todo: get this from server config ...
	allHosts := []string{
		"time.apple.com",
		// "ntp.ubuntu.com",
		// "time.google.com",
		"ntp1.net.berkeley.edu",
		"tock.ucla.edu",
		"ntp.inet.tele.dk",
		"uslax1-ntp-001.aaplimg.com",
		"defra1-ntp-002.aaplimg.com",
		"uklon5-ntp-001.aaplimg.com",
		"ntp.stupi.se",
		// "ntp.se",
		"ntp.nict.jp",
		"ntp.ripe.net",
		"time.fu-berlin.de",
	}

	type namedIP struct {
		Name string
		IP   *netaddr.IP
	}

	hosts := []namedIP{}

	fails := 0

	// log.Printf("Looking for ipv4: %t", l.isv4)

	for _, h := range allHosts {

		ips, err := net.LookupIP(h)
		if err != nil {
			log.Printf("dns lookup for '%s': %s", h, err)
			continue
		}
		var ip *netaddr.IP

		// log.Printf("got IPs for %s: %s", h, ips)

		for _, dnsIP := range ips {
			i, ok := netaddr.FromStdIP(dnsIP)
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

	wg := sync.WaitGroup{}
	results := make(chan bool)

	go func() {
		for ok := range results {
			if !ok {
				fails++
			}
		}
	}()

	for i, h := range hosts {
		wg.Add(1)

		if i > 0 {
			time.Sleep(10 * time.Millisecond)
		}

		go func(h namedIP) {
			ok, err := l.sanityCheckHost(h.Name, h.IP)
			if err != nil {
				log.Printf("Failure for %q %q: %s", h.Name, h.IP.String(), err)
			}
			results <- ok
			wg.Done()
		}(h)
	}

	wg.Wait()

	close(results)

	failureThreshold := len(hosts) - ((len(hosts) + 2) / 2)
	log.Printf("failures: %d, threshold: %d, hosts: %d", fails, failureThreshold, len(hosts))

	if fails > failureThreshold {
		return false
	}

	return true
}

func (l *LocalOK) sanityCheckHost(name string, ip *netaddr.IP) (bool, error) {
	status, err := monitor.CheckHost(ip, l.cfg)
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
