package mqserver

import (
	"context"
	"fmt"
	"log/slog"
	"strings"
	"sync"

	"github.com/eclipse/paho.golang/paho"
	"go.opentelemetry.io/otel"
	"go.opentelemetry.io/otel/attribute"
	otrace "go.opentelemetry.io/otel/trace"
)

type responseChan struct {
	ch  chan<- *paho.Publish
	ctx context.Context
}

type mqttResponseRouter struct {
	prefix string
	log    *slog.Logger
	rm     map[string]responseChan
	mu     sync.RWMutex
}

func (mqs *server) setupResponseRouter(_ context.Context, topicPrefix string) *mqttResponseRouter {
	topicPrefix = strings.TrimSuffix(topicPrefix, "#")
	return &mqttResponseRouter{
		prefix: topicPrefix,
		log:    mqs.log,
		rm:     make(map[string]responseChan, 100),
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

		log.DebugContext(ctx, "handling message", "payload", p)

		topic := p.Topic

		span.SetAttributes(attribute.String("mqtt.topic", topic))

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
		rc, ok := rr.rm[topicPath[1]]
		rr.mu.RUnlock()

		if !ok {
			log.Warn("no response channel for", "id", topicPath[1])
			span.RecordError(fmt.Errorf("no response channel"))
			return
		}

		// CloseResponseID may close rc.ch concurrently with this send.
		defer func() {
			if r := recover(); r != nil {
				log.Warn("send on closed response channel", "id", topicPath[1])
			}
		}()

		select {
		case rc.ch <- p:
		case <-rc.ctx.Done():
			log.Debug("receiver context done before delivery", "id", topicPath[1], "err", rc.ctx.Err())
		}
	}
}

func (rr *mqttResponseRouter) AddResponseID(ctx context.Context, id string, ch chan<- *paho.Publish) {
	rr.log.Debug("adding channel", "id", id)
	rr.mu.Lock()
	defer rr.mu.Unlock()
	rr.rm[id] = responseChan{ch: ch, ctx: ctx}
}

func (rr *mqttResponseRouter) CloseResponseID(id string) {
	rr.log.Debug("closing channel", "id", id)
	rr.mu.Lock()
	rc, ok := rr.rm[id]
	delete(rr.rm, id)
	rr.mu.Unlock()
	// Close outside the lock; Handler recovers from a racing send.
	if ok {
		close(rc.ch)
	}
}
