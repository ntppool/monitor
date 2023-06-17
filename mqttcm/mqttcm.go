package mqttcm

import (
	"context"
	"crypto/tls"
	"encoding/json"
	"fmt"
	"net/url"
	"strings"

	"github.com/eclipse/paho.golang/autopaho"
	"github.com/eclipse/paho.golang/paho"
	"golang.org/x/exp/slog"

	apitls "go.ntppool.org/monitor/api/tls"
	"go.ntppool.org/monitor/client/config"
	"go.ntppool.org/monitor/logger"
	"go.ntppool.org/monitor/version"
)

func Setup(ctx context.Context, name, statusChannel string, subscribe []string, router paho.Router, conf config.MQConfigger, cp apitls.CertificateProvider) (*autopaho.ConnectionManager, error) {

	cfg := conf.GetMQTTConfig()

	capool, err := apitls.CAPool()
	if err != nil {
		return nil, err
	}

	tlsConfig := &tls.Config{
		InsecureSkipVerify:   false,
		GetClientCertificate: cp.GetClientCertificate,
		RootCAs:              capool,
	}

	broker, err := url.Parse(fmt.Sprintf("mqtts://%s:%d/", cfg.Host, cfg.Port))
	if err != nil {
		return nil, err
	}

	clientID := name

	if idx := strings.Index(clientID, ".mon.ntppool.dev"); idx > 0 {
		clientID = clientID[:idx]
	}

	mqttcfg := autopaho.ClientConfig{
		BrokerUrls: []*url.URL{broker},
		TlsCfg:     tlsConfig,
		OnConnectionUp: func(cm *autopaho.ConnectionManager, connAck *paho.Connack) {
			slog.Info("mqtt connection up")

			if len(subscribe) > 0 {

				subscriptions := map[string]paho.SubscribeOptions{}
				for _, s := range subscribe {
					subscriptions[s] = paho.SubscribeOptions{QoS: 1}
				}

				suback, err := cm.Subscribe(context.Background(), &paho.Subscribe{
					Subscriptions: subscriptions})
				if err != nil {
					if suback.Properties != nil {
						slog.Error("mqtt subscribe error", "err", err, "reason", suback.Properties.ReasonString)
					} else {
						slog.Error("mqtt subscribe error", "err", err, "reasons", suback.Reasons)
					}
					return
				}
				slog.Debug("mqtt subscription setup")
			}
		},
		OnConnectError: func(err error) {
			slog.Error("mqtt connect", "err", err)
		},
		ClientConfig: paho.ClientConfig{
			ClientID: clientID,
			OnClientError: func(err error) {
				slog.Error("mqtt server requested disconnect (client error)", "err", err)
			},
			OnServerDisconnect: func(d *paho.Disconnect) {
				if d.Properties != nil {
					slog.Error("mqtt server requested disconnect", "reason", d.Properties.ReasonString)
				} else {
					slog.Error("mqtt server requested disconnect", "reasonCode", d.ReasonCode)
				}
			},
		},
	}

	if router != nil {
		mqttcfg.Router = router
	} else {
		mqttcfg.Router = paho.NewSingleHandlerRouter(func(m *paho.Publish) {
			slog.Info("mqtt message (unhandled)", "topic", m.Topic, "payload", m.Payload)
			// h.handle(m)
		})
	}

	// todo: this makes verbose debugging on the server, disable it
	// completely or make it an option
	if len(subscribe) > 0 {
		stdlog := logger.NewStdLog("mqtt debug", logger.FromContext(ctx))
		mqttcfg.Debug = stdlog
		//  mqttcfg.PahoDebug = log.Default()
	}

	mqttcfg.SetConnectPacketConfigurator(func(pc *paho.Connect) *paho.Connect {
		cfg := conf.GetMQTTConfig()
		if cfg != nil {
			slog.Debug("Using JWT to authenticate", "jwt", cfg.JWT)
			pc.Password = cfg.JWT
		}
		return pc
	})

	mqttcfg.SetUsernamePassword(name, cfg.JWT)

	offlineMessage, err := StatusMessageJSON(false)
	if err != nil {
		return nil, fmt.Errorf("status message: %w", err)
	}
	mqttcfg.SetWillMessage(statusChannel, offlineMessage, 1, true)

	// log.Printf("mqtt user name: %s", name)
	// log.Printf("mqtt credentials: %q", string(cfg.JWT))

	cm, err := autopaho.NewConnection(ctx, mqttcfg)
	return cm, err
}

type StatusMessage struct {
	Online  bool
	Version version.Info
}

func StatusMessageJSON(online bool) ([]byte, error) {
	sm := &StatusMessage{Online: online, Version: version.VersionInfo()}
	js, err := json.Marshal(sm)
	if err != nil {
		return nil, err
	}
	return js, err

}
