package cmd

import (
	"context"
	"fmt"
	"os"
	"time"

	"connectrpc.com/connect"
	"github.com/eclipse/paho.golang/autopaho"
	"github.com/eclipse/paho.golang/paho"

	"go.ntppool.org/common/logger"
	"go.ntppool.org/common/tracing"
	"go.ntppool.org/monitor/api"
	"go.ntppool.org/monitor/client/config/checkconfig"
	apiv2 "go.ntppool.org/monitor/gen/monitor/v2"
	"go.ntppool.org/monitor/mqttcm"
)

type apiCmd struct {
	Ok apiOkCmd `cmd:"" help:"Check API connection"`
}

type apiOkCmd struct{}

func (cmd *apiOkCmd) Run(ctx context.Context, cli *ClientCmd) error {
	log := logger.SetupMultiLogger()

	timeout := time.Second * 40

	ctx, cancel := context.WithTimeout(ctx, timeout)
	defer cancel()

	log.InfoContext(ctx, "ok command", "env", cli.DeployEnv)

	err := cli.Config.WaitUntilReady(ctx)
	if err != nil {
		log.ErrorContext(ctx, "config not ready", "err", err)
	}

	if !(cli.Config.IPv4().InUse() || cli.Config.IPv6().InUse()) {
		args := []any{}
		if cli.Config.IPv4().IP != nil {
			args = append(args, "ipv4", string(cli.Config.IPv4().Status))
		}
		if cli.Config.IPv6().IP != nil {
			args = append(args, "ipv6", string(cli.Config.IPv6().Status))
		}
		log.ErrorContext(ctx, "monitor isn't active", args...)
		return nil
	}

	_, certNotAfter, _, err := cli.Config.CertificateDates()
	if err != nil {
		log.ErrorContext(ctx, "could not get certificate notAfter date", "err", err)
	} else {
		log.InfoContext(ctx, "certificate expires", "date", certNotAfter)
	}

	tracingShutdown, err := InitTracing(ctx, cli.DeployEnv, cli.Config)
	if err != nil {
		log.ErrorContext(ctx, "tracing error", "err", err)
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

	// todo: print certificate information
	log.InfoContext(ctx, "client name", "name", cli.Config.TLSName())

	ctx, apiC, err := api.Client(ctx, cli.Config.TLSName(), cli.Config)
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

	conf := checkconfig.NewConfigger(nil)

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
			mq, err = mqttcm.Setup(ctx, cli.Config.TLSName(), "", []string{}, nil, conf, nil)
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
				Topic:   topics.StatusAPITest(cli.Config.TLSName()),
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
