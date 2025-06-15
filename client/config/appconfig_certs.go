package config

import (
	"context"
	"errors"
	"time"

	"github.com/prometheus/client_golang/prometheus"
	"go.ntppool.org/common/logger"
	apitls "go.ntppool.org/monitor/api/tls"
)

// todo: change to have a function that returns the metrics separately
func (ac *appConfig) Manager(ctx context.Context, promreg prometheus.Registerer) error {
	log := logger.FromContext(ctx)

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

	go func() {
		err := ac.LoadAPIAppConfig(ctx)
		if err != nil {
			if !errors.Is(err, context.Canceled) {
				log.Error("RenewCertificates failed", "err", err)
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
