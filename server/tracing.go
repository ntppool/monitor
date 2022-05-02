package server

import (
	"context"
	"log"
	"os"

	"go.ntppool.org/monitor/version"
	"go.opentelemetry.io/otel"
	"go.opentelemetry.io/otel/attribute"
	"go.opentelemetry.io/otel/exporters/jaeger"
	"go.opentelemetry.io/otel/exporters/otlp/otlptrace"
	"go.opentelemetry.io/otel/exporters/otlp/otlptrace/otlptracehttp"
	"go.opentelemetry.io/otel/propagation"
	"go.opentelemetry.io/otel/sdk/resource"
	sdktrace "go.opentelemetry.io/otel/sdk/trace"
	semconv "go.opentelemetry.io/otel/semconv/v1.10.0"
	"go.opentelemetry.io/otel/trace"
)

// https://github.com/open-telemetry/opentelemetry-go-contrib/blob/main/instrumentation/github.com/labstack/echo/otelecho/example/server.go
// https://github.com/open-telemetry/opentelemetry-go/blob/main/exporters/otlp/otlptrace/otlptracehttp/example_test.go

func (srv *Server) NewTracer() trace.Tracer {
	traceProvider := otel.GetTracerProvider()
	return traceProvider.Tracer("monitor-api")
}

func (srv *Server) initTracer(depEnv string) error {

	// exporter, err := srv.newStdoutExporter(os.Stdout)

	var err error
	var exporter sdktrace.SpanExporter

	if otlp := os.Getenv("OTEL_EXPORTER_OTLP_ENDPOINT"); len(otlp) > 0 {
		exporter, err = srv.newOLTPExporter()
	} else if jaegerHost := os.Getenv("OTEL_EXPORTER_JAEGER_ENDPOINT"); len(jaegerHost) > 0 {
		exporter, err = srv.newJaegerExporter()
	}

	if err != nil {
		return err
	}

	if exporter != nil {
		tp := sdktrace.NewTracerProvider(
			sdktrace.WithSampler(sdktrace.AlwaysSample()),
			sdktrace.WithBatcher(exporter),
			sdktrace.WithResource(srv.newResource(depEnv)),
		)

		otel.SetTracerProvider(tp)
		otel.SetTextMapPropagator(propagation.NewCompositeTextMapPropagator(propagation.TraceContext{}, propagation.Baggage{}))
	} else {
		log.Println("tracing not configured")
	}

	return nil
}

func (srv *Server) newJaegerExporter() (sdktrace.SpanExporter, error) {

	exporter, err := jaeger.New(jaeger.WithAgentEndpoint())
	if err != nil {
		log.Fatalf("creating jaeger trace exporter: %v", err)
	}
	return exporter, err
}

func (srv *Server) newOLTPExporter() (sdktrace.SpanExporter, error) {
	ctx := context.TODO()
	client := otlptracehttp.NewClient()
	exporter, err := otlptrace.New(ctx, client)
	if err != nil {
		log.Fatalf("creating OTLP trace exporter: %v", err)
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

	log.Printf("merged resource: %+v", r)

	return r
}
