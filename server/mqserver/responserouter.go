package mqserver

import (
	"context"
	"fmt"
	"log/slog"
	"strings"
	"sync"

	"github.com/eclipse/paho.golang/paho"
	"go.opentelemetry.io/otel"
	otrace "go.opentelemetry.io/otel/trace"
)

type mqttResponseRouter struct {
	prefix string
	log    *slog.Logger
	rm     map[string]chan<- *paho.Publish
	mu     sync.RWMutex
}

func (mqs *server) setupResponseRouter(ctx context.Context, topicPrefix string) *mqttResponseRouter {
	topicPrefix = strings.TrimSuffix(topicPrefix, "#")
	return &mqttResponseRouter{
		prefix: topicPrefix,
		log:    mqs.log,
		rm:     make(map[string]chan<- *paho.Publish, 100),
	}
}

func (rr *mqttResponseRouter) Handler() paho.MessageHandler {

	prefixLength := len(rr.prefix)
	tp := otel.GetTracerProvider().Tracer("mqserver")

	log := rr.log

	return func(p *paho.Publish) {

		log := log

		ctx := context.Background()

		var traceID otrace.TraceID
		var spanID otrace.SpanID

		if tr := p.Properties.User.Get("TraceID"); len(tr) > 0 {
			traceID, _ = otrace.TraceIDFromHex(tr)
			spanID, _ = otrace.SpanIDFromHex(p.Properties.User.Get("SpanID"))
			log = log.With("traceID", traceID)
		}

		spanContext := otrace.NewSpanContext(otrace.SpanContextConfig{
			TraceID: traceID,
			SpanID:  spanID,
			// TraceFlags: traceFlags,
		})

		ctx = otrace.ContextWithSpanContext(ctx, spanContext)

		ctx, span := tp.Start(ctx, "mqhandler")
		defer span.End()

		if deadline, ok := ctx.Deadline(); ok {
			log.Info("context has deadline", "deadline", deadline)
		}

		log.Debug("handling message", "payload", p)
		span.AddEvent(fmt.Sprintf("handling message: %+v", p))

		topic := p.Topic

		if len(topic) < prefixLength {
			log.Warn("message topic too short", "topic", topic, "len", len(topic))
			return
		}

		topicPath := strings.Split(topic[prefixLength:], "/")

		if len(topicPath) < 2 {
			log.Error("could not get host and id from topic", "topic", topic)
			return
		}

		log.Debug("topic path", "path", topicPath)

		rr.mu.RLock()
		defer rr.mu.RUnlock()

		if ch, ok := rr.rm[topicPath[1]]; ok {
			// todo: include host from topicPath[0]
			ch <- p
		} else {
			log.Warn("no response channel for", "id", topicPath[1])
		}

	}
}

func (rr *mqttResponseRouter) AddResponseID(id string, rc chan<- *paho.Publish) {
	rr.log.Debug("adding channel", "id", id)
	rr.mu.Lock()
	defer rr.mu.Unlock()
	rr.rm[id] = rc
}

func (rr *mqttResponseRouter) CloseResponseID(id string) {
	rr.log.Debug("closing channel", "id", id)
	rr.mu.Lock()
	defer rr.mu.Unlock()
	close(rr.rm[id])
	delete(rr.rm, id)
}
