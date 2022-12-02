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
		log.Printf("Could not load certificates: %s", err)
	}

	if ca.Cert == nil {
		err := ca.IssueCertificates()
		if err != nil {
			return err
		}
	}

	return err

}

// checkCertificateValidity checks if the certificate is
// valid and how long until it needs renewal
func (ca *ClientAuth) checkCertificateValidity() (bool, time.Duration) {
	ca.lock.RLock()
	defer ca.lock.RUnlock()

	if ca.Cert == nil || ca.Cert.Leaf == nil {
		return false, time.Second * 2
	}

	c := ca.Cert.Leaf

	duration := c.NotAfter.Sub(c.NotBefore)
	renewAfter := c.NotAfter.Add(-duration / 3)

	if time.Now().After(c.NotAfter.Add(-1 * time.Hour)) {
		return false, time.Second * 0
	}

	return true, renewAfter.Sub(time.Now()) + time.Second*1
}

func (ca *ClientAuth) RenewCertificates() error {

	for {
		valid, wait := ca.checkCertificateValidity()
		if !valid || wait < 0 {
			err := ca.IssueCertificates()
			if err != nil {
				log.Printf("error issuing certificate: %s", err)
				wait = 90 * time.Second
			}
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
