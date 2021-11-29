package ca

import (
	"crypto/x509"
	_ "embed"
	"errors"
)

//go:embed ca.pem
var caBytes []byte

func CAPool() (*x509.CertPool, error) {
	capool := x509.NewCertPool()
	if !capool.AppendCertsFromPEM(caBytes) {
		return nil, errors.New("credentials: failed to append certificates")
	}

	return capool, nil
}
