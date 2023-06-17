package cmd

import (
	"context"
	"crypto/x509"
	"fmt"
	"net/http"
	"os"
	"time"

	"go.ntppool.org/monitor/api"
	"go.ntppool.org/monitor/api/pb"
	apitls "go.ntppool.org/monitor/api/tls"
	"go.ntppool.org/monitor/logger"
	"go.ntppool.org/monitor/mqttcm"
	"go.ntppool.org/monitor/ntpdb"
	"go.ntppool.org/monitor/server"
	sctx "go.ntppool.org/monitor/server/context"
	"go.ntppool.org/monitor/server/jwt"
	"go.ntppool.org/monitor/server/mqserver"
	"golang.org/x/exp/slog"
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
	c *pb.MQTTConfig
}

func (mqcfg *mqconfig) GetMQTTConfig() *pb.MQTTConfig {
	return mqcfg.c
}

func (cli *CLI) serverCLI(cmd *cobra.Command, args []string) error {

	cfg := cli.Config
	ctx := context.Background()

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

	// todo: move this into mqconfig{} so the key gets regenerated more often
	jwttoken, err := jwt.GetToken(cfg.JWTKey, tlsName, true)
	if err != nil {
		log.Error("jwt token", "err", err)
		os.Exit(2)
	}

	mqcfg := &pb.MQTTConfig{
		Host: []byte("mqtt.ntppool.net"),
		Port: 1883,
		JWT:  []byte(jwttoken),
	}

	mqs, err := mqserver.Setup(log, dbconn)
	if err != nil {
		return err
	}

	topicPrefix := fmt.Sprintf("/%s/monitors", cfg.DeploymentMode)

	router := mqs.MQTTRouter(ctx, topicPrefix)

	mq, err := mqttcm.Setup(
		ctx, tlsName,
		"",                           // status channel
		[]string{topicPrefix + "/#"}, // subscriptions
		router, &mqconfig{c: mqcfg}, cm,
	)
	if err != nil {
		// todo: autopaho should handle reconnecting, so temporary errors
		// aren't fatal -- unclear if they error out and should be ignored
		// and Setup() only returns errors that are unrecoverable (and thus
		// worth not starting on).
		log.Error("could not setup mqtt connection", "err", err)
	}

	mqs.SetConnectionManager(mq)

	ctx, cancel := context.WithCancel(ctx)
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
	go healthCheckListener(ctx, log)

	g.Go(func() error {
		srv, err := server.NewServer(ctx, log, scfg, dbconn)
		if err != nil {
			log.Error("NewServer() error", "err", err)
			return fmt.Errorf("srv setup: %s", err)
		}
		return srv.Run()
	})

	err = g.Wait()
	if err != nil {
		log.Error("server error", "err", err)
	}

	mq.Disconnect(ctx)

	cancel()

	return err

}

func healthCheckListener(ctx context.Context, log *slog.Logger) {
	serveMux := http.NewServeMux()

	serveMux.HandleFunc("/__health", func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(200)
		w.Write([]byte("ok"))
	})

	srv := &http.Server{
		Addr:         ":8080",
		ReadTimeout:  10 * time.Second,
		WriteTimeout: 20 * time.Second,
		IdleTimeout:  120 * time.Second,
		Handler:      serveMux,
	}

	go func() {

		err := srv.ListenAndServe()
		if err != http.ErrServerClosed {
			log.Warn("health check server done listening: %s", err)
		}
	}()

	<-ctx.Done()

	if err := srv.Shutdown(ctx); err != nil {
		log.Error("health check server shutdown Failed:%+v", err)
	}

}
