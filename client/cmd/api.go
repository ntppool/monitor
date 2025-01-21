package cmd

import (
	"context"
	"fmt"
	"os"
	"time"

	"connectrpc.com/connect"
	"github.com/eclipse/paho.golang/autopaho"
	"github.com/eclipse/paho.golang/paho"
	"github.com/spf13/cobra"

	"go.ntppool.org/common/logger"
	"go.ntppool.org/common/tracing"
	"go.ntppool.org/monitor/api"
	"go.ntppool.org/monitor/client/auth"
	"go.ntppool.org/monitor/client/config"
	apiv2 "go.ntppool.org/monitor/gen/monitor/v2"
	"go.ntppool.org/monitor/mqttcm"
)

func (cli *CLI) apiCmd() *cobra.Command {
	apiCmd := &cobra.Command{
		Use:   "api",
		Short: "API admin commands",
		Long:  ``,
	}
	apiCmd.PersistentFlags().AddGoFlagSet(cli.Flags())
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
	apiOkCmd.PersistentFlags().AddGoFlagSet(cli.Flags())
	return apiOkCmd
}

func (cli *CLI) apiOK(cmd *cobra.Command, _ []string) error {
	log := logger.SetupMultiLogger()

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

	tracingShutdown, err := InitTracing(cli.Config.Name(), cauth)
	if err != nil {
		log.Error("tracing error", "err", err)
	}
	defer func() {
		time.Sleep(3 * time.Second)
		err := tracingShutdown(context.Background())
		if err != nil {
			log.Info("tracing shutdown", "err", err)
		}
	}()

	ctx, span := tracing.Start(ctx, "api-test")
	defer span.End()

	secretInfo, err := cauth.Vault.SecretInfo(ctx, cli.Config.Name())
	if err != nil {
		log.ErrorContext(ctx, "Could not get metadata for API secret", "err", err)
	}

	log.InfoContext(ctx, "API key information", "expiration", secretInfo["expiration_time"], "created", secretInfo["creation_time"], "remaining_uses", secretInfo["secret_id_num_uses"])

	ctx, apiC, err := api.Client(ctx, cli.Config.Name(), cauth)
	if err != nil {
		log.ErrorContext(ctx, "could not setup API client", "err", err)
	}

	depEnv := cli.Config.Env()

	cfgresp, err := apiC.GetConfig(ctx, connect.NewRequest(&apiv2.GetConfigRequest{}))
	if err != nil {
		if conerr, ok := err.(*connect.Error); ok {
			if conerr.Code() == connect.CodePermissionDenied {
				log.ErrorContext(ctx,
					"permission error getting config",
					"err", conerr.Error(),
				)
			} else {
				log.ErrorContext(ctx,
					"error getting config",
					"code", conerr.Code(),
					"err", conerr.Error(),
				)
			}
		} else {
			log.ErrorContext(ctx, "could not get config", "err", err)
		}
		return nil
	}

	if cfgresp == nil || cfgresp.Msg == nil {
		log.ErrorContext(ctx, "did not receive configuration", "resp", cfgresp)
		return nil
	}

	cfg := cfgresp.Msg

	conf := config.NewConfigger(nil)

	if cfg == nil {
		log.WarnContext(ctx, "didn't get configuration from the API")
	} else {
		conf.SetConfigFromApi(cfg)
		if cfg.Samples > 0 {
			log.InfoContext(ctx, "got valid config; API access validated")
		} else {
			log.InfoContext(ctx, "configuration didn't have samples configured")
		}
	}

	var mq *autopaho.ConnectionManager

	if cfg := conf.GetConfig(); cfg != nil {
		if mqcfg := cfg.MQTTConfig; mqcfg != nil && len(mqcfg.Host) > 0 {
			mq, err = mqttcm.Setup(ctx, cli.Config.Name(), "", []string{}, nil, conf, cauth)
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
				Topic:   topics.StatusAPITest(cli.Config.Name()),
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

	log.InfoContext(ctx, "api test done")

	return nil
}

func (cli *CLI) ClientAuth(ctx context.Context) (*auth.ClientAuth, error) {
	log := logger.FromContext(ctx)

	log.InfoContext(ctx, "configuring authentication")

	cauth, err := auth.New(ctx, cli.Config)
	if err != nil {
		return nil, err
	}

	return cauth, nil
}
