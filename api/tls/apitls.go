package apitls

import (
	"crypto/tls"
	"crypto/x509"
	_ "embed"
	"errors"
	"log"

	"github.com/dyson/certman"
	"github.com/spf13/cobra"
)

//go:embed ca.pem
var caBytes []byte

type CertificateProvider interface {
	GetCertificate(hello *tls.ClientHelloInfo) (*tls.Certificate, error)
	GetClientCertificate(certRequestInfo *tls.CertificateRequestInfo) (*tls.Certificate, error)
}

func CAPool() (*x509.CertPool, error) {
	capool := x509.NewCertPool()
	if !capool.AppendCertsFromPEM(caBytes) {
		return nil, errors.New("credentials: failed to append certificates")
	}

	return capool, nil
}

func GetCertman(cmd *cobra.Command) (*certman.CertMan, error) {
	keyFile, err := cmd.Flags().GetString("key")
	if err != nil {
		return nil, err
	}
	certFile, err := cmd.Flags().GetString("cert")
	if err != nil {
		return nil, err
	}

	cm, err := certman.New(certFile, keyFile)
	if err != nil {
		return nil, err
	}
	cm.Logger(log.Default())
	err = cm.Watch()
	if err != nil {
		return nil, err
	}
	return cm, nil
}
