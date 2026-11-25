package ntpdb

import (
	"testing"

	"github.com/jackc/pgx/v5/pgtype"
)

func TestMonitorConfigWireLevel(t *testing.T) {
	for _, tt := range []struct {
		name     string
		defaults *string // nil: no settings row, use the single config
		monitor  string
		want     string
	}{
		{name: "no level anywhere", defaults: new(`{}`), monitor: `{}`, want: "info"},
		{name: "default level", defaults: new(`{"otlp_log_level":"warn"}`), monitor: `{}`, want: "warn"},
		{name: "monitor override", defaults: new(`{"otlp_log_level":"warn"}`), monitor: `{"otlp_log_level":"debug"}`, want: "debug"},
		{name: "empty string override", defaults: new(`{}`), monitor: `{"otlp_log_level":""}`, want: "info"},
		{name: "empty override clears default", defaults: new(`{"otlp_log_level":"debug"}`), monitor: `{"otlp_log_level":""}`, want: "info"},
		{name: "no settings row", monitor: `{}`, want: "info"},
		{name: "no settings row with level", monitor: `{"otlp_log_level":"error"}`, want: "error"},
	} {
		t.Run(tt.name, func(t *testing.T) {
			monitor := &Monitor{
				Config: []byte(tt.monitor),
				Ip:     pgtype.Text{String: "192.0.2.1", Valid: true},
			}

			var cfg *MonitorConfig
			var err error
			if tt.defaults != nil {
				cfg, err = monitor.GetConfigWithDefaults([]byte(*tt.defaults))
			} else {
				cfg, err = monitor.GetConfig()
			}
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
