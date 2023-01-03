package cmd

import (
	"context"
	"fmt"
	"log"
	"os"
	"time"

	"github.com/eclipse/paho.golang/autopaho"
	"github.com/eclipse/paho.golang/paho"
	"github.com/spf13/cobra"
	"github.com/twitchtv/twirp"

	"go.ntppool.org/monitor/api"
	"go.ntppool.org/monitor/api/pb"
	"go.ntppool.org/monitor/client/auth"
	"go.ntppool.org/monitor/mqttcm"
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

	var mq *autopaho.ConnectionManager

	if cfg.MQTTConfig != nil && len(cfg.MQTTConfig.Host) > 0 {

		mq, err = mqttcm.Setup(ctx, cauth.Name, "", []string{}, nil, cfg.MQTTConfig, cauth)
		if err != nil {
			log.Fatalf("mqtt: %s", err)
		}
		err := mq.AwaitConnection(ctx)
		if err != nil {
			log.Fatalf("mqtt connection error: %s", err)
		}
		msg := []byte(fmt.Sprintf(
			"API test - %s", time.Now(),
		))
		channel := fmt.Sprintf("%s/status/%s/api-test", cfg.MQTTConfig.Prefix, cauth.Name)

		_, err = mq.Publish(ctx, &paho.Publish{
			QoS:     1,
			Topic:   channel,
			Payload: msg,
			Retain:  false,
		})
		if err != nil {
			log.Printf("publish error: %s", err)
		}

		// log.Printf("sending offline message")
		// token = mq.Publish(statusChannel+"/bye", 1, true, "offline")
		// log.Printf("offline message queued")

	}

	if mq != nil {
		mq.Disconnect(ctx)
	}

	cancel()

	if mq != nil {
		// wait until the mqtt connection is done; or two seconds
		select {
		case <-mq.Done():
		case <-time.After(2 * time.Second):
		}
	}

	log.Printf("done")

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
