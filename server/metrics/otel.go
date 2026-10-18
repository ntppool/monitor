package metrics

import (
	"fmt"

	"github.com/prometheus/client_golang/prometheus"
	otelprom "go.opentelemetry.io/otel/exporters/prometheus"
	sdkmetric "go.opentelemetry.io/otel/sdk/metric"
)

// NewMeterProvider returns an OpenTelemetry meter provider that exports
// through the Prometheus registerer r, for instrumentation such as
// otelconnect that only records OpenTelemetry metrics.
func NewMeterProvider(r prometheus.Registerer) (*sdkmetric.MeterProvider, error) {
	exporter, err := otelprom.New(
		otelprom.WithRegisterer(r),
		otelprom.WithoutTargetInfo(),
		otelprom.WithoutScopeInfo(),
	)
	if err != nil {
		return nil, fmt.Errorf("prometheus exporter: %w", err)
	}
	return sdkmetric.NewMeterProvider(sdkmetric.WithReader(exporter)), nil
}
