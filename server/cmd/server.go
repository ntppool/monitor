package cmd

import (
	"context"
	"crypto/x509"
	"fmt"
	"log"
	"net/http"
	"time"

	"go.ntppool.org/monitor/api"
	"go.ntppool.org/monitor/api/pb"
	apitls "go.ntppool.org/monitor/api/tls"
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

func (cli *CLI) serverCLI(cmd *cobra.Command, args []string) error {

	cfg := cli.Config
	ctx := context.Background()

	log.Printf("acfg: %+v", cfg)

	if len(cfg.DeploymentMode) == 0 {
		return fmt.Errorf("deployment_mode configuration required")
	}

	cm, err := apitls.GetCertman(cfg.TLS.Cert, cfg.TLS.Key)
	if err != nil {
		log.Fatal(err)
	}

	dbconn, err := ntpdb.OpenDB(cli.Config.Database)
	if err != nil {
		log.Fatalf(err.Error())
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
			log.Printf("cert: %+v", parsed)
			return "", fmt.Errorf("no certificate?")
		}

		for _, dnsName := range parsed.DNSNames {
			if len(r) == 0 {
				r = dnsName
			}
			log.Printf("dnsName from cert: %s", dnsName)
		}
		return r, nil
	}()
	if err != nil {
		return err
	}

	depEnv, err := api.DeploymentEnvironmentFromString(cfg.DeploymentMode)
	if err != nil {
		log.Fatalf("unknown deployment mode %q: %s", cfg.DeploymentMode, err)
	}

	ctx = context.WithValue(ctx, sctx.DeploymentEnv, depEnv)

	jwttoken, err := jwt.GetToken(cfg.JWTKey, tlsName, true)
	if err != nil {
		log.Fatalf("jwt token: %s", err)
	}

	mqcfg := &pb.MQTTConfig{
		Host: []byte("mqtt.ntppool.net"),
		Port: 1883,
		JWT:  []byte(jwttoken),
	}

	mqs, err := mqserver.Setup()
	if err != nil {
		return err
	}

	topicPrefix := fmt.Sprintf("/%s/monitors", cfg.DeploymentMode)

	log.Printf("ctx: %+v", ctx)

	router := mqs.MQTTRouter(ctx, topicPrefix)

	mq, err := mqttcm.Setup(
		ctx, tlsName,
		"",                           // status channel
		[]string{topicPrefix + "/#"}, // subscriptions
		router, mqcfg, cm,
	)
	if err != nil {
		// todo: autopaho should handle reconnecting, so temporary errors
		// aren't fatal -- unclear if they error out and should be ignored
		// and Setup() only returns errors that are unrecoverable (and thus
		// worth not starting on).
		log.Printf("could not setup mqtt connection: %s", err)
	}

	mqs.SetConnectionManager(mq)

	ctx, cancel := context.WithCancel(ctx)
	g, ctx := errgroup.WithContext(ctx)

	g.Go(func() error {
		<-mq.Done()
		log.Printf("mqtt connection done")
		return nil
	})

	g.Go(func() error {
		return mqs.Run(ctx)
	})

	// todo: ctx + errgroup
	go healthCheckListener()

	log.Printf("xxx NewServer next")

	g.Go(func() error {
		log.Printf("NewServer()")
		srv, err := server.NewServer(scfg, dbconn)
		if err != nil {
			log.Printf("NewServer() error: %s", err)
			return fmt.Errorf("srv setup: %s", err)
		}
		log.Printf("xxx Run() next")
		return srv.Run(ctx)
	})

	log.Printf("Wait()'ing")

	err = g.Wait()
	if err != nil {
		log.Printf("server error: %s", err)
	}

	mq.Disconnect(ctx)

	cancel()

	return err

}

func healthCheckListener() {
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

		Handler: serveMux,
	}

	err := srv.ListenAndServe()
	log.Printf("http server done listening: %s", err)
}
