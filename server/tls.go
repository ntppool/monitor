package server

import (
	"context"
	"crypto/x509"
	"errors"
	"net/http"
	"strings"

	sctx "go.ntppool.org/monitor/server/context"
)

func (srv *Server) certificateMiddleware(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_, name := srv.getVerifiedCert(r.TLS.VerifiedChains)

		ctx := context.WithValue(r.Context(), sctx.CertificateKey, name)
		rctx := r.WithContext(ctx)
		next.ServeHTTP(w, rctx)
	})
}

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
	cert, _ := srv.getVerifiedCert(verifiedChains)
	if cert != nil {
		return nil
	}
	return errors.New("no valid certificate found")
}
