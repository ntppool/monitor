package metrics

import (
	"context"
	"testing"

	"github.com/prometheus/client_golang/prometheus"
	"go.opentelemetry.io/otel/attribute"
	"go.opentelemetry.io/otel/metric"
)

func TestNewMeterProvider(t *testing.T) {
	reg := prometheus.NewRegistry()
	mp, err := NewMeterProvider(reg)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() {
		if err := mp.Shutdown(context.Background()); err != nil {
			t.Error(err)
		}
	})

	h, err := mp.Meter("test").Float64Histogram("rpc.server.call.duration", metric.WithUnit("s"))
	if err != nil {
		t.Fatal(err)
	}
	h.Record(context.Background(), 0.1, metric.WithAttributes(
		attribute.String("rpc.method", "monitor.v2.MonitorService/GetConfig"),
		attribute.String("rpc.response.status_code", "INVALID_ARGUMENT"),
	))

	mfs, err := reg.Gather()
	if err != nil {
		t.Fatal(err)
	}

	names := map[string]bool{}
	for _, mf := range mfs {
		names[mf.GetName()] = true
		t.Logf("%s", mf.String())
		if mf.GetName() != "rpc_server_call_duration_seconds" {
			continue
		}
		labels := map[string]string{}
		for _, l := range mf.GetMetric()[0].GetLabel() {
			labels[l.GetName()] = l.GetValue()
		}
		if labels["rpc_response_status_code"] != "INVALID_ARGUMENT" {
			t.Errorf("status code label = %q", labels["rpc_response_status_code"])
		}
		if _, ok := labels["otel_scope_name"]; ok {
			t.Errorf("unexpected scope labels: %v", labels)
		}
	}

	if !names["rpc_server_call_duration_seconds"] {
		t.Errorf("rpc_server_call_duration_seconds not exported, got %v", names)
	}
	if names["target_info"] {
		t.Errorf("unexpected target_info metric")
	}
}
