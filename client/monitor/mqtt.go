package monitor

import (
	"context"
	"encoding/json"
	"net/netip"
	"time"

	"github.com/eclipse/paho.golang/autopaho"
	"github.com/eclipse/paho.golang/packets"
	"github.com/eclipse/paho.golang/paho"
	"go.ntppool.org/monitor/api"
	"go.ntppool.org/monitor/api/pb"
	"go.ntppool.org/monitor/mqttcm"
	"golang.org/x/exp/slog"
)

type mqclient struct {
	mq     *autopaho.ConnectionManager
	topics *mqttcm.MQTTTopics
	cfg    *pb.Config
	log    *slog.Logger
}

func NewMQClient(log *slog.Logger, topics *mqttcm.MQTTTopics, cfg *pb.Config) *mqclient {
	return &mqclient{topics: topics, cfg: cfg, log: log}
}

func (mqc *mqclient) SetMQ(mq *autopaho.ConnectionManager) {
	mqc.mq = mq
}

func (mqc *mqclient) Handler(m *paho.Publish) {

	mqc.log.Info("mqtt handler active")

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	mqc.log.Info("mqtt client message", "topic", m.Topic, "payload", m.Payload, "properties", m.Properties)

	requestType, _, err := mqc.topics.ParseRequestTopic(m.Topic)
	if err != nil {
		mqc.log.Error("mqtt request error", "err", err)
		return
	}

	msg := struct{ IP string }{}

	err = json.Unmarshal(m.Payload, &msg)
	if err != nil {
		mqc.log.Error("mqtt request error", "err", err)
		return
	}

	if requestType == "ntp" {
		mqc.log.Info("mqtt check ntp", "ip", msg.IP)

		ip, err := netip.ParseAddr(msg.IP)
		if err != nil {
			mqc.log.Error("mqtt request error, invalid ip", "ip", msg.IP, "err", err)
			return
		}

		cfg := &pb.Config{
			IPBytes:    mqc.cfg.IPBytes,
			IPNatBytes: mqc.cfg.IPNatBytes,
			Samples:    1,
		}

		_, resp, err := CheckHost(&ip, cfg)
		if err != nil {
			mqc.log.Error("mqtt ntp check error", "ip", msg.IP, "err", err)
			return
		}

		r := &api.NTPResponse{
			NTP:   resp,
			Error: err,
		}

		responseData, err := json.Marshal(r)
		if err != nil {
			mqc.log.Error("mqtt json error", "err", err)
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
			mqc.log.Error("mqtt mq==nil")
		}

		mqc.mq.AwaitConnection(ctx)
		mqc.log.Info("mqtt connection for response established")

		mqc.mq.Publish(ctx, rmsg)

	}

}
