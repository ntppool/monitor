package config

import (
	"context"
	"crypto/tls"
	"crypto/x509"
	"errors"
)

func (ac *appConfig) HaveCertificate() bool {
	ac.lock.RLock()
	defer ac.lock.RUnlock()

	if ac.tlsCert == nil {
		return false
	}

	if len(ac.tlsCert.Certificate) == 0 {
		return false
	}

	// if ac.tlsCert.Leaf == nil {
	// 	return false
	// }

	return true
}

func (ac *appConfig) GetClientCertificate(certRequestInfo *tls.CertificateRequestInfo) (*tls.Certificate, error) {
	ac.lock.RLock()
	defer ac.lock.RUnlock()
	if ac.tlsCert == nil || len(ac.tlsCert.Certificate) == 0 {
		return nil, errors.New("no client certificate available (pending approval or not issued)")
	}
	return ac.tlsCert, nil
}

func (ac *appConfig) GetCertificate(hello *tls.ClientHelloInfo) (*tls.Certificate, error) {
	ac.lock.RLock()
	defer ac.lock.RUnlock()
	if ac.tlsCert == nil || len(ac.tlsCert.Certificate) == 0 {
		return nil, errors.New("no certificate available (pending approval or not issued)")
	}
	return ac.tlsCert, nil
}

func (ac *appConfig) setCertificate(_ context.Context, cert *tls.Certificate) error {
	ac.lock.Lock()
	defer ac.lock.Unlock()

	if cert == nil || len(cert.Certificate) == 0 {
		return errors.New("setCertificate called with no certificate")
	}

	if c, e := x509.ParseCertificate(cert.Certificate[0]); e == nil {
		cert.Leaf = c
	}

	// log := logger.FromContext(ctx)
	// log.DebugContext(ctx, "loaded certificate")

	ac.tlsCert = cert

	return nil
}
