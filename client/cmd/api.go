package cmd

import (
	"context"
	"fmt"
	"os"
	"time"

	"github.com/eclipse/paho.golang/autopaho"
	"github.com/eclipse/paho.golang/paho"
	"github.com/spf13/cobra"
	"github.com/twitchtv/twirp"

	"go.ntppool.org/common/logger"
	"go.ntppool.org/monitor/api"
	"go.ntppool.org/monitor/api/pb"
	"go.ntppool.org/monitor/client/auth"
	"go.ntppool.org/monitor/client/config"
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

	log := logger.Setup()

	timeout := time.Second * 20
	timeout = time.Minute * 5

	ctx, cancel := context.WithTimeout(context.Background(), timeout)
	defer cancel()

	log.Info("checking API")
	cauth, err := cli.ClientAuth(ctx)
	if err != nil {
		log.Error("auth setup error", "err", err)
		os.Exit(2)
	}

	err = cauth.Login()
	if err != nil {
		log.Error("could not authenticate to API", "err", err)
		os.Exit(2)
	}

	err = cauth.LoadOrIssueCertificates()
	if err != nil {
		log.Error("getting certificates failed", "err", err)
	}

	secretInfo, err := cauth.Vault.SecretInfo(ctx, cli.Config.Name)
	if err != nil {
		log.Error("Could not get metadata for API secret", "err", err)
	}

	log.Info("API key information", "expiration", secretInfo["expiration_time"], "created", secretInfo["creation_time"], "remaining_uses", secretInfo["secret_id_num_uses"])

	ctx, apiC, err := api.Client(ctx, cli.Config.Name, cauth)
	if err != nil {
		log.Error("could not setup API client", "err", err)
	}

	cfg, err := apiC.GetConfig(ctx, &pb.GetConfigParams{})
	if err != nil {
		if twerr, ok := err.(twirp.Error); ok {
			if twerr.Code() == twirp.PermissionDenied {
				log.Error("permission error getting config", "err", twerr.Msg())
			}
		}
		log.Error("could not get config", "err", err)
	}

	depEnv, err := api.GetDeploymentEnvironmentFromName(cli.Config.Name)
	if err != nil {
		log.Error("could not get deployment environment", "err", err)
		os.Exit(2)
	}

	if cfg == nil {
		log.Warn("didn't get configuration from the API")
	} else {
		if cfg.Samples > 0 {
			log.Info("got valid config; API access validated")
		} else {
			log.Info("configuration didn't have samples configured")
		}
	}

	conf := config.NewConfigger(cfg)

	var mq *autopaho.ConnectionManager

	if cfg := conf.GetConfig(); cfg != nil {
		if mqcfg := cfg.MQTTConfig; mqcfg != nil && len(mqcfg.Host) > 0 {
			mq, err = mqttcm.Setup(ctx, cauth.Name, "", []string{}, nil, conf, cauth)
			if err != nil {
				log.Error("mqtt", "err", err)
				os.Exit(2)
			}
			err := mq.AwaitConnection(ctx)
			if err != nil {
				log.Error("mqtt connection error", "err", err)
				os.Exit(2)
			}
			msg := []byte(fmt.Sprintf(
				"API test - %s", time.Now(),
			))

			topics := mqttcm.NewTopics(depEnv)

			_, err = mq.Publish(ctx, &paho.Publish{
				QoS:     1,
				Topic:   topics.StatusAPITest(cauth.Name),
				Payload: msg,
				Retain:  false,
			})
			if err != nil {
				log.Error("mqtt publish error", "err", err)
			}

			// log.Printf("sending offline message")
			// token = mq.Publish(statusChannel+"/bye", 1, true, "offline")
			// log.Printf("offline message queued")

		}
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

	log.Info("api test done")

	return nil
}

func (cli *CLI) ClientAuth(ctx context.Context) (*auth.ClientAuth, error) {
	cfg := cli.Config
	stateDir := cfg.StateDir
	name := cfg.Name

	log := logger.FromContext(ctx)

	log.Info("configuring authentication", "name", name, "api_key", cfg.API.Key)

	cauth, err := auth.New(ctx, stateDir, name, cfg.API.Key, cfg.API.Secret)
	if err != nil {
		return nil, err
	}

	return cauth, nil
}
