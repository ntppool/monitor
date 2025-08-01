package config

import (
	"context"
	"time"

	"go.ntppool.org/common/logger"
	apitls "go.ntppool.org/monitor/api/tls"
)

// certificateRenewalThreshold defines when to renew certificates as a fraction of their lifetime
const certificateRenewalThreshold = 1.0 / 3.0

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
	renewThresholdDuration := time.Duration(float64(duration) * certificateRenewalThreshold)
	renewAfter := notAfter.Add(-renewThresholdDuration)

	log.DebugContext(ctx, "certificate validity", "notBefore", notBefore, "notAfter", notAfter, "renewAfter", renewAfter, "duration", duration)

	if time.Now().After(renewAfter) {
		// check again in 2 hours if the certificate is not valid
		// todo: revisit why this is being returned at all
		maxTime := time.Hour * 2
		delay := renewThresholdDuration
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
