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
	appConfig api.AppConfig

	Vault *Vault

	Cert *tls.Certificate `json:"-"`
	lock sync.RWMutex

	// CertRaw []byte `json:"Cert"`
	// KeyRaw  []byte `json:"Key"`

	ctx context.Context
}

func New(ctx context.Context, appConfig api.AppConfig) (*ClientAuth, error) {
	vault, err := NewVault(appConfig.AppKey(), appConfig.AppSecret(), fmt.Sprintf("%s/%s", vaultAuthPrefix, appConfig.Env()))
	if err != nil {
		return nil, err
	}

	ca := &ClientAuth{
		appConfig: appConfig,
		ctx:       ctx,
		Vault:     vault,
	}

	if vault == nil || vault.client == nil {
		if vault == nil {
			return nil, fmt.Errorf("vault struct not setup")
		}

		return nil, fmt.Errorf("vault client not setup")
	}

	return ca, nil
}

func (ca *ClientAuth) Manager(promreg prometheus.Registerer) error {
	log := logger.FromContext(ca.ctx)

	promGauge := prometheus.NewGaugeFunc(prometheus.GaugeOpts{
		Name: "ssl_earliest_cert_expiry",
		Help: "TLS expiration time",
	}, func() float64 {
		_, notAfter, _, err := ca.CertificateDates()
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
	log := logger.FromContext(ca.ctx)

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
				log.WarnContext(ca.ctx, "Vault token updated, handling not implemented!")
				// todo: save vault token to disk? Or maybe logging
				// in for each run makes sense now that we have another
				// api key for the monitor
				// ca.save()

			case <-ca.ctx.Done():
				return
			}
		}
	}()

	return nil
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
