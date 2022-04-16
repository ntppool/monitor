package auth

import (
	"crypto/tls"
	"crypto/x509"
	"log"
)

func (ca *ClientAuth) GetClientCertificate(certRequestInfo *tls.CertificateRequestInfo) (*tls.Certificate, error) {
	ca.lock.RLock()
	defer ca.lock.RUnlock()
	return ca.Cert, nil
}

func (ca *ClientAuth) GetCertificate(hello *tls.ClientHelloInfo) (*tls.Certificate, error) {
	ca.lock.RLock()
	defer ca.lock.RUnlock()
	return ca.Cert, nil
}

func (ca *ClientAuth) SetCertificate(cert *tls.Certificate) {
	ca.lock.Lock()
	defer ca.lock.Unlock()

	if cert == nil || len(cert.Certificate) == 0 {
		log.Printf("SetCertificate called with no certificate")
		return
	}

	if c, e := x509.ParseCertificate(cert.Certificate[0]); e == nil {
		cert.Leaf = c
	}

	ca.Cert = cert

}
