package server

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/jackc/pgx/v5/pgtype"
	"github.com/prometheus/client_golang/prometheus"
	dto "github.com/prometheus/client_model/go"
	"go.opentelemetry.io/otel/attribute"

	"go.ntppool.org/common/timeutil"
	"go.ntppool.org/monitor/monitorsettings"
	"go.ntppool.org/monitor/ntpdb"
	"go.ntppool.org/monitor/server/metrics"
	"go.ntppool.org/monitor/testutil"
)

type histogramSample struct {
	count uint64
	sum   float64
}

// readMetric returns the current value of one series.
func readMetric(t *testing.T, m prometheus.Metric) *dto.Metric {
	t.Helper()
	var out dto.Metric
	testutil.AssertNoError(t, m.Write(&out), "read metric")
	return &out
}

func handoutRow(status ntpdb.ServerScoresStatus, prev *time.Time) ntpdb.GetServersRow {
	row := ntpdb.GetServersRow{Status: status}
	if prev != nil {
		row.PrevQueueTs = pgtype.Timestamptz{Time: *prev, Valid: true}
	}
	return row
}

func downRow(row ntpdb.GetServersRow) ntpdb.GetServersRow {
	row.Down = true
	return row
}

func TestRecordHandouts(t *testing.T) {
	now := time.Date(2026, 11, 26, 12, 0, 0, 0, time.UTC)
	ago := func(d time.Duration) *time.Time {
		ts := now.Add(-d)
		return &ts
	}

	tests := []struct {
		name       string
		rows       []ntpdb.GetServersRow
		wantCounts map[string]float64
		wantHist   map[string]histogramSample
	}{
		{
			name: "candidate, never-queued candidate and active",
			rows: []ntpdb.GetServersRow{
				handoutRow(ntpdb.ServerScoresStatusCandidate, ago(2*time.Hour)),
				handoutRow(ntpdb.ServerScoresStatusCandidate, nil),
				handoutRow(ntpdb.ServerScoresStatusActive, ago(9*time.Minute)),
			},
			wantCounts: map[string]float64{"candidate": 2, "active": 1},
			wantHist: map[string]histogramSample{
				"candidate": {count: 1, sum: 7200},
				"active":    {count: 1, sum: 540},
			},
		},
		{
			name: "down server counted, not in the interval histogram",
			rows: []ntpdb.GetServersRow{
				handoutRow(ntpdb.ServerScoresStatusActive, ago(9*time.Minute)),
				downRow(handoutRow(ntpdb.ServerScoresStatusActive, ago(2*time.Hour))),
				downRow(handoutRow(ntpdb.ServerScoresStatusTesting, ago(2*time.Hour))),
			},
			wantCounts: map[string]float64{"active": 2, "testing": 1},
			wantHist: map[string]histogramSample{
				"active": {count: 1, sum: 540},
			},
		},
		{
			name: "no rows",
			rows: nil,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			m := metrics.New(prometheus.NewRegistry())

			recordHandouts(m, "v4", tt.rows, now)

			for _, status := range []string{"candidate", "active", "testing"} {
				got := readMetric(t, m.QueueHandouts.WithLabelValues("v4", status)).GetCounter().GetValue()
				testutil.AssertEqual(t, tt.wantCounts[status], got, "queue_handouts_total for %s", status)

				h := readMetric(t, m.QueueHandoutInterval.WithLabelValues("v4", status).(prometheus.Metric)).GetHistogram()
				gotHist := histogramSample{count: h.GetSampleCount(), sum: h.GetSampleSum()}
				testutil.AssertEqual(t, tt.wantHist[status], gotHist, "queue_handout_interval_seconds for %s", status)
			}
		})
	}
}

func TestHandoutSpanAttributes(t *testing.T) {
	rows := []ntpdb.GetServersRow{
		handoutRow(ntpdb.ServerScoresStatusCandidate, nil),
		handoutRow(ntpdb.ServerScoresStatusCandidate, nil),
		handoutRow(ntpdb.ServerScoresStatusActive, nil),
	}

	tests := []struct {
		name string
		rows []ntpdb.GetServersRow
		want map[attribute.Key]int64
	}{
		{"one attribute per status present", rows, map[attribute.Key]int64{"handouts.candidate": 2, "handouts.active": 1}},
		{"no rows", nil, map[attribute.Key]int64{}},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			attrs := handoutSpanAttributes(tt.rows)
			testutil.AssertEqual(t, len(tt.want), len(attrs), "attribute count")
			for _, a := range attrs {
				want, ok := tt.want[a.Key]
				testutil.AssertTrue(t, ok, "unexpected attribute %s", a.Key)
				testutil.AssertEqual(t, want, a.Value.AsInt64(), "attribute %s", a.Key)
			}
		})
	}
}

type liveServersDB struct {
	ntpdb.QuerierTx
	rows []ntpdb.CountLiveServersRow
	err  error
}

func (db liveServersDB) CountLiveServers(context.Context) ([]ntpdb.CountLiveServersRow, error) {
	return db.rows, db.err
}

func TestUpdateQueueGauges(t *testing.T) {
	settings := monitorsettings.MonitorSettings{
		IntervalActive:    timeutil.Duration{Duration: 9 * time.Minute},
		IntervalTesting:   timeutil.Duration{Duration: 25 * time.Minute},
		IntervalCandidate: timeutil.Duration{Duration: 2 * time.Hour},
	}

	tests := []struct {
		name         string
		db           liveServersDB
		wantErr      bool
		wantServers  map[string]float64
		wantInterval map[string]float64
	}{
		{
			name: "sets both gauges",
			db: liveServersDB{rows: []ntpdb.CountLiveServersRow{
				{IpVersion: ntpdb.ServersIpVersionV4, Servers: 120},
				{IpVersion: ntpdb.ServersIpVersionV6, Servers: 30},
			}},
			wantServers:  map[string]float64{"v4": 120, "v6": 30},
			wantInterval: map[string]float64{"active": 540, "testing": 1500, "candidate": 7200},
		},
		{
			name:    "query error",
			db:      liveServersDB{err: errors.New("db down")},
			wantErr: true,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			srv := &Server{db: tt.db, m: metrics.New(prometheus.NewRegistry())}

			err := srv.updateQueueGauges(t.Context(), settings)
			if tt.wantErr {
				testutil.AssertError(t, err, "updateQueueGauges")
				return
			}
			testutil.AssertNoError(t, err, "updateQueueGauges")

			for ipVersion, want := range tt.wantServers {
				got := readMetric(t, srv.m.QueueLiveServers.WithLabelValues(ipVersion)).GetGauge().GetValue()
				testutil.AssertEqual(t, want, got, "queue_live_servers %s", ipVersion)
			}
			for status, want := range tt.wantInterval {
				got := readMetric(t, srv.m.QueueInterval.WithLabelValues(status)).GetGauge().GetValue()
				testutil.AssertEqual(t, want, got, "queue_interval_seconds %s", status)
			}
		})
	}
}
