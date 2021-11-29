package main

import (
	"fmt"
	"log"
	"net"
	"sync"
	"time"

	"go.ntppool.org/monitor/api/pb"

	"go.ntppool.org/monitor"
)

type LocalOK struct {
	cfg        *pb.Config
	isv4       bool
	lastCheck  time.Time
	lastStatus bool
	mu         sync.RWMutex
}

const localCacheSeconds = 60
const maxOffset = 2 * time.Millisecond

func NewLocalOK(cfg *pb.Config) *LocalOK {
	var isv4 bool
	if cfg.IP.To4() != nil {
		isv4 = true
	} else {
		isv4 = false
	}

	return &LocalOK{cfg: cfg, isv4: isv4}
}

func (l *LocalOK) Check() bool {
	l.mu.RLock()
	if time.Now().Before(l.lastCheck.Add(localCacheSeconds * time.Second)) {
		l.mu.RUnlock()
		return l.lastStatus
	}
	l.mu.RUnlock()
	l.mu.Lock()
	defer l.mu.Unlock()
	// this might make a bunch check at once ...
	ok := l.update()
	l.lastCheck = time.Now()
	l.lastStatus = ok
	return ok
}

func (l *LocalOK) update() bool {

	// todo: get this from server config ...
	allHosts := []string{
		// "localhost",
		"time-macos.apple.com",
		// "ntp.ubuntu.com",
		// "time.google.com",
		"tock.ucla.edu",
		// "time-d-b.nist.gov",
		"ntp.inet.tele.dk",
		"uslax1-ntp-001.aaplimg.com",
		"defra1-ntp-002.aaplimg.com",
		"uklon5-ntp-001.aaplimg.com",
		"ntp.stupi.se",
		"ntp4.sptime.se",
		"ntp.se",
	}

	type namedIP struct {
		Name string
		IP   *net.IP
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
		var ip *net.IP

		// log.Printf("got IPs for %s: %s", h, ips)

		for _, i := range ips {
			switch i.To4() != nil {
			case true:
				if l.isv4 {
					ip = &i
				}
			case false:
				if !l.isv4 {
					ip = &i
				}
			}
			if ip != nil {
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
				log.Printf("Checking %q %q: %s", h.Name, h.IP.String(), err)
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
		log.Printf("Too many errors, declare local not-sane")
		return false
	}

	return true
}

func (l *LocalOK) sanityCheckHost(name string, ip *net.IP) (bool, error) {
	status, err := monitor.CheckHost(ip, l.cfg)
	if err != nil {
		return false, err
	}

	offset := status.Offset
	if offset < 0 {
		offset = offset * -1
	}

	log.Printf("offset for %s (%s): %s", name, ip, status.Offset)

	if offset > maxOffset {
		return false, fmt.Errorf("offset too large: %s", status.Offset)
	}

	if status.Leap == 3 {
		return false, fmt.Errorf("NotInSync")
	}

	return true, nil
}
