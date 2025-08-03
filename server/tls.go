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

// getVerifiedCertFromPeers extracts certificate identity from PeerCertificates array
func (srv *Server) getVerifiedCertFromPeers(peerCerts []*x509.Certificate) (*x509.Certificate, string) {
	for _, cert := range peerCerts {
		for _, name := range cert.DNSNames {
			if strings.HasSuffix(name, ".mon.ntppool.dev") {
				return cert, name
			}
		}
	}
	return nil, ""
}

func (srv *Server) verifyClient(rawCerts [][]byte, verifiedChains [][]*x509.Certificate) error {
	// If normal verification already succeeded (RequireAndVerifyClientCert mode)
	if len(verifiedChains) > 0 {
		cert, _ := srv.getVerifiedCert(verifiedChains)
		if cert != nil {
			return nil
		}
	}

	// Manual verification for RequestClientCert mode
	if len(rawCerts) == 0 {
		return nil // No cert provided, allow JWT auth
	}

	// Parse leaf certificate
	leafCert, err := x509.ParseCertificate(rawCerts[0])
	if err != nil {
		return err
	}

	// Build intermediate pool from remaining rawCerts
	intermediates := x509.NewCertPool()
	for i := 1; i < len(rawCerts); i++ {
		if cert, err := x509.ParseCertificate(rawCerts[i]); err == nil {
			intermediates.AddCert(cert)
		}
	}

	// Verify certificate chain using stored CA pool
	_, err = leafCert.Verify(x509.VerifyOptions{
		Roots:         srv.clientCAs,
		Intermediates: intermediates,
	})
	if err != nil {
		return err
	}

	// Check DNS name constraint (.mon.ntppool.dev)
	for _, name := range leafCert.DNSNames {
		if strings.HasSuffix(name, ".mon.ntppool.dev") {
			return nil // Valid certificate
		}
	}

	return errors.New("certificate missing required DNS suffix")
}
