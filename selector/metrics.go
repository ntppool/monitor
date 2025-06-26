package selector

import (
	"strconv"

	"github.com/prometheus/client_golang/prometheus"
	"go.ntppool.org/monitor/ntpdb"
)

// Metrics contains all prometheus metrics for the selector system
type Metrics struct {
	// Status change tracking
	StatusChanges *prometheus.CounterVec

	// Constraint violation metrics
	ConstraintViolations    *prometheus.CounterVec
	GrandfatheredViolations *prometheus.GaugeVec

	// Selection algorithm performance
	ProcessDuration   *prometheus.HistogramVec
	MonitorsEvaluated *prometheus.CounterVec
	ChangesApplied    *prometheus.CounterVec
	ChangesFailed     *prometheus.CounterVec

	// Monitor pool health
	MonitorPoolSize           *prometheus.GaugeVec
	GloballyActiveMonitors    *prometheus.GaugeVec
	ConstraintBlockedMonitors *prometheus.GaugeVec
}

// NewMetrics creates and registers all selector metrics
func NewMetrics(reg prometheus.Registerer) *Metrics {
	m := &Metrics{
		// Track status changes with dual monitor identification
		StatusChanges: prometheus.NewCounterVec(
			prometheus.CounterOpts{
				Name: "selector_status_changes_total",
				Help: "Total number of monitor status changes by the selector",
			},
			[]string{"monitor_id_token", "monitor_tls_name", "from_status", "to_status", "server_id", "reason"},
		),

		// Track constraint violations
		ConstraintViolations: prometheus.NewCounterVec(
			prometheus.CounterOpts{
				Name: "selector_constraint_violations_total",
				Help: "Total number of constraint violations detected",
			},
			[]string{"monitor_id_token", "monitor_tls_name", "constraint_type", "server_id", "is_grandfathered"},
		),

		// Track grandfathered violations
		GrandfatheredViolations: prometheus.NewGaugeVec(
			prometheus.GaugeOpts{
				Name: "selector_grandfathered_violations",
				Help: "Current number of grandfathered constraint violations",
			},
			[]string{"monitor_id_token", "monitor_tls_name", "constraint_type", "server_id"},
		),

		// Track processing performance
		ProcessDuration: prometheus.NewHistogramVec(
			prometheus.HistogramOpts{
				Name:    "selector_process_duration_seconds",
				Help:    "Time spent processing each server in seconds",
				Buckets: prometheus.DefBuckets,
			},
			[]string{"server_id"},
		),

		MonitorsEvaluated: prometheus.NewCounterVec(
			prometheus.CounterOpts{
				Name: "selector_monitors_evaluated_total",
				Help: "Total number of monitors evaluated per server",
			},
			[]string{"server_id"},
		),

		ChangesApplied: prometheus.NewCounterVec(
			prometheus.CounterOpts{
				Name: "selector_changes_applied_total",
				Help: "Total number of status changes successfully applied",
			},
			[]string{"server_id"},
		),

		ChangesFailed: prometheus.NewCounterVec(
			prometheus.CounterOpts{
				Name: "selector_changes_failed_total",
				Help: "Total number of status changes that failed to apply",
			},
			[]string{"server_id"},
		),

		// Track monitor pool health
		MonitorPoolSize: prometheus.NewGaugeVec(
			prometheus.GaugeOpts{
				Name: "selector_monitor_pool_size",
				Help: "Number of monitors in each status per server",
			},
			[]string{"status", "server_id"},
		),

		GloballyActiveMonitors: prometheus.NewGaugeVec(
			prometheus.GaugeOpts{
				Name: "selector_globally_active_monitors",
				Help: "Number of globally active monitors in testing pool per server",
			},
			[]string{"server_id"},
		),

		ConstraintBlockedMonitors: prometheus.NewGaugeVec(
			prometheus.GaugeOpts{
				Name: "selector_constraint_blocked_monitors",
				Help: "Number of monitors blocked by constraints per server",
			},
			[]string{"constraint_type", "server_id"},
		),
	}

	// Register all metrics
	reg.MustRegister(
		m.StatusChanges,
		m.ConstraintViolations,
		m.GrandfatheredViolations,
		m.ProcessDuration,
		m.MonitorsEvaluated,
		m.ChangesApplied,
		m.ChangesFailed,
		m.MonitorPoolSize,
		m.GloballyActiveMonitors,
		m.ConstraintBlockedMonitors,
	)

	return m
}

