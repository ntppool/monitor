package mqttcm

import (
	"context"
	"crypto/tls"
	"encoding/json"
	"fmt"
	"log"
	"net/url"
	"strings"

	"github.com/eclipse/paho.golang/autopaho"
	"github.com/eclipse/paho.golang/paho"

	"go.ntppool.org/monitor/api/pb"
	apitls "go.ntppool.org/monitor/api/tls"
)

func Setup(ctx context.Context, name, statusChannel string, subscribe []string, router paho.Router, cfg *pb.MQTTConfig, cp apitls.CertificateProvider) (*autopaho.ConnectionManager, error) {

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
			fmt.Println("mqtt connection up")

			if len(subscribe) > 0 {

				subscriptions := map[string]paho.SubscribeOptions{}
				for _, s := range subscribe {
					subscriptions[s] = paho.SubscribeOptions{QoS: 1}
				}

				suback, err := cm.Subscribe(context.Background(), &paho.Subscribe{
					Subscriptions: subscriptions})
				if err != nil {
					log.Printf("failed to subscribe: %s (%s)", err, suback.Properties.ReasonString)
					return
				}
				fmt.Println("mqtt subscription made")
			}
		},
		OnConnectError: func(err error) {
			fmt.Printf("error whilst attempting connection: %s\n", err)
		},
		ClientConfig: paho.ClientConfig{
			ClientID: clientID,
			OnClientError: func(err error) {
				log.Printf("server requested disconnect (client error): %s\n", err)
			},
			OnServerDisconnect: func(d *paho.Disconnect) {
				if d.Properties != nil {
					log.Printf("server requested disconnect: %s\n", d.Properties.ReasonString)
				} else {
					log.Printf("server requested disconnect; reason code: %d\n", d.ReasonCode)
				}
			},
		},
	}

	if router != nil {
		mqttcfg.Router = router
	} else {
		mqttcfg.Router = paho.NewSingleHandlerRouter(func(m *paho.Publish) {
			log.Printf("got message on %q: %s", m.Topic, m.Payload)
			// h.handle(m)
		})
	}

	// todo: this makes verbose debugging on the server, disable it
	// completely or make it an option
	if len(subscribe) > 0 {
		mqttcfg.Debug = log.Default()
		//  mqttcfg.PahoDebug = log.Default()
	}

	mqttcfg.SetConnectPacketConfigurator(func(pc *paho.Connect) *paho.Connect {
		// todo: set pc.Password from recently fetched config
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
	Online bool
}

func StatusMessageJSON(online bool) ([]byte, error) {
	sm := &StatusMessage{Online: online}
	js, err := json.Marshal(sm)
	if err != nil {
		return nil, err
	}
	return js, err

}
