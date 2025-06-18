package config

import (
	"context"
	"time"

	"github.com/prometheus/client_golang/prometheus"
	"go.ntppool.org/common/logger"
	apitls "go.ntppool.org/monitor/api/tls"
)

// Manager handles AppConfig hot reloading and certificate management
func (ac *appConfig) Manager(ctx context.Context, promreg prometheus.Registerer) error {
	log := logger.FromContext(ctx).WithGroup("appconfig-manager")

	promGauge := prometheus.NewGaugeFunc(prometheus.GaugeOpts{
		Name: "ssl_earliest_cert_expiry",
		Help: "TLS expiration time",
	}, func() float64 {
		_, notAfter, _, err := ac.CertificateDates()
		if err != nil {
			log.Error("could not get certificate notAfter date", "err", err)
			return 0
		}
		return float64(notAfter.Unix())
	})
	promreg.MustRegister(promGauge)

	// Hot reloading goroutine
	go func() {
		log.InfoContext(ctx, "AppConfig hot reloader started")

		// Default reload interval
		const defaultReloadInterval = 5 * time.Minute
		const errorRetryInterval = 2 * time.Minute

		// Track previous protocol states for change detection
		var prevIPv4Live, prevIPv6Live bool

		for {
			// Load current configuration
			err := ac.LoadAPIAppConfig(ctx)
			var nextCheck time.Duration

			if err != nil {
				log.WarnContext(ctx, "failed to reload AppConfig", "err", err)
				nextCheck = errorRetryInterval
			} else {
				log.DebugContext(ctx, "AppConfig reloaded successfully")
				nextCheck = defaultReloadInterval

				// Check for protocol status changes
				currentIPv4Live := ac.IPv4().IsLive()
				currentIPv6Live := ac.IPv6().IsLive()

				if currentIPv4Live != prevIPv4Live {
					log.InfoContext(ctx, "IPv4 protocol status changed",
						"previous", prevIPv4Live, "current", currentIPv4Live,
						"status", ac.IPv4().Status, "ip", ac.IPv4().IP)
					prevIPv4Live = currentIPv4Live
				}

				if currentIPv6Live != prevIPv6Live {
					log.InfoContext(ctx, "IPv6 protocol status changed",
						"previous", prevIPv6Live, "current", currentIPv6Live,
						"status", ac.IPv6().Status, "ip", ac.IPv6().IP)
					prevIPv6Live = currentIPv6Live
				}
			}

			// Ensure we don't check too frequently (minimum 1 minute) or too infrequently (maximum 1 hour)
			if nextCheck < 1*time.Minute {
				nextCheck = 1 * time.Minute
			} else if nextCheck > 1*time.Hour {
				nextCheck = 1 * time.Hour
			}

			log.DebugContext(ctx, "scheduling next AppConfig reload", "duration", nextCheck)

			select {
			case <-time.After(nextCheck):
				// Continue to next iteration
			case <-ctx.Done():
				log.InfoContext(ctx, "AppConfig hot reloader shutting down")
				return
			}
		}
	}()

	return nil
}

// CertificateDates returns NotBefore, NotAfter and the remaining validity
func (ac *appConfig) CertificateDates() (time.Time, time.Time, time.Duration, error) {
	ac.lock.RLock()
	defer ac.lock.RUnlock()

	if ac.tlsCert == nil || ac.tlsCert.Leaf == nil {
		return time.Time{}, time.Time{}, 0, apitls.ErrNoCertificate
	}

	c := ac.tlsCert.Leaf

	return c.NotBefore, c.NotAfter, time.Until(c.NotAfter), nil
}

// checkCertificateValidity checks if the certificate is
// valid and how long until it needs renewal
func (ac *appConfig) checkCertificateValidity(ctx context.Context) (bool, time.Duration, error) {
	log := logger.FromContext(ctx)
	notBefore, notAfter, _, err := ac.CertificateDates()
	if err != nil {
		return false, 0, err
	}

	duration := notAfter.Sub(notBefore)
	renewAfter := notAfter.Add(-duration / 3)

	log.DebugContext(ctx, "certificate validity", "notBefore", notBefore, "notAfter", notAfter, "renewAfter", renewAfter, "duration", duration)

	if time.Now().After(notAfter.Add(-duration / 3)) {
		// check again in 2 hours if the certificate is not valid
		// todo: revisit why this is being returned at all
		maxTime := time.Hour * 2
		delay := duration / 3
		if delay > maxTime {
			delay = maxTime
		}
		return false, delay, nil
	}

	return true, time.Until(renewAfter) + time.Second*1, nil
}

// CheckCertificateValidity is the public wrapper for checkCertificateValidity
func (ac *appConfig) CheckCertificateValidity(ctx context.Context) (bool, time.Duration, error) {
	return ac.checkCertificateValidity(ctx)
}
