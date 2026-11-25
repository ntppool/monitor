package localok

import (
	"bytes"
	"log/slog"
	"net/netip"
	"slices"
	"strings"
	"sync"
	"testing"
	"time"

	"go.opentelemetry.io/otel"
	sdkmetric "go.opentelemetry.io/otel/sdk/metric"
	"go.opentelemetry.io/otel/sdk/metric/metricdata"

	"go.ntppool.org/common/logger"
	"go.ntppool.org/monitor/client/config/checkconfig"
	"go.ntppool.org/monitor/client/metrics"
	apiv2 "go.ntppool.org/monitor/gen/monitor/v2"
)

// The client registers instruments once for the life of the process.
var localCheckTestReader = sync.OnceValues(func() (*sdkmetric.ManualReader, error) {
	reader := sdkmetric.NewManualReader()
	provider := sdkmetric.NewMeterProvider(sdkmetric.WithReader(reader))
	otel.SetMeterProvider(provider)
	return reader, metrics.InitInstruments()
})

func TestLocalOKRefreshesBaseChecks(t *testing.T) {
	reader, err := localCheckTestReader()
	if err != nil {
		t.Fatal(err)
	}

	var logs bytes.Buffer
	ctx := logger.NewContext(t.Context(), slog.New(slog.NewTextHandler(&logs, nil)))
	conf := checkconfig.NewConfigger(nil)
	ipv4 := netip.MustParseAddr("192.0.2.1").AsSlice()
	ipv6 := netip.MustParseAddr("2001:db8::1").AsSlice()
	// Loopback hosts are rejected before any NTP network I/O, but still
	// produce per-host metrics for the attempted checks.
	conf.SetConfigFromApi(&apiv2.GetConfigResponse{IpBytes: ipv4, BaseChecks: [][]byte{[]byte("127.0.0.1")}})
	local := NewLocalOK(conf)

	for _, tt := range []struct {
		name    string
		config  *apiv2.GetConfigResponse
		host    string
		warning string
	}{
		{name: "startup", host: "127.0.0.1"},
		{name: "replace hosts", config: &apiv2.GetConfigResponse{IpBytes: ipv4, BaseChecks: [][]byte{[]byte("127.0.0.2")}}, host: "127.0.0.2"},
		{name: "empty list", config: &apiv2.GetConfigResponse{IpBytes: ipv4}, host: "127.0.0.2"},
		{name: "wrong family", config: &apiv2.GetConfigResponse{IpBytes: ipv6, BaseChecks: [][]byte{[]byte("127.0.0.3")}}, warning: "IP family"},
	} {
		t.Run(tt.name, func(t *testing.T) {
			logs.Reset()
			if tt.config != nil {
				conf.SetConfigFromApi(tt.config)
			}
			local.lastCheck = time.Time{}
			local.Check(ctx)
			if tt.warning != "" {
				if !strings.Contains(logs.String(), tt.warning) || strings.Contains(logs.String(), "local-check failure") {
					t.Fatalf("expected %q warning and no host check, got %s", tt.warning, logs.String())
				}
				return
			}

			var data metricdata.ResourceMetrics
			if err := reader.Collect(ctx, &data); err != nil {
				t.Fatal(err)
			}
			hosts := map[string][]string{}
			for _, scope := range data.ScopeMetrics {
				for _, instrument := range scope.Metrics {
					if instrument.Name != "monitor.local_check_up" && instrument.Name != "monitor.local_check_time" {
						continue
					}
					gauge, ok := instrument.Data.(metricdata.Gauge[int64])
					if !ok {
						t.Fatalf("%s aggregation = %T, want an int64 gauge", instrument.Name, instrument.Data)
					}
					for _, point := range gauge.DataPoints {
						host, _ := point.Attributes.Value("host")
						hosts[instrument.Name] = append(hosts[instrument.Name], host.AsString())
					}
				}
			}
			for _, name := range []string{"monitor.local_check_up", "monitor.local_check_time"} {
				if got := hosts[name]; !slices.Equal(got, []string{tt.host}) {
					t.Errorf("%s hosts = %v, want [%s]", name, got, tt.host)
				}
			}
		})
	}

	t.Run("concurrent config, checks and metrics", func(t *testing.T) {
		var workers sync.WaitGroup
		workers.Go(func() {
			for range 20 {
				conf.SetConfigFromApi(&apiv2.GetConfigResponse{IpBytes: ipv4, BaseChecks: [][]byte{[]byte("127.0.0.2")}})
			}
		})
		workers.Go(func() {
			for range 20 {
				local.mu.Lock()
				local.lastCheck = time.Time{}
				local.mu.Unlock()
				local.Check(ctx)
			}
		})
		workers.Go(func() {
			for range 20 {
				local.Check(ctx)
				var data metricdata.ResourceMetrics
				if err := reader.Collect(ctx, &data); err != nil {
					t.Error(err)
				}
			}
		})
		workers.Wait()
	})
}
