//go:build integration
// +build integration

package ntpdb_test

import (
	"testing"
	"time"

	"go.ntppool.org/monitor/ntpdb"
	"go.ntppool.org/monitor/testutil"
)

// A monitor whose samples for a server all have a NULL rtt (every one a
// timeout) must still come back from GetMonitorPriority; avg(rtt) is NULL
// for it, and the row is scanned into non-nullable int32 fields.
func TestGetMonitorPriority_AllRttNull(t *testing.T) {
	tdb := testutil.NewTestDB(t)
	defer tdb.Close()

	factory := testutil.NewDataFactory(tdb)

	// The packages' integration tests share one database and run in
	// parallel. testutil.CleanupTestData deletes every row with an id in
	// 1000-9999, so use ids outside that range and delete only our own
	// rows, or this test removes rows other packages are using (and their
	// cleanup removes ours).
	const (
		accountID         = 90000
		serverID          = 90001
		timeoutMonitorID  = 90002
		respondingMonitor = 90003
		oldSampleMonitor  = 90004
	)

	cleanupServerAccount(t, tdb, serverID, accountID)
	defer cleanupServerAccount(t, tdb, serverID, accountID)

	now := time.Now()

	factory.CreateTestAccount(t, accountID, "test@example.com")
	factory.CreateTestServer(t, serverID, "192.0.2.20", "v4", nil)
	factory.CreateTestMonitor(t, timeoutMonitorID, "timeouts.test", accountID, "192.0.2.21", "active")
	factory.CreateTestMonitor(t, respondingMonitor, "responds.test", accountID, "192.0.2.22", "active")
	factory.CreateTestServerScore(t, serverID, timeoutMonitorID, "active", -50)
	factory.CreateTestMonitor(t, oldSampleMonitor, "old-sample.test", accountID, "192.0.2.23", "active")
	factory.CreateTestServerScore(t, serverID, respondingMonitor, "active", 20)
	factory.CreateTestServerScore(t, serverID, oldSampleMonitor, "active", 20)

	rtt := int32(25000) // microseconds
	for i := 0; i < 5; i++ {
		ts := now.Add(-time.Duration(i) * time.Minute)
		// timeouts: step -5, no rtt
		factory.CreateTestLogScore(t, serverID, timeoutMonitorID, -50, -5, nil, ts)
		factory.CreateTestLogScore(t, serverID, respondingMonitor, 20, 1, &rtt, ts)
	}

	// One sample 30h old and one recent: the old one only counts when the
	// sample window is longer than 24h.
	factory.CreateTestLogScore(t, serverID, oldSampleMonitor, 20, 1, &rtt, now.Add(-30*time.Hour))
	factory.CreateTestLogScore(t, serverID, oldSampleMonitor, 20, 1, &rtt, now)

	rows, err := tdb.Queries().GetMonitorPriority(tdb.Context(), ntpdb.GetMonitorPriorityParams{
		ServerID:            serverID,
		SampleWindowSeconds: (24 * time.Hour).Seconds(),
	})
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

	counts := []struct {
		name   string
		window time.Duration
		want   int64
	}{
		{"24h window leaves out the 30h-old sample", 24 * time.Hour, 1},
		{"72h window counts the 30h-old sample", 72 * time.Hour, 2},
	}
	for _, tt := range counts {
		t.Run(tt.name, func(t *testing.T) {
			rows, err := tdb.Queries().GetMonitorPriority(tdb.Context(), ntpdb.GetMonitorPriorityParams{
				ServerID:            serverID,
				SampleWindowSeconds: tt.window.Seconds(),
			})
			testutil.AssertNoError(t, err, "GetMonitorPriority")
			var got int64 = -1
			for _, r := range rows {
				if r.ID == oldSampleMonitor {
					got = r.Count
				}
			}
			testutil.AssertEqual(t, tt.want, got, "sample count for the monitor with an old sample")
		})
	}
}

// cleanupServerAccount deletes one server's rows and the monitors and
// account that own them, and nothing else.
func cleanupServerAccount(t *testing.T, tdb *testutil.TestDB, serverID, accountID int64) {
	t.Helper()
	stmts := []struct {
		sql string
		id  int64
	}{
		{"DELETE FROM log_scores WHERE server_id = $1", serverID},
		{"DELETE FROM server_scores WHERE server_id = $1", serverID},
		{"DELETE FROM servers WHERE id = $1", serverID},
		{"DELETE FROM monitors WHERE account_id = $1", accountID},
		{"DELETE FROM accounts WHERE id = $1", accountID},
	}
	for _, s := range stmts {
		if _, err := tdb.Exec(tdb.Context(), s.sql, s.id); err != nil {
			t.Logf("cleanup %q: %v", s.sql, err)
		}
	}
}
