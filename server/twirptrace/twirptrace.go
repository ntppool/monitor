package ottwirp

import (
	"context"
	"log"
	"net/http"
	"strconv"

	"go.opentelemetry.io/otel"
	"go.opentelemetry.io/otel/attribute"
	"go.opentelemetry.io/otel/propagation"
	otrace "go.opentelemetry.io/otel/trace"

	"github.com/twitchtv/twirp"
)

const (
	RequestReceivedEvent = "request.received"
)

type TraceServerHooks struct {
	Tracer otrace.Tracer
	opts   *TraceOptions
}

type TraceOptions struct {
	includeClientErrors bool
	tags                []TraceTag
	ctxTagFn            func(ctx context.Context) (context.Context, []TraceTag)
}

// TraceTag represents a single span tag.
type TraceTag struct {
	// Key defines the span's tag key.
	Key string

	// Value defines the span's tag value. Values can be numeric types, strings, or
	// bools.
	Value interface{}
}

type TraceOption func(opts *TraceOptions)

// IncludeClientErrors, if set, will report client errors (4xx) as errors in the server span.
// If not set, only 5xx status will be reported as erroneous.
func IncludeClientErrors(includeClientErrors bool) TraceOption {
	return func(opts *TraceOptions) {
		opts.includeClientErrors = includeClientErrors
	}
}

// WithTags defines tags to be added to each outoing span by default.  If there
// is a pre-existing tag set for `key`, it is overwritten.
func WithTags(tags ...TraceTag) TraceOption {
	return func(opts *TraceOptions) {
		opts.tags = tags
	}
}

// WithContextTags defines a function that returns set of trace tags. This is
// useful to extract values from the request ctx and return a set of tags that
// are set on the span. The function is used during the `RequestReceived`
// server hook.
func WithContextTags(fn func(ctx context.Context) (context.Context, []TraceTag)) TraceOption {
	return func(opts *TraceOptions) {
		opts.ctxTagFn = fn
	}
}

// NewOpenTracingHooks provides a twirp.ServerHooks struct which records
// OpenTracing spans.
func NewOpenTracingHooks(tracer otrace.Tracer, opts ...TraceOption) *twirp.ServerHooks {
	serverOpts := &TraceOptions{
		includeClientErrors: true,
	}

	for _, opt := range opts {
		opt(serverOpts)
	}

	traceHooks := &TraceServerHooks{
		Tracer: tracer,
		opts:   serverOpts,
	}

	return traceHooks.TwirpHooks()
}

func (t *TraceServerHooks) TwirpHooks() *twirp.ServerHooks {
	hooks := &twirp.ServerHooks{
		RequestReceived: t.startTraceSpan,
		RequestRouted:   t.handleRequestRouted,
		ResponseSent:    t.finishTrace,
		Error:           t.handleError,
	}

	return hooks
}

func (t *TraceServerHooks) startTraceSpan(ctx context.Context) (context.Context, error) {

	opts := []otrace.SpanStartOption{
		otrace.WithSpanKind(otrace.SpanKindServer),
		otrace.WithNewRoot(),
	}

	ctx, span := t.Tracer.Start(ctx, RequestReceivedEvent, opts...)

	if span != nil {
		span.SetAttributes(attribute.String("component", "twirp"))

		packageName, ok := twirp.PackageName(ctx)
		if ok {
			span.SetAttributes(attribute.String("package", packageName))
		}

		if serviceName, ok := twirp.ServiceName(ctx); ok {
			span.SetAttributes(attribute.String("serviceName", serviceName))
		} else {
			if len(packageName) > 0 {
				span.SetAttributes(attribute.String("serviceName", packageName))
			}
		}

		if len(t.opts.tags) != 0 {
			for _, tag := range t.opts.tags {
				span.SetAttributes(interfaceToAttribute(tag.Key, tag.Value))
			}
		}

		if t.opts.ctxTagFn != nil {
			var ctxTags []TraceTag
			ctx, ctxTags = t.opts.ctxTagFn(ctx)
			for _, tag := range ctxTags {
				span.SetAttributes(interfaceToAttribute(tag.Key, tag.Value))
			}
		}
	}

	return ctx, nil
}

func interfaceToAttribute(k string, value interface{}) attribute.KeyValue {
	switch v := value.(type) {
	case string:
		return attribute.String(k, v)
	case int:
		return attribute.Int(k, v)
	case int32:
		return attribute.Int(k, int(v))
	case int64:
		return attribute.Int64(k, v)
	case float64:
		return attribute.Float64(k, v)
	case bool:
		return attribute.Bool(k, v)
	default:
		log.Printf("need interfaceToAttribute for type %s=%T", k, value)
		return attribute.Bool(k, true)
	}
}

// handleRequestRouted sets the operation name because we won't know what it is
// until the RequestRouted hook.
func (t *TraceServerHooks) handleRequestRouted(ctx context.Context) (context.Context, error) {

	span := otrace.SpanFromContext(ctx)
	if span != nil {
		if method, ok := twirp.MethodName(ctx); ok {
			span.SetName(method)
		}
	}

	return ctx, nil
}

func (t *TraceServerHooks) finishTrace(ctx context.Context) {
	span := otrace.SpanFromContext(ctx)
	if span != nil {
		status, haveStatus := twirp.StatusCode(ctx)
		code, err := strconv.ParseInt(status, 10, 64)
		if haveStatus && err == nil {
			// TODO: Check the status code, if it's a non-2xx/3xx status code, we
			// should probably mark it as an error of sorts.
			span.SetAttributes(attribute.Int64("http.status_code", code))
		}

		span.End()
	}
}

func (t *TraceServerHooks) handleError(ctx context.Context, err twirp.Error) context.Context {
	span := otrace.SpanFromContext(ctx)
	statusCode := twirp.ServerHTTPStatusFromErrorCode(err.Code())
	if span != nil {
		if t.opts.includeClientErrors || statusCode >= 500 {
			span.SetAttributes(attribute.Bool("error", true))
		}
		// span.Logs(otlog.String("event", "error"), otlog.String("message", err.Msg()))
	}

	return ctx
}

// WithTraceContext wraps the handler and extracts the span context from request
// headers to attach to the context for connecting client and server calls.
func WithTraceContext(base http.Handler, tracer otrace.Tracer) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		ctx := r.Context()

		p := otel.GetTextMapPropagator()

		ctx = p.Extract(ctx, propagation.HeaderCarrier(r.Header))
		// ctx = context.WithValue(ctx, tracingInfoKey{}, carrier)
		r = r.WithContext(ctx)

		base.ServeHTTP(w, r)
	})
}

// func extractSpanCtx(ctx context.Context, tracer otrace.Tracer) (ot.SpanContext, error) {
// 	carrier := ctx.Value(tracingInfoKey{})
// 	return tracer.Extract(ot.HTTPHeaders, carrier)
// }
