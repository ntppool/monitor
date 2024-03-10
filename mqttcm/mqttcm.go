package mqttcm

// "mqtt connection manager"

import (
	"context"
	"crypto/tls"
	"encoding/json"
	"fmt"
	"net/url"
	"strings"
	"time"

	"github.com/eclipse/paho.golang/autopaho"
	"github.com/eclipse/paho.golang/paho"

	"go.ntppool.org/common/logger"
	"go.ntppool.org/common/version"
	apitls "go.ntppool.org/monitor/api/tls"
	"go.ntppool.org/monitor/client/config"
)

func Setup(ctx context.Context, name, statusChannel string, subscribe []string, router paho.Router, conf config.MQConfigger, cp apitls.CertificateProvider) (*autopaho.ConnectionManager, error) {
	log := logger.Setup()

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

	log.InfoContext(ctx, "mqtt", "clientID", clientID)

	publishOnlineMessage := func(cm *autopaho.ConnectionManager) {
		if len(statusChannel) == 0 {
			return
		}
		msg, err := StatusMessageJSON(true)
		if err != nil {
			log.Warn("mqtt status error", "err", err)
		}
		log.Debug("sending mqtt status message", "topic", statusChannel, "msg", msg)
		expireSeconds := uint32(86400)
		_, err = cm.Publish(ctx, &paho.Publish{
			Topic:   statusChannel,
			Payload: msg,
			QoS:     1,
			Retain:  true,
			Properties: &paho.PublishProperties{
				MessageExpiry: &expireSeconds,
			},
		})
		if err != nil {
			log.Warn("mqtt status publish error", "err", err)
		}
	}

	offlineMessage, err := StatusMessageJSON(false)
	if err != nil {
		return nil, fmt.Errorf("status message: %w", err)
	}

	// log.Info("status for will message", "topic", statusChannel, "msg", offlineMessage)

	willMessage := &paho.WillMessage{
		// QoS:     1,
		Retain:  true,
		Topic:   statusChannel,
		Payload: offlineMessage,
	}

	mqttcfg := autopaho.ClientConfig{
		ServerUrls:                    []*url.URL{broker},
		CleanStartOnInitialConnection: true,
		SessionExpiryInterval:         60,
		TlsCfg:                        tlsConfig,
		KeepAlive:                     120,

		ConnectPacketBuilder: func(pc *paho.Connect, u *url.URL) *paho.Connect {
			cfg := conf.GetMQTTConfig()
			if cfg != nil {
				log.DebugContext(ctx, "Using JWT to authenticate", "jwt", cfg.JWT)
				pc.Password = cfg.JWT
			}
			return pc
		},
		ConnectUsername: name,
		ConnectPassword: cfg.JWT,

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

			publishOnlineMessage(cm)

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
			// cancel()
		},
		ClientConfig: paho.ClientConfig{
			ClientID: clientID,
			OnClientError: func(err error) {
				log.Error("mqtt client error", "err", err)
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

	if len(statusChannel) > 0 {
		mqttcfg.WillMessage = willMessage
		mqttcfg.WillProperties = &paho.WillProperties{
			WillDelayInterval: paho.Uint32(30),
			MessageExpiry:     paho.Uint32(86400),
		}
	}

	// todo: make this an option
	if true {
		// stdlog := logger.NewStdLog("mqtt debug", true, logger.FromContext(ctx))
		// mqttcfg.Debug = stdlog
		// mqttcfg.PahoDebug = stdlog

		errlog := logger.NewStdLog("mqtt error", true, logger.FromContext(ctx))
		mqttcfg.Errors = errlog
		mqttcfg.PahoErrors = errlog
	}

	cm, err := autopaho.NewConnection(ctx, mqttcfg)
	if err != nil {
		return cm, err
	}

	// if err = cm.AwaitConnection(ctx); err != nil {
	// 	log.ErrorContext(ctx, "mqtt awaitconnection", "err", err)
	// 	return cm, err
	// }

	go func() {
		for {
			select {
			case <-time.After(1 * time.Hour):
				publishOnlineMessage(cm)
			case <-cm.Done():
				return
			}
		}
	}()

	return cm, err
}

type StatusMessage struct {
	Online    bool
	Version   version.Info
	UpdatedMQ time.Time
}

func StatusMessageJSON(online bool) ([]byte, error) {
	sm := &StatusMessage{
		Online:    online,
		Version:   version.VersionInfo(),
		UpdatedMQ: time.Now().Truncate(time.Second),
	}
	js, err := json.Marshal(sm)
	if err != nil {
		return nil, err
	}
	return js, err

}
