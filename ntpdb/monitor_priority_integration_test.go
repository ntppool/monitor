//go:build integration
// +build integration

package ntpdb_test

import (
	"testing"
	"time"

	"go.ntppool.org/monitor/testutil"
)

// A monitor whose samples for a server all have a NULL rtt (every one a
// timeout) must still come back from GetMonitorPriority; avg(rtt) is NULL
// for it, and the row is scanned into non-nullable int32 fields.
func TestGetMonitorPriority_AllRttNull(t *testing.T) {
	tdb := testutil.NewTestDB(t)
	defer tdb.Close()
	defer tdb.CleanupTestData(t)

	factory := testutil.NewDataFactory(tdb)

	const (
		serverID          = 4001
		timeoutMonitorID  = 4002
		respondingMonitor = 4003
	)

	now := time.Now()

	factory.CreateTestAccount(t, 4000, "test@example.com")
	factory.CreateTestServer(t, serverID, "192.0.2.20", "v4", nil)
	factory.CreateTestMonitor(t, timeoutMonitorID, "timeouts.test", 4000, "192.0.2.21", "active")
	factory.CreateTestMonitor(t, respondingMonitor, "responds.test", 4000, "192.0.2.22", "active")
	factory.CreateTestServerScore(t, serverID, timeoutMonitorID, "active", -50)
	factory.CreateTestServerScore(t, serverID, respondingMonitor, "active", 20)

	rtt := int32(25000) // microseconds
	for i := 0; i < 5; i++ {
		ts := now.Add(-time.Duration(i) * time.Minute)
		// timeouts: step -5, no rtt
		factory.CreateTestLogScore(t, serverID, timeoutMonitorID, -50, -5, nil, ts)
		factory.CreateTestLogScore(t, serverID, respondingMonitor, 20, 1, &rtt, ts)
	}

	rows, err := tdb.Queries().GetMonitorPriority(tdb.Context(), serverID)
	if err != nil {
		t.Fatalf("GetMonitorPriority() error = %v", err)
	}

	byID := map[int64]int{}
	for i, r := range rows {
		byID[r.ID] = i
	}

	ti, ok := byID[timeoutMonitorID]
	if !ok {
		t.Fatalf("no row for the all-timeout monitor; got %d rows", len(rows))
	}
	if got := rows[ti]; got.AvgRtt != 0 || got.MonitorPriority != 0 || got.Healthy {
		t.Errorf("all-timeout monitor: AvgRtt = %d, MonitorPriority = %d, Healthy = %v; want 0, 0, false",
			got.AvgRtt, got.MonitorPriority, got.Healthy)
	}

	ri, ok := byID[respondingMonitor]
	if !ok {
		t.Fatalf("no row for the responding monitor; got %d rows", len(rows))
	}
	if got := rows[ri]; got.AvgRtt != 25 {
		t.Errorf("responding monitor: AvgRtt = %d, want 25", got.AvgRtt)
	}
}
