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
	"github.com/eclipse/paho.golang/packets"
	"github.com/eclipse/paho.golang/paho"

	"go.ntppool.org/common/logger"
	"go.ntppool.org/common/version"
	apitls "go.ntppool.org/monitor/api/tls"
	"go.ntppool.org/monitor/client/config/checkconfig"
)

// statusMessageExpiry is the MQTT message expiry, in seconds, for the
// retained online and offline status messages.
const statusMessageExpiry = 86400

// Setup creates an autopaho connection manager with session-takeover backoff
// wired in. If state is nil, Setup uses a throwaway state internal to this
// connection. Callers that need to observe takeover state from outside (e.g.
// to pause work) pass a non-nil state they own.
func Setup(ctx context.Context, name, statusChannel string, subscribe []string, router paho.Router, conf checkconfig.MQConfigger, cp apitls.AuthProvider, state *TakeoverState) (*autopaho.ConnectionManager, error) {
	log := logger.Setup()

	cfg := conf.GetMQTTConfig()

	capool, err := apitls.CAPool()
	if err != nil {
		return nil, err
	}

	if cp == nil {
		return nil, apitls.ErrNoAuthProvider
	}

	if state == nil {
		state = NewTakeoverState()
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

	// log.DebugContext(ctx, "mqtt", "clientID", clientID)

	publishOnlineMessage := func(cm *autopaho.ConnectionManager) {
		if len(statusChannel) == 0 {
			return
		}
		msg, err := StatusMessageJSON(true)
		if err != nil {
			// Don't publish an empty retained message; it would clear the status.
			log.Warn("mqtt status error", "err", err)
			return
		}
		log.Debug("sending mqtt status message", "topic", statusChannel, "msg", msg)
		_, err = cm.Publish(ctx, &paho.Publish{
			Topic:   statusChannel,
			Payload: msg,
			QoS:     1,
			Retain:  true,
			Properties: &paho.PublishProperties{
				MessageExpiry: paho.Uint32(statusMessageExpiry),
			},
		})
		if err != nil {
			log.Warn("mqtt status publish error", "err", err)
		}
	}

	mqttcfg := autopaho.ClientConfig{
		ServerUrls:                    []*url.URL{broker},
		CleanStartOnInitialConnection: true,
		SessionExpiryInterval:         60,
		TlsCfg:                        tlsConfig,
		KeepAlive:                     120,

		ConnectPacketBuilder: func(pc *paho.Connect, u *url.URL) (*paho.Connect, error) {
			cfg := conf.GetMQTTConfig()
			if cfg != nil {
				// log.DebugContext(ctx, "Using JWT to authenticate", "jwt", cfg.JWT)
				pc.Password = cfg.JWT
			}
			return pc, nil
		},
		ConnectUsername: name,
		ConnectPassword: cfg.JWT,

		OnConnectionUp: func(cm *autopaho.ConnectionManager, connAck *paho.Connack) {
			log.Info("mqtt connection up", "clientID", clientID)
			state.markConnectionUp()

			if len(subscribe) > 0 {

				subscriptions := []paho.SubscribeOptions{}
				for _, s := range subscribe {
					subscriptions = append(subscriptions, paho.SubscribeOptions{
						Topic: s,
						QoS:   1,
					})
				}

				suback, err := cm.Subscribe(context.Background(), &paho.Subscribe{
					Subscriptions: subscriptions,
				})
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
				log.Info("MQTT subscription completed successfully", "reasons", suback.Reasons)
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
				reason := ""
				if d.Properties != nil {
					reason = d.Properties.ReasonString
				}
				label := ""
				if d.ReasonCode == packets.DisconnectSessionTakenOver {
					label = "session_taken_over"
				}
				log.Error("mqtt server requested disconnect",
					"reasonCode", d.ReasonCode,
					"reason", reason,
					"label", label)
				state.recordDisconnect(d.ReasonCode)
			},
		},
	}

	mqttcfg.ReconnectBackoff = func(attempt int) time.Duration {
		delay := state.reconnectDelay()
		if delay > defaultReconnectDelay {
			log.Warn("mqtt reconnect delayed due to session takeover",
				"delay", delay,
				"attempt", attempt)
		}
		return delay
	}

	if router != nil {
		mqttcfg.Router = router
	} else {
		mqttcfg.Router = paho.NewStandardRouterWithDefault(func(m *paho.Publish) {
			log.Info("mqtt message received by DEFAULT HANDLER (this indicates routing problem)", "topic", m.Topic, "payload", string(m.Payload))
			// h.handle(m)
		})
	}

	if len(statusChannel) > 0 {
		offlineMessage, err := StatusMessageJSON(false)
		if err != nil {
			return nil, fmt.Errorf("status message: %w", err)
		}
		setOfflineWill(&mqttcfg, statusChannel, offlineMessage)
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
			case <-time.After(5 * time.Minute):
				publishOnlineMessage(cm)
			case <-cm.Done():
				return
			}
		}
	}()

	return cm, err
}

// setOfflineWill sets the retained "offline" status as the will message and
// makes a clean disconnect send it too. With MQTT 5 the broker discards the
// will on a DISCONNECT with reason code 0, so a normal stop would otherwise
// never report the client offline.
//
// The broker waits WillDelayInterval and drops the will if the client resumes
// the session in that time. A new process connects with Clean Start set
// (CleanStartOnInitialConnection), which ends the old session, so the broker
// publishes the will right away and the new process then publishes "online".
func setOfflineWill(cfg *autopaho.ClientConfig, statusChannel string, offlineMessage []byte) {
	cfg.WillMessage = &paho.WillMessage{
		Retain:  true,
		Topic:   statusChannel,
		Payload: offlineMessage,
	}
	cfg.WillProperties = &paho.WillProperties{
		WillDelayInterval: paho.Uint32(30),
		MessageExpiry:     paho.Uint32(statusMessageExpiry),
	}
	cfg.DisconnectPacketBuilder = func() *paho.Disconnect {
		return &paho.Disconnect{ReasonCode: packets.DisconnectDisconnectWithWillMessage}
	}
}

// StatusMessage is the retained status an agent publishes on its status
// topic. UpdatedMQ is when an "online" status was sent. The "offline" status
// is the will, built when the connection is set up and published by the
// broker later, so it has no UpdatedMQ.
type StatusMessage struct {
	Online    bool
	Version   version.Info
	UpdatedMQ time.Time `json:",omitzero"`
}

func StatusMessageJSON(online bool) ([]byte, error) {
	sm := &StatusMessage{
		Online:  online,
		Version: version.VersionInfo(),
	}
	if online {
		sm.UpdatedMQ = time.Now().Truncate(time.Second)
	}
	return json.Marshal(sm)
}
