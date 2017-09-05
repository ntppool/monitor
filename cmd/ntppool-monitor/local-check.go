package main

import (
	"fmt"
	"log"
	"sync"
	"time"
)

type LocalOK struct {
	lastCheck  time.Time
	lastStatus bool
	mu         sync.RWMutex
}

const localCacheSeconds = 60

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
	hosts := []string{
		"127.0.0.1",
		"time.apple.com",
		"time-macos.apple.com",
		"tock.phyber.com",
		"ntp0.us.grundclock.com",
		// "time.google.com",
		"tock.ucla.edu",
		"ntp.stupi.se",
	}

	fails := 0

	for _, h := range hosts {
		ok, err := sanityCheckHost(h)
		if err != nil {
			log.Printf("Checking %q: %s", h, err)
		}
		if !ok {
			fails++
		}
	}

	failureThreshold := len(hosts) - ((len(hosts) + 2) / 2)
	log.Printf("failures: %d, threshold: %d, hosts: %d", fails, failureThreshold, len(hosts))

	if fails > failureThreshold {
		log.Printf("Too many errors, declare local not-sane")
		return false
	}

	return true
}

func sanityCheckHost(host string) (bool, error) {
	status, err := CheckHost(host, 4)
	if err != nil {
		return false, err
	}

	offset := status.Offset
	if offset < 0 {
		offset = offset * -1
	}

	log.Printf("offset for %s: %s", host, status.Offset)

	if offset > (time.Millisecond * 2) {
		return false, fmt.Errorf("offset too large: %s", status.Offset)
	}

	if status.Leap == 3 {
		return false, fmt.Errorf("NotInSync")
	}

	return true, nil
}
