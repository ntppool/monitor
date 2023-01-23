package monitor

import (
	"context"
	"encoding/json"
	"log"
	"net/netip"
	"time"

	"github.com/eclipse/paho.golang/autopaho"
	"github.com/eclipse/paho.golang/packets"
	"github.com/eclipse/paho.golang/paho"
	"go.ntppool.org/monitor/api"
	"go.ntppool.org/monitor/api/pb"
	"go.ntppool.org/monitor/mqttcm"
)

type mqclient struct {
	mq     *autopaho.ConnectionManager
	topics *mqttcm.MQTTTopics
	cfg    *pb.Config
}

func NewMQClient(topics *mqttcm.MQTTTopics, cfg *pb.Config) *mqclient {
	return &mqclient{topics: topics, cfg: cfg}
}

func (mqc *mqclient) SetMQ(mq *autopaho.ConnectionManager) {
	mqc.mq = mq
}

func (mqc *mqclient) Handler(m *paho.Publish) {

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	log.Printf("mqtt client message on %q: %s\n%+v", m.Topic, m.Payload, m.Properties)

	requestType, _, err := mqc.topics.ParseRequestTopic(m.Topic)
	if err != nil {
		log.Printf("mqtt request error: %s", err)
		return
	}

	msg := struct{ IP string }{}

	err = json.Unmarshal(m.Payload, &msg)
	if err != nil {
		log.Printf("mqtt request error: %s", err)
		return
	}

	if requestType == "ntp" {
		log.Printf("check ntp for %s", msg.IP)

		ip, err := netip.ParseAddr(msg.IP)
		if err != nil {
			log.Printf("mqtt request error, invalid ip %q: %s", msg.IP, err)
			return
		}

		cfg := &pb.Config{
			IPBytes:    mqc.cfg.IPBytes,
			IPNatBytes: mqc.cfg.IPNatBytes,
			Samples:    1,
		}

		_, resp, err := CheckHost(&ip, cfg)
		if err != nil {
			log.Printf("ntp check error %q: %s", msg.IP, err)
			return
		}

		r := &api.NTPResponse{
			NTP:   resp,
			Error: err,
		}

		responseData, err := json.Marshal(r)
		if err != nil {
			log.Printf("json error: %s", err)
			return
		}

		rmsg := &paho.Publish{
			Topic:   m.Properties.ResponseTopic,
			Payload: responseData,
			Retain:  false,
			QoS:     0,
		}
		rmsg.InitProperties(&packets.Properties{})

		for _, t := range []string{"TraceID", "SpanID"} {
			rmsg.Properties.User.Add(t, m.Properties.User.Get(t))
		}

		if mqc.mq == nil {
			log.Printf("mq==nil!")
		}

		mqc.mq.AwaitConnection(ctx)
		log.Printf("have mqtt connection")

		mqc.mq.Publish(ctx, rmsg)

	}

}
