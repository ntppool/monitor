package metrics

import (
	"net/http"

	"github.com/prometheus/client_golang/prometheus"
	"github.com/prometheus/client_golang/prometheus/promhttp"
	"go.ntppool.org/monitor/logger"
)

type Metrics struct {
	r              *prometheus.Registry
	TestsRequested *prometheus.CounterVec
	TestsCompleted *prometheus.CounterVec
}

func New() *Metrics {
	r := prometheus.NewRegistry()

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
			labels = append(labels, "result")
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

func (m *Metrics) Handler() http.Handler {

	log := logger.NewStdLog("prom http", false, nil)

	return promhttp.HandlerFor(m.r, promhttp.HandlerOpts{
		ErrorLog:          log,
		Registry:          m.r,
		EnableOpenMetrics: true,
	})
}
