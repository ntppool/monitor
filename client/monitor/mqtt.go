package monitor

import (
	"context"
	"encoding/json"
	"fmt"
	"log/slog"
	"net/netip"
	"time"

	"github.com/eclipse/paho.golang/autopaho"
	"github.com/eclipse/paho.golang/packets"
	"github.com/eclipse/paho.golang/paho"
	"go.opentelemetry.io/otel"
	"go.opentelemetry.io/otel/propagation"

	"go.ntppool.org/common/tracing"
	"go.ntppool.org/monitor/api"
	"go.ntppool.org/monitor/client/config/checkconfig"
	"go.ntppool.org/monitor/mqttcm"
)

type mqclient struct {
	mq     *autopaho.ConnectionManager
	topics *mqttcm.MQTTTopics
	conf   checkconfig.ConfigGetter
	log    *slog.Logger
}

func NewMQClient(log *slog.Logger, topics *mqttcm.MQTTTopics, conf checkconfig.ConfigGetter) *mqclient {
	return &mqclient{topics: topics, conf: conf, log: log}
}

func (mqc *mqclient) SetMQ(mq *autopaho.ConnectionManager) {
	mqc.mq = mq
}

func (mqc *mqclient) Handler(m *paho.Publish) {
	log := mqc.log

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	log.Debug("mqtt client message", "topic", m.Topic, "payload", m.Payload, "properties", m.Properties)

	tracePropagator := otel.GetTextMapPropagator()

	traceCarrier := propagation.MapCarrier{}
	for _, f := range tracePropagator.Fields() {
		traceCarrier[f] = m.Properties.User.Get(f)
	}

	ctx = tracePropagator.Extract(ctx, traceCarrier)
	ctx, span := tracing.Start(ctx, "mqtt")
	defer span.End()

	requestType, _, err := mqc.topics.ParseRequestTopic(m.Topic)
	if err != nil {
		log.Error("mqtt request error", "err", err)
		return
	}

	span.SetName("mqtt-" + requestType)

	switch requestType {

	case "metrics":
		// Return empty metrics set as requested during Prometheus to OTel migration
		log.Debug("metrics request received, returning empty metrics set")

		err := mqc.sendResponse(ctx, log, []byte{}, m)
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

	if err := mqc.mq.AwaitConnection(ctx); err != nil {
		return fmt.Errorf("failed to establish MQTT connection: %w", err)
	}
	log.Debug("connection for response established")

	_, err := mqc.mq.Publish(ctx, rmsg)
	return err
}
