package server

import (
	"crypto/x509"
	"errors"
	"strings"
)

// certificateMiddleware is now replaced by dualAuthMiddleware in auth.go

func (srv *Server) getVerifiedCert(verifiedChains [][]*x509.Certificate) (*x509.Certificate, string) {
	for _, chain := range verifiedChains {
		for _, cert := range chain {
			for _, name := range cert.DNSNames {
				// log.Printf("verified cert, dns name: %+v", cert.DNSNames)
				// log.Printf("issuer: %s", cert.Issuer)
				if strings.HasSuffix(name, ".mon.ntppool.dev") {
					return cert, name
				}
			}
		}
	}
	return nil, ""
}

func (srv *Server) verifyClient(rawCerts [][]byte, verifiedChains [][]*x509.Certificate) error {
	// With JWT authentication support, client certificates are optional
	// This function is only called when a client certificate is presented
	cert, _ := srv.getVerifiedCert(verifiedChains)
	if cert != nil {
		return nil
	}
	// Allow no certificate - JWT authentication will be used instead
	return errors.New("no valid certificate found")
}
