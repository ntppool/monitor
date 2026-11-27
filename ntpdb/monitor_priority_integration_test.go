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

// GetMonitorPriority starts from server_scores: rows without samples in the
// window come back too (count 0, not healthy, after every row with samples),
// and monitors of type 'score' stay out.
func TestGetMonitorPriority_RowsWithoutSamples(t *testing.T) {
	tdb := testutil.NewTestDB(t)
	defer tdb.Close()

	factory := testutil.NewDataFactory(tdb)

	const (
		accountID     = 90150
		serverID      = 90151
		activeID      = 90152
		candidateID   = 90153
		pausedOldID   = 90154
		scoreMonitor  = 90155
		unusedMonitor = 90156 // no server_scores row for this server
	)

	cleanupServerAccount(t, tdb, serverID, accountID)
	defer cleanupServerAccount(t, tdb, serverID, accountID)

	now := time.Now()

	factory.CreateTestAccount(t, accountID, "test@example.com")
	factory.CreateTestServer(t, serverID, "192.0.2.70", "v4", nil)
	factory.CreateTestMonitor(t, activeID, "active.test", accountID, "192.0.2.71", "active")
	factory.CreateTestMonitor(t, candidateID, "candidate.test", accountID, "192.0.2.72", "active")
	factory.CreateTestMonitor(t, pausedOldID, "paused.test", accountID, "192.0.2.73", "active")
	factory.CreateTestMonitorWithType(t, scoreMonitor, "scorer.test", accountID, "192.0.2.74", "active", "score")
	factory.CreateTestMonitor(t, unusedMonitor, "unused.test", accountID, "192.0.2.75", "active")
	factory.CreateTestServerScore(t, serverID, activeID, "active", 20)
	factory.CreateTestServerScore(t, serverID, candidateID, "candidate", 0)
	factory.CreateTestServerScore(t, serverID, pausedOldID, "paused", 0)
	factory.CreateTestServerScore(t, serverID, scoreMonitor, "active", 20)

	rtt := int32(25000)
	for i := 0; i < 10; i++ {
		factory.CreateTestLogScore(t, serverID, activeID, 20, 1, &rtt, now.Add(-time.Duration(i)*time.Minute))
		factory.CreateTestLogScore(t, serverID, scoreMonitor, 20, 1, &rtt, now.Add(-time.Duration(i)*time.Minute))
	}
	// log_scores rows of a monitor with no server_scores row are not returned
	factory.CreateTestLogScore(t, serverID, unusedMonitor, 20, 1, &rtt, now)
	factory.CreateTestLogScore(t, serverID, pausedOldID, 20, 1, &rtt, now.Add(-30*time.Hour))

	rows, err := tdb.Queries().GetMonitorPriority(tdb.Context(), ntpdb.GetMonitorPriorityParams{
		ServerID:            serverID,
		SampleWindowSeconds: (24 * time.Hour).Seconds(),
	})
	testutil.AssertNoError(t, err, "GetMonitorPriority")
	testutil.AssertEqual(t, 3, len(rows), "number of rows (no score monitor, no monitor without server_scores row)")

	if rows[0].ID != activeID {
		t.Fatalf("first row = monitor %d, want the active one with samples (%d)", rows[0].ID, activeID)
	}
	testutil.AssertEqual(t, int64(10), rows[0].Count, "count of the active row")
	testutil.AssertTrue(t, rows[0].Healthy, "active row with samples is healthy")
	testutil.AssertEqual(t, ntpdb.ServerScoresStatusActive, rows[0].Status, "status of the active row")
	testutil.AssertTrue(t, rows[0].StatusSince.Valid, "status_since is set")

	wantStatus := map[int64]ntpdb.ServerScoresStatus{
		candidateID: ntpdb.ServerScoresStatusCandidate,
		pausedOldID: ntpdb.ServerScoresStatusPaused,
	}
	for _, r := range rows[1:] {
		want, ok := wantStatus[r.ID]
		if !ok {
			t.Errorf("unexpected row for monitor %d", r.ID)
			continue
		}
		testutil.AssertEqual(t, want, r.Status, "status of monitor %d", r.ID)
		testutil.AssertEqual(t, int64(0), r.Count, "count of monitor %d", r.ID)
		testutil.AssertTrue(t, !r.Healthy, "monitor %d without samples is not healthy", r.ID)
		testutil.AssertEqual(t, float64(0), r.AvgStep, "avg_step of monitor %d", r.ID)
	}
}

// cleanupServerAccount deletes one server's rows, including its status log,
// and the monitors and account that own them, and nothing else.
func cleanupServerAccount(t *testing.T, tdb *testutil.TestDB, serverID, accountID int64) {
	t.Helper()
	stmts := []struct {
		sql string
		id  int64
	}{
		{"DELETE FROM log_scores WHERE server_id = $1", serverID},
		{"DELETE FROM server_scores WHERE server_id = $1", serverID},
		{"DELETE FROM server_scores_status_log WHERE server_id = $1", serverID},
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

// setServerScoreStatus sets the status of one server_scores row.
func setServerScoreStatus(t *testing.T, tdb *testutil.TestDB, serverID, monitorID int64, status ntpdb.ServerScoresStatus) {
	t.Helper()
	err := tdb.Queries().UpdateServerScoreStatus(tdb.Context(), ntpdb.UpdateServerScoreStatusParams{
		Status:    status,
		MonitorID: monitorID,
		ServerID:  serverID,
	})
	if err != nil {
		t.Fatalf("UpdateServerScoreStatus(%d, %d, %s) error = %v", serverID, monitorID, status, err)
	}
}
