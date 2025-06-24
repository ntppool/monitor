package metrics

import (
	"github.com/prometheus/client_golang/prometheus"
)

type Metrics struct {
	r              prometheus.Registerer
	TestsRequested *prometheus.CounterVec
	TestsCompleted *prometheus.CounterVec
}

func New(r prometheus.Registerer) *Metrics {
	m := &Metrics{
		r: r,
	}

	requestCounters := map[string]*prometheus.CounterVec{
		"tests_requested_total": nil,
		"tests_completed_total": nil,
	}

	for k := range requestCounters {

		labels := []string{"monitor", "ip_version"}
		if k == "tests_completed_total" {
			labels = append(labels, "result", "version")
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

func (m *Metrics) Registry() prometheus.Registerer {
	return m.r
}
