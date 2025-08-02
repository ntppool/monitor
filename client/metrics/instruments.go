package metrics

import (
	"context"
	"log/slog"
	"sync"

	"go.ntppool.org/common/metrics"
	"go.opentelemetry.io/otel/metric"
)

var (
	// Existing metrics (converted from Prometheus)
	LocalCheckUp   metric.Int64Gauge
	LocalCheckTime metric.Int64Gauge

	// New counters
	ServersChecked metric.Int64Counter
	NTPQueriesSent metric.Int64Counter
	RPCRequests    metric.Int64Counter

	setupOnce sync.Once
	setupErr  error
)

// InitInstruments initializes all metric instruments for the client.
// This function is safe to call multiple times - it will only initialize once.
func InitInstruments() error {
	setupOnce.Do(func() {
		setupErr = initializeInstruments()
	})
	return setupErr
}

func initializeInstruments() error {
	log := slog.Default()
	meter := metrics.GetMeter("monitor.client")

	var err error

	// Existing metrics (converted from Prometheus)
	LocalCheckUp, err = meter.Int64Gauge("monitor.local_check_up",
		metric.WithDescription("Local NTP check status (1=up, 0=down)"))
	if err != nil {
		log.ErrorContext(context.Background(), "failed to create LocalCheckUp gauge", "err", err)
		return err
	}

	LocalCheckTime, err = meter.Int64Gauge("monitor.local_check_time",
		metric.WithDescription("Last local NTP check timestamp"))
	if err != nil {
		log.ErrorContext(context.Background(), "failed to create LocalCheckTime gauge", "err", err)
		return err
	}

	// SSLCertExpiry is created as a placeholder - callback registration happens in appconfig_manager.go
	// SSLCertExpiry will be created with callback in the config manager
	// We can't create it here because we need access to the appConfig instance

	// New counters
	ServersChecked, err = meter.Int64Counter("monitor.servers_checked_total",
		metric.WithDescription("Total number of NTP servers checked"))
	if err != nil {
		log.ErrorContext(context.Background(), "failed to create ServersChecked counter", "err", err)
		return err
	}

	NTPQueriesSent, err = meter.Int64Counter("monitor.ntp_queries_sent_total",
		metric.WithDescription("Total number of NTP queries sent"))
	if err != nil {
		log.ErrorContext(context.Background(), "failed to create NTPQueriesSent counter", "err", err)
		return err
	}

	RPCRequests, err = meter.Int64Counter("monitor.rpc_requests_total",
		metric.WithDescription("Total number of RPC requests made"))
	if err != nil {
		log.ErrorContext(context.Background(), "failed to create RPCRequests counter", "err", err)
		return err
	}

	log.Info("client metrics instruments initialized successfully")
	return nil
}
