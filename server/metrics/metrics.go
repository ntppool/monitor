package metrics

import (
	"github.com/prometheus/client_golang/prometheus"
)

type Metrics struct {
	TestsRequested *prometheus.CounterVec
	TestsCompleted *prometheus.CounterVec
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

	return m
}
