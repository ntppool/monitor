package selector

import (
	"sync"

	"github.com/prometheus/client_golang/prometheus"
	"go.ntppool.org/monitor/ntpdb"
)

// Metrics contains all prometheus metrics for the selector system.
//
// Metrics are labeled per monitor where useful, never per server: with
// thousands of servers a server_id label made the selector export
// well over 100k series.
type Metrics struct {
	// Status change tracking
	StatusChanges *prometheus.CounterVec

	// Constraint violation metrics
	ConstraintViolations *prometheus.CounterVec

	// Selection algorithm performance
	ProcessDuration   prometheus.Histogram
	MonitorsEvaluated prometheus.Counter
	ChangesApplied    prometheus.Counter
	ChangesFailed     prometheus.Counter

	// Monitor pool health
	ServerMonitors *prometheus.HistogramVec
	MonitorServers *monitorServersCollector
}

// NewMetrics creates and registers all selector metrics
func NewMetrics(reg prometheus.Registerer) *Metrics {
	m := &Metrics{
		StatusChanges: prometheus.NewCounterVec(
			prometheus.CounterOpts{
				Name: "selector_status_changes_total",
				Help: "Total number of monitor status changes by the selector",
			},
			[]string{"monitor_id_token", "monitor_tls_name", "from_status", "to_status", "reason"},
		),

		ConstraintViolations: prometheus.NewCounterVec(
			prometheus.CounterOpts{
				Name: "selector_constraint_violations_total",
				Help: "Total number of server reviews where a monitor was in constraint violation",
			},
			[]string{"monitor_id_token", "monitor_tls_name", "constraint_type"},
		),

		ProcessDuration: prometheus.NewHistogram(
			prometheus.HistogramOpts{
				Name:    "selector_process_duration_seconds",
				Help:    "Time spent processing servers in seconds",
				Buckets: prometheus.DefBuckets,
			},
		),

		MonitorsEvaluated: prometheus.NewCounter(
			prometheus.CounterOpts{
				Name: "selector_monitors_evaluated_total",
				Help: "Total number of monitors evaluated",
			},
		),

		ChangesApplied: prometheus.NewCounter(
			prometheus.CounterOpts{
				Name: "selector_changes_applied_total",
				Help: "Total number of status changes successfully applied",
			},
		),

		ChangesFailed: prometheus.NewCounter(
			prometheus.CounterOpts{
				Name: "selector_changes_failed_total",
				Help: "Total number of status changes that failed to apply",
			},
		),

		ServerMonitors: prometheus.NewHistogramVec(
			prometheus.HistogramOpts{
				Name:    "selector_server_monitors",
				Help:    "Number of monitors per server in each status, observed when the server is reviewed",
				Buckets: []float64{0, 1, 2, 3, 4, 5, 7, 10, 15, 20, 30, 50, 100, 200},
			},
			[]string{"status"},
		),

		MonitorServers: newMonitorServersCollector(),
	}

	reg.MustRegister(
		m.StatusChanges,
		m.ConstraintViolations,
		m.ProcessDuration,
		m.MonitorsEvaluated,
		m.ChangesApplied,
		m.ChangesFailed,
		m.ServerMonitors,
		m.MonitorServers,
	)

	return m
}

// Helper functions for extracting monitor labels

// monitorLabels returns the id_token and tls_name label values,
// using "unknown" for empty fields.
func monitorLabels(idToken, tlsName string) (string, string) {
	if idToken == "" {
		idToken = "unknown"
	}
	if tlsName == "" {
		tlsName = "unknown"
	}
	return idToken, tlsName
}

// getMonitorLabels extracts both id_token and tls_name from a monitor candidate
func getMonitorLabels(monitor *monitorCandidate) (string, string) {
	return monitorLabels(monitor.IDToken, monitor.TLSName)
}

// Metric tracking helper methods

