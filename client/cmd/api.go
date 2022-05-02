package cmd

import (
	"context"
	"fmt"
	"log"
	"os"
	"time"

	"github.com/spf13/cobra"

	"go.ntppool.org/monitor/api"
	"go.ntppool.org/monitor/api/pb"
	"go.ntppool.org/monitor/client/auth"
)

func (cli *CLI) apiCmd() *cobra.Command {

	apiCmd := &cobra.Command{
		Use:   "api",
		Short: "API admin commands",
		Long:  ``,
	}
	apiCmd.PersistentFlags().AddGoFlagSet(cli.Config.Flags())
	apiCmd.AddCommand(cli.apiOkCmd())

	return apiCmd
}

func (cli *CLI) apiOkCmd() *cobra.Command {

	apiOkCmd := &cobra.Command{
		Use:   "ok",
		Short: "Check API connection",
		Long:  ``,
		RunE:  cli.Run(cli.apiOK),
	}
	apiOkCmd.PersistentFlags().AddGoFlagSet(cli.Config.Flags())
	return apiOkCmd
}

func (cli *CLI) apiOK(cmd *cobra.Command) error {
	ctx, cancel := context.WithTimeout(context.Background(), time.Second*20)
	defer cancel()

	fmt.Println("Checking API")

	// time.Sleep(20 * time.Second)

	cauth, err := cli.ClientAuth(ctx)
	if err != nil {
		log.Fatalf("auth error: %s", err)
	}

	err = cauth.Login()
	if err != nil {
		log.Printf("Could not authenticate: %s", err)
		os.Exit(2)
	}

	err = cauth.LoadOrIssueCertificates()
	if err != nil {
		log.Printf("getting certificates failed: %s", err)
	}

	ctx, api, err := api.Client(ctx, cli.Config.Name, cauth)
	if err != nil {
		log.Fatalf("Could not setup API: %s", err)
	}

	cfg, err := api.GetConfig(ctx, &pb.GetConfigParams{})
	if err != nil {
		log.Fatalf("Could not get config: %s", err)
	}

	if cfg.Samples > 0 {
		fmt.Printf("Ok\n")
	}

	return nil

}

func (cli *CLI) ClientAuth(ctx context.Context) (*auth.ClientAuth, error) {
	cfg := cli.Config
	stateDir := cfg.StateDir
	name := cfg.Name

	log.Printf("Configuring name: %s (%s)", name, cfg.API.Key)

	cauth, err := auth.New(ctx, stateDir, name, cfg.API.Key, cfg.API.Secret)
	if err != nil {
		return nil, err
	}

	return cauth, nil
}
