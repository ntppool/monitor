package apitls

import (
	"crypto/tls"
	"crypto/x509"
	_ "embed" // embed CA from file
	"errors"

	"github.com/abh/certman"
	"go.ntppool.org/common/logger"
)

//go:embed ca.pem
var caBytes []byte

var (
	ErrNoAuthProvider = errors.New("no auth provider configured")
	ErrNoCertificate  = errors.New("no certificate")
)

type AuthProvider interface {
	GetCertificate(hello *tls.ClientHelloInfo) (*tls.Certificate, error)
	GetClientCertificate(certRequestInfo *tls.CertificateRequestInfo) (*tls.Certificate, error)
	GetAPIKey() string
}

func CAPool() (*x509.CertPool, error) {
	capool := x509.NewCertPool()
	if !capool.AppendCertsFromPEM(caBytes) {
		return nil, errors.New("credentials: failed to append certificates")
	}

	return capool, nil
}

// GetCertman sets up certman for the specified cert / key pair. It is
// used in the monitor-api and (for now) in the client
func GetCertman(certFile, keyFile string) (*certman.CertMan, error) {
	cm, err := certman.New(certFile, keyFile)
	if err != nil {
		return nil, err
	}
	log := logger.NewStdLog("cm", false, nil)
	cm.Logger(log)
	err = cm.Watch()
	if err != nil {
		return nil, err
	}
	return cm, nil
}
