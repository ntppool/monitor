package mqtt

import (
	"crypto/tls"
	"fmt"
	"log"
	"os"
	"strings"
	"time"

	paho "github.com/eclipse/paho.mqtt.golang"

	"go.ntppool.org/monitor/api/pb"
	apitls "go.ntppool.org/monitor/api/tls"
)

type MQTT struct {
	client paho.Client
}

func New(name, willTopic string, cfg *pb.Config, cp apitls.CertificateProvider) (*MQTT, error) {

	paho.ERROR = log.New(os.Stdout, "[ERROR] ", 0)
	paho.CRITICAL = log.New(os.Stdout, "[CRIT] ", 0)
	paho.WARN = log.New(os.Stdout, "[WARN]  ", 0)
	paho.DEBUG = log.New(os.Stdout, "[DEBUG] ", 0)

	capool, err := apitls.CAPool()
	if err != nil {
		return nil, err
	}

	tlsConfig := &tls.Config{
		InsecureSkipVerify:   false,
		GetClientCertificate: cp.GetClientCertificate,
		RootCAs:              capool,
	}

	opts := paho.NewClientOptions().AddBroker(
		fmt.Sprintf("mqtts://%s:%d/", cfg.MQTTConfig.Host, cfg.MQTTConfig.Port),
	)
	opts.SetTLSConfig(tlsConfig)
	opts.SetUsername(name)
	opts.SetPassword(string(cfg.MQTTConfig.JWT))
	log.Printf("setting will on %s", willTopic+"/will")
	opts.SetWill(willTopic, "will-bye", 1, true)
	opts.SetKeepAlive(5 * time.Second)
	opts.SetCleanSession(false)

	if opts.WillEnabled {
		log.Printf("will is enabled")
	}

	time.Sleep(1 * time.Second)

	log.Printf("logging in with name: %s", name)
	log.Printf("logging in with pass: %q", string(cfg.MQTTConfig.JWT))

	clientID := name

	if idx := strings.Index(clientID, ".mon.ntppool.dev"); idx > 0 {
		clientID = clientID[:idx]
	}

	opts.SetClientID(clientID)

	opts.SetDefaultPublishHandler(func(client paho.Client, msg paho.Message) {
		fmt.Printf("P TOPIC: %s\n", msg.Topic())
		fmt.Printf("P MSG: %s\n", msg.Payload())
	})

	opts.SetWill("/devel/monitors/monitor-status", "disconnected client!", 1, true)

	client := paho.NewClient(opts)
	return &MQTT{client: client}, nil
}

func (m *MQTT) Connect() (bool, paho.Token) {
	if m.client.IsConnected() {
		return true, nil
	}
	token := m.client.Connect()
	return false, token
}

func (m *MQTT) Disconnect() {
	m.client.Disconnect(2000)
}

func (m *MQTT) Publish(topic string, qos byte, retained bool, payload interface{}) paho.Token {
	return m.client.Publish(topic, qos, retained, payload)
}
