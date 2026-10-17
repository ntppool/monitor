package selector

import (
	"maps"
	"testing"

	"github.com/jackc/pgx/v5/pgtype"
	"github.com/prometheus/client_golang/prometheus"
	"go.ntppool.org/monitor/ntpdb"
)

func TestMonitorServersCollector(t *testing.T) {
	text := func(s string) pgtype.Text { return pgtype.Text{String: s, Valid: s != ""} }
	row := func(idToken, tlsName string, status ntpdb.ServerScoresStatus, servers int64) ntpdb.GetMonitorServerStatusCountsRow {
		return ntpdb.GetMonitorServerStatusCountsRow{IDToken: text(idToken), TlsName: text(tlsName), Status: status, Servers: servers}
	}

	tests := []struct {
		name    string
		updates [][]ntpdb.GetMonitorServerStatusCountsRow
		want    map[string]float64 // "id_token|tls_name|status" -> value
	}{
		{
			name: "one monitor, two statuses",
			updates: [][]ntpdb.GetMonitorServerStatusCountsRow{{
				row("tok1", "mon1.example.com", ntpdb.ServerScoresStatusActive, 7),
				row("tok1", "mon1.example.com", ntpdb.ServerScoresStatusTesting, 3),
			}},
			want: map[string]float64{
				"tok1|mon1.example.com|active":  7,
				"tok1|mon1.example.com|testing": 3,
			},
		},
		{
			name: "missing labels are reported as unknown and summed",
			updates: [][]ntpdb.GetMonitorServerStatusCountsRow{{
				row("", "", ntpdb.ServerScoresStatusCandidate, 2),
				row("", "", ntpdb.ServerScoresStatusCandidate, 5),
			}},
			want: map[string]float64{
				"unknown|unknown|candidate": 7,
			},
		},
		{
			name: "update replaces the previous snapshot",
			updates: [][]ntpdb.GetMonitorServerStatusCountsRow{
				{
					row("tok1", "mon1.example.com", ntpdb.ServerScoresStatusActive, 7),
					row("tok2", "mon2.example.com", ntpdb.ServerScoresStatusActive, 4),
				},
				{
					row("tok1", "mon1.example.com", ntpdb.ServerScoresStatusActive, 8),
				},
			},
			want: map[string]float64{
				"tok1|mon1.example.com|active": 8,
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			reg := prometheus.NewRegistry()
			m := NewMetrics(reg)
			for _, rows := range tt.updates {
				m.MonitorServers.Update(rows)
			}

			mfs, err := reg.Gather()
			if err != nil {
				t.Fatalf("gather: %v", err)
			}

			got := map[string]float64{}
			for _, mf := range mfs {
				if mf.GetName() != "selector_monitor_servers" {
					continue
				}
				for _, metric := range mf.GetMetric() {
					labels := map[string]string{}
					for _, lp := range metric.GetLabel() {
						labels[lp.GetName()] = lp.GetValue()
					}
					key := labels["monitor_id_token"] + "|" + labels["monitor_tls_name"] + "|" + labels["status"]
					got[key] = metric.GetGauge().GetValue()
				}
			}

			if !maps.Equal(got, tt.want) {
				t.Errorf("got %v, want %v", got, tt.want)
			}
		})
	}
}
