package cmd

import (
	"fmt"
	"log"
	"net/http"
	"os"
	"time"

	"database/sql"

	apitls "go.ntppool.org/monitor/api/tls"
	"go.ntppool.org/monitor/ntpdb"
	"go.ntppool.org/monitor/server"

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

func (cli *CLI) serverCLI(cmd *cobra.Command) error {

	cfg := cli.Config

	log.Printf("loaded? %t", cfg.loaded)
	log.Printf("acfg: %+v", cfg)

	cm, err := apitls.GetCertman(cfg.TLS.Cert, cfg.TLS.Key)
	if err != nil {
		log.Fatal(err)
		os.Exit(2)
	}

	dbconn := sql.OpenDB(ntpdb.Driver{CreateConnectorFunc: createConnector(cfg)})
	if err != nil {
		log.Fatalf(err.Error())
	}

	dbconn.SetConnMaxLifetime(time.Minute * 3)
	dbconn.SetMaxOpenConns(10)
	dbconn.SetMaxIdleConns(5)

	err = dbconn.Ping()
	if err != nil {
		log.Printf("Could not connect to database: %s", err)
		os.Exit(2)
	}

	scfg := server.Config{
		Listen:       cfg.Listen,
		CertProvider: cm,
	}

	go healthCheckListener()

	srv, err := server.NewServer(scfg, dbconn)
	if err != nil {
		return fmt.Errorf("srv setup: %s", err)
	}
	return srv.Run()
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