package cmd

import (
	"context"
	"fmt"
	"log"
	"os"
	"time"

	"github.com/spf13/cobra"
	"github.com/twitchtv/twirp"

	"go.ntppool.org/monitor/api"
	"go.ntppool.org/monitor/api/pb"
	"go.ntppool.org/monitor/client/auth"
	"go.ntppool.org/monitor/mqtt"
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

	timeout := time.Second * 20
	timeout = time.Minute * 5

	ctx, cancel := context.WithTimeout(context.Background(), timeout)
	defer cancel()

	log.Println("Checking API")
	cauth, err := cli.ClientAuth(ctx)
	if err != nil {
		log.Fatalf("auth setup error: %s", err)
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

	secretInfo, err := cauth.Vault.SecretInfo(ctx, cli.Config.Name)
	if err != nil {
		log.Fatalf("Could not get secret metadata: %s", err)
	}

	log.Printf("API key expires %s, created %s, remaining uses: %s", secretInfo["expiration_time"], secretInfo["creation_time"], secretInfo["secret_id_num_uses"])

	ctx, api, err := api.Client(ctx, cli.Config.Name, cauth)
	if err != nil {
		log.Fatalf("Could not setup API: %s", err)
	}

	cfg, err := api.GetConfig(ctx, &pb.GetConfigParams{})
	if err != nil {
		if twerr, ok := err.(twirp.Error); ok {
			if twerr.Code() == twirp.PermissionDenied {
				log.Fatalf("could not get config: %s", twerr.Msg())
			}
		}
		log.Fatalf("could not get config: %s", err)
	}

	if cfg.Samples > 0 {
		log.Println("Got valid config; API access validated")
	}

	var mq *mqtt.MQTT

	if cfg.MQTTConfig != nil && len(cfg.MQTTConfig.Host) > 0 {

		statusChannel := fmt.Sprintf("/devel/monitors/status/%s", cauth.Name)

		mq, err = mqtt.New(cauth.Name, statusChannel, cfg, cauth)
		if err != nil {
			log.Fatalf("mqtt: %s", err)
		}
		log.Printf("mq: %+v", mq)
		ok, token := mq.Connect()
		if ok {
			log.Printf("mq connected!")
		} else {
			token.WaitTimeout(5 * time.Second)
			if err := token.Error(); err != nil {
				log.Printf("mqtt connect error: %s", err)
			} else {
				log.Printf("publishing")
				msg := fmt.Sprintf(
					"online - %s", time.Now(),
				)
				token := mq.Publish(statusChannel, 1, true, msg)
				token.WaitTimeout(5 * time.Second)
				if err := token.Error(); err != nil {
					log.Printf("mqtt publish error: %s", err)
				} else {
					log.Printf("published")
				}
			}
		}

		log.Printf("sending offline message")
		token = mq.Publish(statusChannel+"/bye", 1, true, "offline")
		log.Printf("offline message queued")

		token.WaitTimeout(2 * time.Second)
		if err := token.Error(); err != nil {
			log.Printf("mqtt publish error: %s", err)
		}
		log.Printf("offline token done")

		time.Sleep(2 * time.Second)
		os.Exit(0)
	}

	cancel()

	// mq.Disconnect()

	time.Sleep(time.Millisecond * 2000)

	log.Printf("done!")

	return nil
}

func (cli *CLI) ClientAuth(ctx context.Context) (*auth.ClientAuth, error) {
	cfg := cli.Config
	stateDir := cfg.StateDir
	name := cfg.Name

	log.Printf("Configuring %s (%s)", name, cfg.API.Key)

	cauth, err := auth.New(ctx, stateDir, name, cfg.API.Key, cfg.API.Secret)
	if err != nil {
		return nil, err
	}

	return cauth, nil
}
