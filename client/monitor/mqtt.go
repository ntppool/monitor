package monitor

import (
	"bytes"
	"context"
	"encoding/json"
	"log/slog"
	"net/netip"
	"time"

	"github.com/eclipse/paho.golang/autopaho"
	"github.com/eclipse/paho.golang/packets"
	"github.com/eclipse/paho.golang/paho"

	"github.com/prometheus/client_golang/prometheus"
	"github.com/prometheus/common/expfmt"

	"go.ntppool.org/monitor/api"
	"go.ntppool.org/monitor/client/config"
	"go.ntppool.org/monitor/mqttcm"
)

type mqclient struct {
	mq     *autopaho.ConnectionManager
	topics *mqttcm.MQTTTopics
	conf   config.ConfigUpdater
	log    *slog.Logger
	prom   prometheus.Gatherer
}

func NewMQClient(log *slog.Logger, topics *mqttcm.MQTTTopics, conf config.ConfigUpdater, promreg prometheus.Gatherer) *mqclient {
	return &mqclient{topics: topics, conf: conf, log: log, prom: promreg}
}

func (mqc *mqclient) SetMQ(mq *autopaho.ConnectionManager) {
	mqc.mq = mq
}

func (mqc *mqclient) Handler(m *paho.Publish) {

	log := mqc.log

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	log.Debug("mqtt client message", "topic", m.Topic, "payload", m.Payload, "properties", m.Properties)

	requestType, _, err := mqc.topics.ParseRequestTopic(m.Topic)
	if err != nil {
		log.Error("mqtt request error", "err", err)
		return
	}

	switch requestType {

	case "metrics":

		gathering, err := mqc.prom.Gather()
		if err != nil {
			log.Error("could not fetch metrics", "err", err)
		}

		out := &bytes.Buffer{}
		for _, mf := range gathering {
			if _, err := expfmt.MetricFamilyToText(out, mf); err != nil {
				log.Error("metrics format", "err", err)
			}
		}

		err = mqc.sendResponse(ctx, log, out.Bytes(), m)
		if err != nil {
			log.Error("mqtt response", "err", err)
		}

	case "ntp":
		msg := struct{ IP string }{}

		err = json.Unmarshal(m.Payload, &msg)
		if err != nil {
			log.Error("mqtt request error", "err", err)
			return
		}
		log.Info("mqtt check ntp", "ip", msg.IP)

		ip, err := netip.ParseAddr(msg.IP)
		if err != nil {
			log.Error("request error, invalid ip", "ip", msg.IP, "err", err)
			return
		}

		log.With("ip", ip.String())

		cfg := mqc.conf.GetConfig()
		if cfg == nil {
			log.Error("no config available")
			return
		}
		cfg.Samples = 1

		_, resp, err := CheckHost(ctx, &ip, cfg)
		r := &api.NTPResponse{
			NTP: resp,
		}
		if err != nil {
			log.Info("ntp check error", "err", err)
			r.Error = err.Error()
		} else {
			r.NTP = resp
		}

		responseData, err := json.Marshal(r)
		if err != nil {
			log.Error("json error", "err", err)
			return
		}

		log.Debug("response", "payload", responseData)

		err = mqc.sendResponse(ctx, log, responseData, m)
		if err != nil {
			log.Error("mqtt response", "err", err)
		}

	}

}

func (mqc *mqclient) sendResponse(ctx context.Context, log *slog.Logger, data []byte, m *paho.Publish) error {

	rmsg := &paho.Publish{
		Topic:   m.Properties.ResponseTopic,
		Payload: data,
		Retain:  false,
		QoS:     0,
	}
	rmsg.InitProperties(&packets.Properties{})

	for _, t := range []string{"TraceID", "SpanID"} {
		rmsg.Properties.User.Add(t, m.Properties.User.Get(t))
	}

	if mqc.mq == nil {
		log.Error("mq==nil")
	}

	mqc.mq.AwaitConnection(ctx)
	log.Debug("connection for response established")

	_, err := mqc.mq.Publish(ctx, rmsg)
	return err
}
