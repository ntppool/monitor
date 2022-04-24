package auth

import (
	"context"
	"crypto/tls"
	"fmt"
	"log"
	"os"
	"sync"
	"time"

	vaultapi "github.com/hashicorp/vault/api"
	"go.ntppool.org/monitor/api"
)

type ClientAuth struct {
	Name  string
	Vault *Vault

	Cert *tls.Certificate `json:"-"`

	deploymentEnv string

	// CertRaw []byte `json:"Cert"`
	// KeyRaw  []byte `json:"Key"`

	ctx  context.Context
	dir  string
	lock sync.RWMutex
}

type Vault struct {
	key    string
	secret string
	Token  string

	client *vaultapi.Client
	lock   sync.RWMutex
}

func New(ctx context.Context, dir, name, key, secret string) (*ClientAuth, error) {

	vault := &Vault{
		key:    key,
		secret: secret,
	}

	depEnv, err := api.GetDeploymentEnvironment(name)
	if err != nil {
		return nil, err
	}

	ca := &ClientAuth{
		Name:          name,
		deploymentEnv: depEnv,
		ctx:           ctx,
		dir:           dir,
		Vault:         vault,
	}

	err = ca.load()
	if err != nil {
		if !os.IsNotExist(err) {
			return nil, fmt.Errorf("could not load existing state: %s", err)
		}
	}

	changed := false

	if ca.Name != name {
		log.Printf("monitor name changed from the last state (%s => %s)", ca.Name, name)
		ca.Name = name
		changed = true
	}

	if ca.Vault.secret != secret || ca.Vault.key != key {
		ca.Vault.secret = secret
		ca.Vault.key = key
		changed = true
	}

	if changed {
		err = ca.save()
		if err != nil {
			return nil, err
		}
	}

	return ca, nil

}

func (ca *ClientAuth) Manager() error {

	go func() {
		err := ca.RenewCertificates()
		if err != nil {
			log.Printf("RenewCertificates failed: %s", err)
		}
	}()

	return nil
}

func (ca *ClientAuth) Login() error {
	err := ca.Vault.Login(ca.ctx, ca.deploymentEnv)
	if err != nil {
		return err
	}

	return ca.save()
}

func (ca *ClientAuth) WaitUntilReady() error {
	for {
		log.Printf("WaitUntilReady: Checking if cert is ready")
		if ok, _ := ca.checkCertificateValidity(); ok {
			return nil
		}
		log.Printf("WaitUntilReady: Not yet ...")

		timer := time.NewTimer(5 * time.Second)

		select {
		case <-timer.C:
		case <-ca.ctx.Done():
			timer.Stop()
			return ca.ctx.Err()
		}
	}
}
