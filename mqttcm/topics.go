package mqttcm

import (
	"fmt"
	"log"
	"strings"

	"go.ntppool.org/monitor/api"
)

type MQTTTopics struct {
	e api.DeploymentEnvironment
}

func NewTopics(depEnv api.DeploymentEnvironment) *MQTTTopics {
	return &MQTTTopics{e: depEnv}
}

func (t *MQTTTopics) prefix() string {
	return fmt.Sprintf("/%s/monitors", t.e)
}

func (t *MQTTTopics) StatusSubscription() string {
	return fmt.Sprintf("%s/status/#", t.prefix())
}

func (t *MQTTTopics) StatusAPITest(name string) string {
	return fmt.Sprintf("%s/status/%s/api-test", t.prefix(), name)
}

func (t *MQTTTopics) Status(name string) string {
	return fmt.Sprintf("%s/status/%s/status", t.prefix(), name)
}

func (t *MQTTTopics) DataResponse(name, id string) string {
	return fmt.Sprintf("%s/data/%s/%s", t.prefix(), name, id)
}

func (t *MQTTTopics) DataResponseSubscription() string {
	return fmt.Sprintf("%s/data/#", t.prefix())
}

func (t *MQTTTopics) Request(name, check string) string {
	return fmt.Sprintf("%s/requests/%s/%s", t.prefix(), name, check)
}

func (t *MQTTTopics) RequestSubscription(name string) string {
	return fmt.Sprintf("%s/requests/%s/+", t.prefix(), name)
}

func (t *MQTTTopics) ParseRequestTopic(topic string) (string, string, error) {
	// /devel/monitors/requests/uspao1-21wase0.devel.mon.ntppool.dev/ntp
	topic = strings.TrimPrefix(topic, t.prefix())
	p := strings.Split(topic, "/")

	log.Printf("P: %+v %d", p, len(p))

	if len(p) < 4 {
		return "", "", fmt.Errorf("could not parse request topic: %q", topic)
	}
	return p[3], p[2], nil

}
