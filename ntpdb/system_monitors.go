package ntpdb

import (
	"context"
	"database/sql"
	"fmt"
)

// we store "fake" monitors in the monitors table for default
// settings and as pseudo-monitors for scoring experiments.

type notFoundError struct{}

func (m *notFoundError) Error() string {
	return "monitor not found"
}

type SystemMonitor struct {
	Monitor
}

func (mipv MonitorsIpVersion) String() string {
	return string(mipv)
}

func GetSystemMonitor(ctx context.Context, q QuerierTx, name string, ipVersion NullMonitorsIpVersion) (*SystemMonitor, error) {
	name = name + "-" + ipVersion.MonitorsIpVersion.String()

	monitors, err := q.GetMonitorsTLSName(ctx, sql.NullString{String: name + ".system", Valid: true})
	if err != nil {
		if err == sql.ErrNoRows {
			return nil, &notFoundError{}
		}
		return nil, err
	}
	if len(monitors) != 1 {
		return nil, fmt.Errorf("expected 1 system monitor, got %d", len(monitors))
	}

	return &SystemMonitor{monitors[0]}, nil
}
