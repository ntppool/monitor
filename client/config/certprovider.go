package config

import (
	"context"
	"crypto/tls"
	"crypto/x509"
	"errors"
)

func (ac *appConfig) GetClientCertificate(certRequestInfo *tls.CertificateRequestInfo) (*tls.Certificate, error) {
	ac.lock.RLock()
	defer ac.lock.RUnlock()
	return ac.tlsCert, nil
}

func (ac *appConfig) GetCertificate(hello *tls.ClientHelloInfo) (*tls.Certificate, error) {
	ac.lock.RLock()
	defer ac.lock.RUnlock()
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
