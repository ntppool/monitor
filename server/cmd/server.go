package cmd

import (
	"context"
	"crypto/x509"
	"errors"
	"fmt"
	"net/http"
	"os"
	"os/signal"
	"syscall"

	"go.ntppool.org/common/health"
	"go.ntppool.org/common/logger"
	"go.ntppool.org/common/metricsserver"

	"go.ntppool.org/monitor/api"
	apitls "go.ntppool.org/monitor/api/tls"
	"go.ntppool.org/monitor/client/config"
	"go.ntppool.org/monitor/mqttcm"
	"go.ntppool.org/monitor/ntpdb"
	"go.ntppool.org/monitor/server"
	sctx "go.ntppool.org/monitor/server/context"
	"go.ntppool.org/monitor/server/jwt"
	"go.ntppool.org/monitor/server/mqserver"
	"golang.org/x/sync/errgroup"

	"github.com/spf13/cobra"
)

func (cli *CLI) serverCmd() *cobra.Command {

	var serverCmd = &cobra.Command{
		Use:   "server",
		Short: "server starts the API server",
		Long:  `starts the API server on (default) port 8000`,
		// DisableFlagParsing: true,
		// Args:  cobra.ExactArgs(1),
		RunE: cli.Run(cli.serverCLI),
	}

	serverCmd.PersistentFlags().AddGoFlagSet(cli.Config.Flags())

	return serverCmd
}

type mqconfig struct {
	jwtKey, tlsName string
	host            string
	port            int
}

func (mqcfg *mqconfig) GetMQTTConfig(ctx context.Context) *config.MQTTConfig {
	jwttoken, err := jwt.GetToken(ctx, mqcfg.jwtKey, mqcfg.tlsName, true)
	if err != nil {
		logger.Setup().Error("jwt token", "err", err)
		os.Exit(2)
	}

	return &config.MQTTConfig{
		Host:   mqcfg.host,
		Port:   mqcfg.port,
		JWT:    []byte(jwttoken),
		Prefix: "",
	}
}

func (cli *CLI) serverCLI(cmd *cobra.Command, args []string) error {

	cfg := cli.Config

	ctx, cancel := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer cancel()

	log := logger.Setup()

	//log.Printf("acfg: %+v", cfg)

	if len(cfg.DeploymentMode) == 0 {
		return fmt.Errorf("deployment_mode configuration required")
	}

	cm, err := apitls.GetCertman(cfg.TLS.Cert, cfg.TLS.Key)
	if err != nil {
		log.Error("certificate error", "err", err)
		os.Exit(2)
	}

	dbconn, err := ntpdb.OpenDB(cli.Config.Database)
	if err != nil {
		log.Error("database error", "err", err.Error())
		os.Exit(2)
	}

	scfg := server.Config{
		Listen:        cfg.Listen,
		CertProvider:  cm,
		JWTKey:        cfg.JWTKey,
		DeploymentEnv: cfg.DeploymentMode,
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

	depEnv, err := api.DeploymentEnvironmentFromString(cfg.DeploymentMode)
	if err != nil {
		log.Error("unknown deployment mode", "deployment_mode", cfg.DeploymentMode, "err", err)
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

	topicPrefix := fmt.Sprintf("/%s/monitors", cfg.DeploymentMode)

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
		srv, err := server.NewServer(ctx, log, scfg, dbconn, metricssrv.Registry())
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
