package metrics

import (
	"context"
	"log/slog"
	"sync"
	"time"

	"go.ntppool.org/common/metrics"
	"go.opentelemetry.io/otel/attribute"
	"go.opentelemetry.io/otel/metric"
)

var (
	// New counters
	ServersChecked metric.Int64Counter
	NTPQueriesSent metric.Int64Counter
	RPCRequests    metric.Int64Counter

	setupOnce sync.Once
	setupErr  error
)

type LocalCheckResult struct {
	OK        bool
	CheckTime time.Time
}

var localChecks = struct {
	sync.RWMutex
	byIPVersion map[string]map[string]LocalCheckResult
}{byIPVersion: make(map[string]map[string]LocalCheckResult)}

// SetLocalCheckResults replaces the observations for one IP family.
// The caller gives up ownership of results.
func SetLocalCheckResults(ipVersion string, results map[string]LocalCheckResult) {
	localChecks.Lock()
	defer localChecks.Unlock()
	localChecks.byIPVersion[ipVersion] = results
}

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

	// Existing metrics (converted from Prometheus)
	localCheckUp, err := meter.Int64ObservableGauge("monitor.local_check_up",
		metric.WithDescription("Local NTP check status (1=up, 0=down)"))
	if err != nil {
		log.ErrorContext(context.Background(), "failed to create LocalCheckUp gauge", "err", err)
		return err
	}

	localCheckTime, err := meter.Int64ObservableGauge("monitor.local_check_time",
		metric.WithDescription("Last local NTP check timestamp"))
	if err != nil {
		log.ErrorContext(context.Background(), "failed to create LocalCheckTime gauge", "err", err)
		return err
	}
	// Observable gauges export only the current host set. Synchronous gauges
	// with cumulative temporality keep exporting old values for removed hosts.
	_, err = meter.RegisterCallback(func(_ context.Context, observer metric.Observer) error {
		localChecks.RLock()
		defer localChecks.RUnlock()
		for ipVersion, results := range localChecks.byIPVersion {
			for host, result := range results {
				var up int64
				if result.OK {
					up = 1
				}
				attrs := metric.WithAttributes(attribute.String("host", host), attribute.String("ip_version", ipVersion))
				observer.ObserveInt64(localCheckUp, up, attrs)
				observer.ObserveInt64(localCheckTime, result.CheckTime.Unix(), attrs)
			}
		}
		return nil
	}, localCheckUp, localCheckTime)
	if err != nil {
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
