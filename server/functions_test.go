package server

import (
	"context"
	"testing"

	"github.com/jackc/pgx/v5/pgtype"

	"go.ntppool.org/monitor/ntpdb"
)

type settingsMonitorDB struct {
	ntpdb.QuerierTx
	config string
}

func (db settingsMonitorDB) GetMonitorsTLSName(_ context.Context, name pgtype.Text) ([]ntpdb.Monitor, error) {
	if !name.Valid || name.String != "settings-v4.system" || db.config == "" {
		return nil, nil
	}
	return []ntpdb.Monitor{{Config: []byte(db.config)}}, nil
}

func TestGetMonitorConfigOtlpLogLevel(t *testing.T) {
	for _, tt := range []struct {
		name     string
		defaults string
		monitor  string
		want     string
	}{
		{name: "unspecified", defaults: `{}`, monitor: `{}`, want: "info"},
		{name: "no defaults row", monitor: `{}`, want: "info"},
		{name: "system level", defaults: `{"otlp_log_level":"warn"}`, monitor: `{}`, want: "warn"},
		{name: "monitor override", defaults: `{"otlp_log_level":"warn"}`, monitor: `{"otlp_log_level":"debug"}`, want: "debug"},
		{name: "empty override", defaults: `{"otlp_log_level":"debug"}`, monitor: `{"otlp_log_level":""}`, want: "info"},
		{name: "null override", defaults: `{"otlp_log_level":"debug"}`, monitor: `{"otlp_log_level":null}`, want: "info"},
	} {
		t.Run(tt.name, func(t *testing.T) {
			srv := &Server{db: settingsMonitorDB{config: tt.defaults}}
			monitor := &ntpdb.Monitor{
				Config:    []byte(tt.monitor),
				Ip:        pgtype.Text{String: "192.0.2.1", Valid: true},
				IpVersion: ntpdb.NullMonitorsIpVersion{MonitorsIpVersion: ntpdb.MonitorsIpVersionV4, Valid: true},
			}
			cfg, err := srv.getMonitorConfig(t.Context(), monitor)
			if err != nil {
				t.Fatal(err)
			}
			response, err := cfg.APIv2()
			if err != nil {
				t.Fatal(err)
			}
			if got := response.GetOtlpLogLevel(); got != tt.want {
				t.Errorf("otlp_log_level = %q, want %q", got, tt.want)
			}
		})
	}
}
