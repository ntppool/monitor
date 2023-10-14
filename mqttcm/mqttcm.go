package mqttcm

// "mqtt connection manager"

import (
	"context"
	"crypto/tls"
	"encoding/json"
	"fmt"
	"net/url"
	"strings"

	"github.com/eclipse/paho.golang/autopaho"
	"github.com/eclipse/paho.golang/paho"

	"go.ntppool.org/common/logger"
	"go.ntppool.org/common/version"
	apitls "go.ntppool.org/monitor/api/tls"
	"go.ntppool.org/monitor/client/config"
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

	log := logger.Setup()

	mqttcfg := autopaho.ClientConfig{
		BrokerUrls: []*url.URL{broker},
		TlsCfg:     tlsConfig,
		OnConnectionUp: func(cm *autopaho.ConnectionManager, connAck *paho.Connack) {

			log.Info("mqtt connection up")

			if len(subscribe) > 0 {

				subscriptions := []paho.SubscribeOptions{}
				for _, s := range subscribe {
					subscriptions = append(subscriptions, paho.SubscribeOptions{
						Topic: s,
						QoS:   1,
					})
				}

				suback, err := cm.Subscribe(context.Background(), &paho.Subscribe{
					Subscriptions: subscriptions})

				if err != nil {
					if suback == nil {
						log.Error("mqtt subscribe error", "err", err)
					} else {
						if suback.Properties != nil {
							log.Error("mqtt subscribe error", "err", err, "reason", suback.Properties.ReasonString)
						} else {
							log.Error("mqtt subscribe error", "err", err, "reasons", suback.Reasons)
						}
					}
					return
				}
				log.Debug("mqtt subscription setup")
			}

			if len(statusChannel) > 0 {
				msg, err := StatusMessageJSON(true)
				if err != nil {
					log.Warn("mqtt status error", "err", err)
				}
				log.Debug("sending mqtt status message", "topic", statusChannel, "msg", msg)
				_, err = cm.Publish(ctx, &paho.Publish{
					Topic:   statusChannel,
					Payload: msg,
					QoS:     1,
					Retain:  true,
				})
				if err != nil {
					log.Warn("mqtt status publish error", "err", err)
				}
			}

			// old, clear retained message
			// oldChannel := fmt.Sprintf("%s/status/%s/online", cfg.MQTTConfig.Prefix, cauth.Name)
			// for _, qos := range []byte{0, 1, 2} {
			// 	mq.Publish(ctx, &paho.Publish{
			// 		Topic:   oldChannel,
			// 		Payload: []byte{},
			// 		QoS:     qos,
			// 		Retain:  true,
			// 	})
			// }

		},
		OnConnectError: func(err error) {
			log.Error("mqtt connect", "err", err)
		},
		ClientConfig: paho.ClientConfig{
			ClientID: clientID,
			OnClientError: func(err error) {
				log.Error("mqtt server requested disconnect (client error)", "err", err)
			},
			OnServerDisconnect: func(d *paho.Disconnect) {
				if d.Properties != nil {
					log.Error("mqtt server requested disconnect", "reason", d.Properties.ReasonString)
				} else {
					log.Error("mqtt server requested disconnect", "reasonCode", d.ReasonCode)
				}
			},
		},
	}

	if router != nil {
		mqttcfg.Router = router
	} else {
		mqttcfg.Router = paho.NewSingleHandlerRouter(func(m *paho.Publish) {
			log.Info("mqtt message (unhandled)", "topic", m.Topic, "payload", m.Payload)
			// h.handle(m)
		})
	}

	// todo: this makes verbose debugging on the server, disable it
	// completely or make it an option
	if len(subscribe) > 0 {
		stdlog := logger.NewStdLog("mqtt debug", true, logger.FromContext(ctx))
		mqttcfg.Debug = stdlog
		//  mqttcfg.PahoDebug = log.Default()
	}

	mqttcfg.SetConnectPacketConfigurator(func(pc *paho.Connect) *paho.Connect {
		cfg := conf.GetMQTTConfig()
		if cfg != nil {
			log.Debug("Using JWT to authenticate", "jwt", cfg.JWT)
			pc.Password = cfg.JWT
		}
		return pc
	})

	mqttcfg.SetUsernamePassword(name, conf.GetMQTTConfig().JWT)

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
