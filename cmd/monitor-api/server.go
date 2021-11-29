package main

import (
	"log"

	"go.ntppool.org/monitor/server"

	"github.com/spf13/cobra"
)

var serverCmd = &cobra.Command{
	Use:   "server",
	Short: "server starts the API server",
	Long:  `starts the API server on (default) port 8000`,
	// Args:  cobra.ExactArgs(1),
	Run: func(cmd *cobra.Command, args []string) {
		listen, err := cmd.Flags().GetString("listen")
		if err != nil {
			log.Fatalf("invalid listen parameter: %s", err)
		}

		tlsconfig := server.TLSConfig{}
		tlsconfig.CAFile, _ = cmd.Flags().GetString("cacert")

		cfg := server.Config{
			Listen:    listen,
			TLSConfig: tlsconfig,
		}

		srv, err := server.NewServer(cfg)
		if err != nil {
			log.Fatalf("srv setup: %s", err)
		}
		err = srv.Run()
		if err != nil {
			log.Fatalf("srv error: %s", err)
		}
	},
}

func init() {
	serverCmd.Flags().String("listen", ":8000", "Listen address")
	serverCmd.Flags().String("cacert", "", "CA certificate path")
	serverCmd.Flags().String("key", "", "Server key path")
	serverCmd.Flags().String("cert", "", "Server certificate path")
	rootCmd.AddCommand(serverCmd)
}