// Helper functions for extracting monitor labels

// getMonitorLabels extracts both id_token and tls_name from a monitor candidate
// Returns safe fallback values if either field is empty
func getMonitorLabels(monitor *monitorCandidate) (string, string) {
	idToken := "unknown"
	tlsName := "unknown"

	if monitor.IDToken != "" {
		idToken = monitor.IDToken
	}
	if monitor.TLSName != "" {
		tlsName = monitor.TLSName
	}

	return idToken, tlsName
}

// getMonitorLabelsFromRow extracts monitor labels from database row
func getMonitorLabelsFromRow(row ntpdb.GetMonitorPriorityRow) (string, string) {
	idToken := "unknown"
	tlsName := "unknown"

	if row.IDToken.Valid && row.IDToken.String != "" {
		idToken = row.IDToken.String
	}
	if row.TlsName.Valid && row.TlsName.String != "" {
		tlsName = row.TlsName.String
	}

	return idToken, tlsName
}

// Metric tracking helper methods

// TrackStatusChange records a monitor status change
func (m *Metrics) TrackStatusChange(
	monitor *monitorCandidate,
	fromStatus, toStatus ntpdb.ServerScoresStatus,
	serverID uint32,
	reason string,
) {
	idToken, tlsName := getMonitorLabels(monitor)
	m.StatusChanges.WithLabelValues(
		idToken,
		tlsName,
		string(fromStatus),
		string(toStatus),
		strconv.FormatUint(uint64(serverID), 10),
		reason,
	).Inc()
}

// TrackConstraintViolation records a constraint violation
func (m *Metrics) TrackConstraintViolation(
	monitor *monitorCandidate,
	constraintType constraintViolationType,
	serverID uint32,
	isGrandfathered bool,
) {
	idToken, tlsName := getMonitorLabels(monitor)
	m.ConstraintViolations.WithLabelValues(
		idToken,
		tlsName,
		string(constraintType),
		strconv.FormatUint(uint64(serverID), 10),
		strconv.FormatBool(isGrandfathered),
	).Inc()

	// Track grandfathered violations separately
	if isGrandfathered {
		m.GrandfatheredViolations.WithLabelValues(
			idToken,
			tlsName,
			string(constraintType),
			strconv.FormatUint(uint64(serverID), 10),
		).Inc()
	}
}

// TrackMonitorPoolSizes updates monitor pool size metrics
func (m *Metrics) TrackMonitorPoolSizes(
	serverID uint32,
	activeCount, testingCount, candidateCount, availableCount int,
) {
	serverIDStr := strconv.FormatUint(uint64(serverID), 10)

	m.MonitorPoolSize.WithLabelValues("active", serverIDStr).Set(float64(activeCount))
	m.MonitorPoolSize.WithLabelValues("testing", serverIDStr).Set(float64(testingCount))
	m.MonitorPoolSize.WithLabelValues("candidate", serverIDStr).Set(float64(candidateCount))
	m.MonitorPoolSize.WithLabelValues("available", serverIDStr).Set(float64(availableCount))
}

// TrackConstraintBlockedCount updates constraint blocked monitor counts
func (m *Metrics) TrackConstraintBlockedCount(
	serverID uint32,
	constraintCounts map[constraintViolationType]int,
) {
	serverIDStr := strconv.FormatUint(uint64(serverID), 10)

	for constraintType, count := range constraintCounts {
		m.ConstraintBlockedMonitors.WithLabelValues(
			string(constraintType),
			serverIDStr,
		).Set(float64(count))
	}
}

// RecordProcessingMetrics records various processing metrics for a server
func (m *Metrics) RecordProcessingMetrics(
	serverID uint32,
	duration float64,
	evaluatedCount, appliedChanges, failedChanges, globallyActiveCount int,
) {
	serverIDStr := strconv.FormatUint(uint64(serverID), 10)

	m.ProcessDuration.WithLabelValues(serverIDStr).Observe(duration)
	m.MonitorsEvaluated.WithLabelValues(serverIDStr).Add(float64(evaluatedCount))
	m.ChangesApplied.WithLabelValues(serverIDStr).Add(float64(appliedChanges))
	m.ChangesFailed.WithLabelValues(serverIDStr).Add(float64(failedChanges))
	m.GloballyActiveMonitors.WithLabelValues(serverIDStr).Set(float64(globallyActiveCount))
}
