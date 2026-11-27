package metrics

import (
	"github.com/prometheus/client_golang/prometheus"
)

type Metrics struct {
	TestsRequested *prometheus.CounterVec
	TestsCompleted *prometheus.CounterVec

	// QueueHandouts counts the rows GetServers hands out, by IP version
	// and the status of the server_scores row.
	QueueHandouts *prometheus.CounterVec
	// QueueHandoutInterval is the time since a row's previous queue_ts at
	// the moment GetServers hands it out again.
	QueueHandoutInterval *prometheus.HistogramVec
	// QueueLiveServers is the number of servers GetServers can hand out.
	QueueLiveServers *prometheus.GaugeVec
	// QueueInterval is the configured handout interval per status.
	QueueInterval *prometheus.GaugeVec
}

func New(r prometheus.Registerer) *Metrics {
	m := &Metrics{}

	requestCounters := map[string]*prometheus.CounterVec{
		"tests_requested_total": nil,
		"tests_completed_total": nil,
	}

	for k := range requestCounters {

		labels := []string{"monitor", "ip_version"}
		switch k {
		case "tests_completed_total":
			labels = append(labels, "result", "version", "account", "account_id")
		case "tests_requested_total":
			labels = append(labels, "account", "account_id")
		}

		counter := prometheus.NewCounterVec(
			prometheus.CounterOpts{
				Name: k,
				Help: "count of ntp tests",
			},
			labels,
		)
		r.MustRegister(counter)
		requestCounters[k] = counter
	}

	m.TestsRequested = requestCounters["tests_requested_total"]
	m.TestsCompleted = requestCounters["tests_completed_total"]

	m.QueueHandouts = prometheus.NewCounterVec(
		prometheus.CounterOpts{
			Name: "queue_handouts_total",
			Help: "servers handed out to monitors by GetServers",
		},
		[]string{"ip_version", "status"},
	)
	m.QueueHandoutInterval = prometheus.NewHistogramVec(
		prometheus.HistogramOpts{
			Name: "queue_handout_interval_seconds",
			Help: "seconds since the previous handout of the same server to the same monitor",
			// A row is due only after its interval has passed, so on-time
			// handouts land just above it. The edges just above 9m, 25m
			// and 2h (interval_active, interval_testing, interval_candidate
			// and the down-server cadence) keep histogram_quantile from
			// smearing on-time handouts across a wide bucket.
			Buckets: []float64{
				60, 120, 300, 540, 570, 600, 660, 900, 1200, 1500, 1560, 1680, 1800,
				2700, 3600, 5400, 7200, 7260, 7500, 8100, 10800, 14400, 21600, 28800, 43200,
			},
		},
		[]string{"ip_version", "status"},
	)
	m.QueueLiveServers = prometheus.NewGaugeVec(
		prometheus.GaugeOpts{
			Name: "queue_live_servers",
			Help: "servers GetServers can hand out (not deleted)",
		},
		[]string{"ip_version"},
	)
	m.QueueInterval = prometheus.NewGaugeVec(
		prometheus.GaugeOpts{
			Name: "queue_interval_seconds",
			Help: "configured seconds between handouts of a server, by status",
		},
		[]string{"status"},
	)
	r.MustRegister(m.QueueHandouts, m.QueueHandoutInterval, m.QueueLiveServers, m.QueueInterval)

	return m
}
