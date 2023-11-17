package twirptrace

// from oteltwirp

import (
	"io"
	"net/http"

	"go.ntppool.org/common/tracing"
	"go.opentelemetry.io/otel"
	"go.opentelemetry.io/otel/codes"
	"go.opentelemetry.io/otel/propagation"
	httpconv "go.opentelemetry.io/otel/semconv/v1.20.0/httpconv"
	"go.opentelemetry.io/otel/trace"
)

// HTTPClient as an interface that models *http.Client.
type HTTPClient interface {
	Do(req *http.Request) (*http.Response, error)
}

// TraceHTTPClient wraps a provided http.Client and tracer for instrumenting
// requests.
type TraceHTTPClient struct {
	client              HTTPClient
	includeClientErrors bool
	tracer              trace.Tracer
	propagator          propagation.TextMapPropagator
}

func NewTraceHTTPClient(client HTTPClient) *TraceHTTPClient {
	if client == nil {
		client = http.DefaultClient
	}

	c := &TraceHTTPClient{
		client:              client,
		includeClientErrors: true,
		tracer:              tracing.Tracer(),
		propagator:          otel.GetTextMapPropagator(),
	}
	return c
}

// Do injects the tracing headers into the tracer and updates the headers before
// making the actual request.
func (c *TraceHTTPClient) Do(r *http.Request) (*http.Response, error) {
	ctx := r.Context()

	attr := httpconv.ClientRequest(r)
	ctx, span := c.tracer.Start(
		ctx,
		"client-request",
		trace.WithSpanKind(trace.SpanKindClient),
		trace.WithAttributes(attr...),
	)
	setTwirpInfo(ctx, span)

	c.propagator.Inject(ctx, propagation.HeaderCarrier(r.Header))

	r = r.WithContext(ctx)

	resp, err := c.client.Do(r)

	if err != nil {
		span.RecordError(err)
		span.SetStatus(codes.Error, err.Error())
		span.End()
		return resp, err
	}

	span.SetStatus(httpconv.ClientStatus(resp.StatusCode))
	span.SetAttributes(httpconv.ClientResponse(resp)...)

	// We want to track when the body is closed, meaning the server is done with
	// the response.
	resp.Body = closer{
		ReadCloser: resp.Body,
		span:       span,
	}
	return resp, nil
}

type closer struct {
	io.ReadCloser
	span trace.Span
}

func (c closer) Close() error {
	err := c.ReadCloser.Close()
	c.span.End()
	return err
}
