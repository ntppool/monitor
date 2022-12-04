package auth

import (
	"context"
	"crypto/tls"
	"fmt"
	"log"
	"os"
	"time"

	vaultapi "github.com/hashicorp/vault/api"
)

func (ca *ClientAuth) LoadOrIssueCertificates() error {
	ctx := ca.ctx

	err := ca.LoadCertificates(ctx)
	if err != nil {
		if !os.IsNotExist(err) {
			log.Printf("Could not load certificates: %s", err)
		}
	}

	_, notAfter, _, err := ca.CertificateDates()

	if ca.Cert == nil || err != nil || time.Now().After(notAfter) {
		err := ca.IssueCertificates()
		if err != nil {
			return err
		}
		return nil
	}

	return err

}

// CertificateDates returns NotBefore, NotAfter and the remaining validity
func (ca *ClientAuth) CertificateDates() (time.Time, time.Time, time.Duration, error) {
	ca.lock.RLock()
	defer ca.lock.RUnlock()

	if ca.Cert == nil || ca.Cert.Leaf == nil {
		return time.Time{}, time.Time{}, 0, fmt.Errorf("no certificate")
	}

	c := ca.Cert.Leaf

	return c.NotBefore, c.NotAfter, time.Until(c.NotAfter), nil
}

// checkCertificateValidity checks if the certificate is
// valid and how long until it needs renewal
func (ca *ClientAuth) checkCertificateValidity() (bool, time.Duration, error) {

	notAfter, notBefore, _, err := ca.CertificateDates()
	if err != nil {
		return false, 0, err
	}

	duration := notAfter.Sub(notBefore)
	renewAfter := notAfter.Add(-duration / 3)

	if time.Now().After(notAfter.Add(-duration / 3)) {
		maxTime := time.Second * 30
		delay := duration / 3
		if delay > maxTime {
			delay = maxTime
		}
		return false, delay, nil
	}

	return true, time.Until(renewAfter) + time.Second*1, nil
}

func (ca *ClientAuth) RenewCertificates() error {

	for {
		valid, wait, _ := ca.checkCertificateValidity()
		if !valid || wait < 0 {
			err := ca.IssueCertificates()
			if err != nil {
				log.Printf("error issuing certificate: %s", err)
				wait = 300 * time.Second
			}
		}

		if wait < 0 {
			wait = 0 * time.Second
		}
		log.Printf("RenewCertificates - checking certificate renewal in: %s", wait)
		timer := time.NewTimer(wait)
		select {
		case <-timer.C:
		case <-ca.ctx.Done():
			timer.Stop()
			return ca.ctx.Err()
		}
	}
}

func (ca *ClientAuth) LoadCertificates(ctx context.Context) error {

	certPem, err := os.ReadFile(ca.stateFilePrefix("cert.pem"))
	if err != nil {
		return err
	}
	keyPem, err := os.ReadFile(ca.stateFilePrefix("key.pem"))
	if err != nil {
		return err
	}

	cert, err := tls.X509KeyPair(certPem, keyPem)
	if err != nil {
		return err
	}

	ca.SetCertificate(&cert)

	return nil
}

func (ca *ClientAuth) IssueCertificates() error {
	if ok, nil := ca.Vault.checkToken(context.Background()); ok {
		return nil
	}
	err := ca.Login()
	if err != nil {
		return err
	}

	vault, err := ca.Vault.vaultClient()
	if err != nil {
		return err
	}

	data := map[string]interface{}{
		"common_name": ca.Name,
		"ttl":         "36h",
	}

	issuePath := "pki_servers/issue/monitors-" + ca.deploymentEnv

	rv, err := vault.Logical().WriteWithContext(ca.ctx, issuePath, data)
	if err != nil {
		return err
	}

	cert, err := getVaultDataString(rv, "certificate")
	if err != nil {
		return err
	}

	privateKey, err := getVaultDataString(rv, "private_key")
	if err != nil {
		return err
	}

	tlsCert, err := tls.X509KeyPair([]byte(cert), []byte(privateKey))
	if err != nil {
		return err
	}

	ca.SetCertificate(&tlsCert)

	err = replaceFile(ca.stateFilePrefix("cert.pem"), []byte(cert))
	if err != nil {
		return err
	}

	err = replaceFile(ca.stateFilePrefix("key.pem"), []byte(privateKey))
	if err != nil {
		return err
	}

	return nil

}

func getVaultDataString(rv *vaultapi.Secret, k string) (string, error) {

	var d string

	iv, ok := rv.Data[k]
	if !ok {
		return "", fmt.Errorf("did not get %s data from vault", k)
	}

	switch v := iv.(type) {
	case []interface{}:
		for i, ci := range v {
			c, ok := ci.(string)
			if !ok {
				return "", fmt.Errorf("vault data %s isn't []string (%T)", k, ci)
			}
			if i == 0 {
				d = c
			} else {
				d += "\n" + c
			}
		}
	case string:
		d = v
	default:
		return "", fmt.Errorf("don't know how to handle %s data (%T)", k, v)
	}

	return d, nil
}
