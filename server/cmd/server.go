package cmd

import (
	"context"
	"crypto/x509"
	"errors"
	"fmt"
	"net/http"
	"os"

	"go.ntppool.org/common/config/depenv"
	"go.ntppool.org/common/health"
	"go.ntppool.org/common/logger"
	"go.ntppool.org/common/metricsserver"

	apitls "go.ntppool.org/monitor/api/tls"
	"go.ntppool.org/monitor/client/config/checkconfig"
	"go.ntppool.org/monitor/mqttcm"
	"go.ntppool.org/monitor/ntpdb"
	"go.ntppool.org/monitor/server"
	sctx "go.ntppool.org/monitor/server/context"
	"go.ntppool.org/monitor/server/jwt"
	"go.ntppool.org/monitor/server/mqserver"
	"golang.org/x/sync/errgroup"
)

type serverCmd struct {
	Listen string `default:":8000" help:"Listen address" flag:"listen"`

	TLS struct {
		Key  string `default:"/etc/tls/tls.key" help:"TLS key file"`
		Cert string `default:"/etc/tls/tls.crt" help:"TLS certificate file" alias:"crt"`
	} `embed:"" prefix:"tls."`

	JWTKey string `help:"JWT signing key" flag:"jwtkey" env:"JWT_KEY"`
}

type mqconfig struct {
	jwtKey, tlsName string
	host            string
	port            int
}

func (mqcfg *mqconfig) GetMQTTConfig() *checkconfig.MQTTConfig {
	jwttoken, err := jwt.GetToken(mqcfg.jwtKey, mqcfg.tlsName, jwt.KeyTypeServer)
	if err != nil {
		logger.Setup().Error("jwt token", "err", err)
		os.Exit(2)
	}

	return &checkconfig.MQTTConfig{
		Host:   mqcfg.host,
		Port:   mqcfg.port,
		JWT:    []byte(jwttoken),
		Prefix: "",
	}
}

func (cfg *serverCmd) Run(ctx context.Context, root *ApiCmd) error {
	log := logger.FromContext(ctx)

	deploymentMode := root.DeploymentMode

	// log.Printf("acfg: %+v", cfg)

	if len(root.DeploymentMode) == 0 {
		return fmt.Errorf("deployment_mode configuration required")
	}

	cm, err := apitls.GetCertman(cfg.TLS.Cert, cfg.TLS.Key)
	if err != nil {
		log.Error("certificate error", "err", err)
		os.Exit(2)
	}

	dbconn, err := ntpdb.OpenDB()
	if err != nil {
		log.Error("database error", "err", err.Error())
		os.Exit(2)
	}

	scfg := server.Config{
		Listen:        cfg.Listen,
		CertProvider:  cm,
		JWTKey:        cfg.JWTKey,
		DeploymentEnv: deploymentMode,
	}

	tlsName, err := func() (string, error) {
		cert, err := cm.GetCertificate(nil)
		if err != nil {
			return "", fmt.Errorf("certificate error: %w", err)
		}

		parsed, err := x509.ParseCertificate(cert.Certificate[0])
		if err != nil {
			return "", err
		}

		r := ""

		if parsed == nil {
			log.Warn("could not parse certificate", "cert", parsed)
			return "", fmt.Errorf("no certificate?")
		}

		for _, dnsName := range parsed.DNSNames {
			if len(r) == 0 {
				r = dnsName
			}
			log.Debug("dnsName from cert", "name", dnsName)
		}
		return r, nil
	}()
	if err != nil {
		return err
	}

	depEnv := depenv.DeploymentEnvironmentFromString(deploymentMode)
	if depEnv == depenv.DeployUndefined {
		log.Error("unknown deployment mode", "deployment_mode", deploymentMode)
		os.Exit(2)
	}

	ctx = context.WithValue(ctx, sctx.DeploymentEnv, depEnv)

	metricssrv := metricsserver.New()
	go metricssrv.ListenAndServe(ctx, 9000)

	mqcfg := mqconfig{
		tlsName: tlsName,
		host:    "mqtt.ntppool.net",
		port:    1883,
		jwtKey:  cfg.JWTKey,
	}

	mqs, err := mqserver.Setup(log, dbconn, metricssrv.Registry())
	if err != nil {
		return err
	}

	topicPrefix := fmt.Sprintf("/%s/monitors", depEnv.String())

	router := mqs.MQTTRouter(ctx, topicPrefix)

	mq, err := mqttcm.Setup(
		ctx, tlsName,
		"",                           // status channel
		[]string{topicPrefix + "/#"}, // subscriptions
		router, &mqcfg, cm,
	)
	if err != nil {
		// todo: autopaho should handle reconnecting, so temporary errors
		// aren't fatal -- unclear if they error out and should be ignored
		// and Setup() only returns errors that are unrecoverable (and thus
		// worth not starting on).
		log.Error("could not setup mqtt connection", "err", err)
	}

	mqs.SetConnectionManager(mq)

	g, ctx := errgroup.WithContext(ctx)

	g.Go(func() error {
		<-mq.Done()
		log.Info("mqtt connection done")
		return nil
	})

	g.Go(func() error {
		return mqs.Run(ctx)
	})

	// todo: ctx + errgroup
	go health.HealthCheckListener(ctx, 8080, log)

	g.Go(func() error {
		srv, err := server.NewServer(ctx, scfg, dbconn, metricssrv.Registry())
		if err != nil {
			log.Error("NewServer() error", "err", err)
			return fmt.Errorf("srv setup: %s", err)
		}
		return srv.Run()
	})

	err = g.Wait()
	if err != nil {
		if !errors.Is(err, http.ErrServerClosed) {
			log.Error("server error", "err", err)
		} else {
			err = nil
		}
	}

	mq.Disconnect(ctx)

	return err
}
