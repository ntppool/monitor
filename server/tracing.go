package server

import (
	"context"
	"os"

	"go.ntppool.org/common/logger"
	"go.ntppool.org/common/version"
	"go.opentelemetry.io/otel"
	"go.opentelemetry.io/otel/attribute"
	"go.opentelemetry.io/otel/exporters/jaeger"
	"go.opentelemetry.io/otel/exporters/otlp/otlptrace"
	"go.opentelemetry.io/otel/exporters/otlp/otlptrace/otlptracehttp"
	"go.opentelemetry.io/otel/propagation"
	"go.opentelemetry.io/otel/sdk/resource"
	otelsdktrace "go.opentelemetry.io/otel/sdk/trace"
	semconv "go.opentelemetry.io/otel/semconv/v1.17.0"
	"go.opentelemetry.io/otel/trace"
)

// https://github.com/open-telemetry/opentelemetry-go-contrib/blob/main/instrumentation/github.com/labstack/echo/otelecho/example/server.go
// https://github.com/open-telemetry/opentelemetry-go/blob/main/exporters/otlp/otlptrace/otlptracehttp/example_test.go

func (srv *Server) NewTracer() trace.Tracer {
	traceProvider := otel.GetTracerProvider()
	return traceProvider.Tracer("monitor-api")
}

func (srv *Server) initTracer(depEnv string) error {

	log := logger.Setup()

	// exporter, err := srv.newStdoutExporter(os.Stdout)

	var err error
	var exporter otelsdktrace.SpanExporter

	if otlp := os.Getenv("OTEL_EXPORTER_OTLP_ENDPOINT"); len(otlp) > 0 {
		exporter, err = srv.newOLTPExporter()
	} else if jaegerHost := os.Getenv("OTEL_EXPORTER_JAEGER_ENDPOINT"); len(jaegerHost) > 0 {
		exporter, err = srv.newJaegerExporter()
	}

	if err != nil {
		return err
	}

	if exporter != nil {
		tp := otelsdktrace.NewTracerProvider(
			otelsdktrace.WithSampler(otelsdktrace.AlwaysSample()),
			otelsdktrace.WithBatcher(exporter),
			otelsdktrace.WithResource(srv.newResource(depEnv)),
		)

		srv.traceProvider = tp

		otel.SetTracerProvider(tp)
		otel.SetTextMapPropagator(propagation.NewCompositeTextMapPropagator(propagation.TraceContext{}, propagation.Baggage{}))
	} else {
		log.Warn("tracing not configured")
	}

	return nil
}

func (srv *Server) newJaegerExporter() (otelsdktrace.SpanExporter, error) {
	exporter, err := jaeger.New(jaeger.WithAgentEndpoint())
	if err != nil {
		logger.Setup().Error("creating jaeger trace exporter", "err", err)
	}
	return exporter, err
}

func (srv *Server) newOLTPExporter() (otelsdktrace.SpanExporter, error) {
	ctx := context.TODO()
	client := otlptracehttp.NewClient()
	exporter, err := otlptrace.New(ctx, client)
	if err != nil {
		logger.Setup().Error("creating OTLP trace exporter", "err", err)
	}
	return exporter, err
}

// func (srv *Server) newStdoutExporter(w io.Writer) (sdktrace.SpanExporter, error) {
// 	return stdouttrace.New(
// 		stdouttrace.WithWriter(w),
// 		// Use human-readable output.
// 		stdouttrace.WithPrettyPrint(),
// 		// Do not print timestamps for the demo.
// 		stdouttrace.WithoutTimestamps(),
// 	)
// }

// newResource returns a resource describing this application.
func (srv *Server) newResource(depEnv string) *resource.Resource {

	r, err := resource.Merge(
		resource.Default(),
		resource.NewWithAttributes(
			semconv.SchemaURL,
			semconv.ServiceNameKey.String("monitor-api"),
			semconv.ServiceVersionKey.String(version.Version()),
			attribute.String("environment", depEnv),
		),
	)
	if err != nil {
		panic(err)
	}

	return r
}
