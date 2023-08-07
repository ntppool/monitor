package auth

import (
	"context"
	"crypto/tls"
	"errors"
	"fmt"
	"os"
	"sync"
	"time"

	"github.com/prometheus/client_golang/prometheus"
	"go.ntppool.org/common/logger"
	"go.ntppool.org/monitor/api"
)

const vaultAuthPrefix = "/monitors"

type ClientAuth struct {
	Name  string
	Vault *Vault

	Cert *tls.Certificate `json:"-"`

	deploymentEnv api.DeploymentEnvironment

	// CertRaw []byte `json:"Cert"`
	// KeyRaw  []byte `json:"Key"`

	ctx  context.Context
	dir  string
	lock sync.RWMutex
}

func New(ctx context.Context, dir, name, key, secret string) (*ClientAuth, error) {

	log := logger.FromContext(ctx)

	depEnv, err := api.GetDeploymentEnvironmentFromName(name)
	if err != nil {
		return nil, err
	}

	vault, err := NewVault(key, secret, fmt.Sprintf("%s/%s", vaultAuthPrefix, depEnv))
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

	if vault == nil || vault.client == nil {
		if vault == nil {
			return nil, fmt.Errorf("vault struct not setup")
		}

		return nil, fmt.Errorf("vault client not setup")
	}

	err = ca.load()
	if err != nil {
		if !os.IsNotExist(err) {
			return nil, fmt.Errorf("could not load existing state: %s", err)
		}
	}

	changed := false

	if ca.Name != name {
		log.Warn("monitor name changed from the last state (%s => %s)", ca.Name, name)
		ca.Name = name
		changed = true
	}

	if ca.Vault.secret != secret || ca.Vault.key != key {
		ca.Vault.secret = secret
		ca.Vault.key = key
		changed = true
	}

	if changed {
		ca.Vault.AuthSecret = nil
		ca.Vault.Token = ""
		err = ca.save()
		if err != nil {
			return nil, err
		}
	}

	return ca, nil

}

func (ca *ClientAuth) Manager(promreg prometheus.Registerer) error {
	log := logger.FromContext(ca.ctx)

	promGauge := prometheus.NewGaugeFunc(prometheus.GaugeOpts{
		Name: "ssl_earliest_cert_expiry",
		Help: "TLS expiration time",
	}, func() float64 {
		notAfter, _, _, err := ca.CertificateDates()
		if err != nil {
			log.Error("could not get certificate notAfter date", "err", err)
			return 0
		}
		return float64(notAfter.Unix())
	})
	promreg.MustRegister(promGauge)

	go func() {
		err := ca.RenewCertificates()
		if err != nil {
			if !errors.Is(err, context.Canceled) {
				log.Error("RenewCertificates failed", "err", err)
			}
		}
	}()

	return nil
}

func (ca *ClientAuth) Login() error {
	// log.Printf("ClientAuth.Login(), vault: %+v", ca.Vault)
	authInfo, err := ca.Vault.Login(ca.ctx)
	if err != nil {
		return err
	}

	// log.Printf("ClientAuth logged in: %+v", authInfo)

	updateChannel := make(chan bool, 10)

	go func() {
		err := ca.Vault.RenewToken(ca.ctx, authInfo, updateChannel)
		if err != nil {
			logger.Setup().Error("failed in RenewToken, fatal error", "err", err)
			// todo: handle this more gracefully
			os.Exit(11)
		}
	}()

	go func() {
		for {
			select {
			case <-updateChannel:
				ca.save()

			case <-ca.ctx.Done():
				return
			}
		}
	}()

	return ca.save()
}

func (ca *ClientAuth) WaitUntilReady() error {
	log := logger.FromContext(ca.ctx)
	for {
		if ok, _, _ := ca.checkCertificateValidity(); ok {
			return nil
		}
		log.Info("Waiting for TLS certificate to be available")

		timer := time.NewTimer(5 * time.Second)

		select {
		case <-timer.C:
		case <-ca.ctx.Done():
			timer.Stop()
			return ca.ctx.Err()
		}
	}
}