// TrackStatusChange records a monitor status change
func (m *Metrics) TrackStatusChange(
	monitor *monitorCandidate,
	fromStatus, toStatus ntpdb.ServerScoresStatus,
	reason string,
) {
	idToken, tlsName := getMonitorLabels(monitor)
	m.StatusChanges.WithLabelValues(
		idToken,
		tlsName,
		string(fromStatus),
		string(toStatus),
		reason,
	).Inc()
}

// TrackConstraintViolation records a monitor found in constraint
// violation during a server review
func (m *Metrics) TrackConstraintViolation(
	monitor *monitorCandidate,
	constraintType constraintViolationType,
) {
	idToken, tlsName := getMonitorLabels(monitor)
	m.ConstraintViolations.WithLabelValues(
		idToken,
		tlsName,
		string(constraintType),
	).Inc()
}

// TrackServerMonitorCounts records the number of monitors in each status
// for a reviewed server
func (m *Metrics) TrackServerMonitorCounts(activeCount, testingCount, candidateCount, pausedCount int) {
	m.ServerMonitors.WithLabelValues(string(ntpdb.ServerScoresStatusActive)).Observe(float64(activeCount))
	m.ServerMonitors.WithLabelValues(string(ntpdb.ServerScoresStatusTesting)).Observe(float64(testingCount))
	m.ServerMonitors.WithLabelValues(string(ntpdb.ServerScoresStatusCandidate)).Observe(float64(candidateCount))
	m.ServerMonitors.WithLabelValues(string(ntpdb.ServerScoresStatusPaused)).Observe(float64(pausedCount))
}

// RecordProcessingMetrics records various processing metrics for a server
func (m *Metrics) RecordProcessingMetrics(
	duration float64,
	evaluatedCount, appliedChanges, failedChanges int,
) {
	m.ProcessDuration.Observe(duration)
	m.MonitorsEvaluated.Add(float64(evaluatedCount))
	m.ChangesApplied.Add(float64(appliedChanges))
	m.ChangesFailed.Add(float64(failedChanges))
}

// monitorServersCollector exports selector_monitor_servers, the number of
// servers each monitor is assigned to per status. The values are a
// snapshot replaced on each Update, so monitors without server_scores
// rows drop out instead of leaving stale series.
type monitorServersCollector struct {
	desc *prometheus.Desc

	mu     sync.Mutex
	counts map[monitorServersKey]int64
}

type monitorServersKey struct {
	idToken, tlsName, status string
}

func newMonitorServersCollector() *monitorServersCollector {
	return &monitorServersCollector{
		desc: prometheus.NewDesc(
			"selector_monitor_servers",
			"Number of servers each monitor is assigned to, by server_scores status",
			[]string{"monitor_id_token", "monitor_tls_name", "status"},
			nil,
		),
		counts: map[monitorServersKey]int64{},
	}
}

// Update replaces the snapshot with the given rows.
func (c *monitorServersCollector) Update(rows []ntpdb.GetMonitorServerStatusCountsRow) {
	counts := make(map[monitorServersKey]int64, len(rows))
	for _, row := range rows {
		idToken, tlsName := monitorLabels(row.IDToken.String, row.TlsName.String)
		// sum, so monitors with missing labels can't produce duplicate series
		counts[monitorServersKey{idToken, tlsName, string(row.Status)}] += row.Servers
	}

	c.mu.Lock()
	c.counts = counts
	c.mu.Unlock()
}

// Describe implements prometheus.Collector.
func (c *monitorServersCollector) Describe(ch chan<- *prometheus.Desc) {
	ch <- c.desc
}

// Collect implements prometheus.Collector.
func (c *monitorServersCollector) Collect(ch chan<- prometheus.Metric) {
	c.mu.Lock()
	defer c.mu.Unlock()

	for k, v := range c.counts {
		m, err := prometheus.NewConstMetric(
			c.desc, prometheus.GaugeValue, float64(v),
			k.idToken, k.tlsName, k.status,
		)
		if err != nil {
			m = prometheus.NewInvalidMetric(c.desc, err)
		}
		ch <- m
	}
}
