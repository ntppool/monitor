//go:build integration
// +build integration

package ntpdb_test

import (
	"testing"

	"go.ntppool.org/monitor/ntpdb"
	"go.ntppool.org/monitor/testutil"
)

// Promotions are counted per boundary from the status log: only updates
// inside the window, only candidate -> testing and testing -> active.
func TestGetServerPromotionCounts(t *testing.T) {
	tdb := testutil.NewTestDB(t)
	defer tdb.Close()

	// Ids outside 1000-9999 and distinct from the other ntpdb integration
	// tests.
	const (
		serverID      = 90120
		otherServerID = 90121
		monitorID     = 90122
		scoreMonitor  = 90123
		accountID     = 90124
	)

	cleanup := func() {
		for _, id := range []int64{serverID, otherServerID} {
			if _, err := tdb.Exec(tdb.Context(),
				"DELETE FROM server_scores_status_log WHERE server_id = $1", id); err != nil {
				t.Logf("cleanup: %v", err)
			}
		}
		if _, err := tdb.Exec(tdb.Context(), "DELETE FROM monitors WHERE account_id = $1", accountID); err != nil {
			t.Logf("cleanup: %v", err)
		}
		if _, err := tdb.Exec(tdb.Context(), "DELETE FROM accounts WHERE id = $1", accountID); err != nil {
			t.Logf("cleanup: %v", err)
		}
	}
	cleanup()
	defer cleanup()

	factory := testutil.NewDataFactory(tdb)
	factory.CreateTestAccount(t, accountID, "promotioncounts-log@example.com")
	factory.CreateTestMonitor(t, monitorID, "promo-log.test", accountID, "192.0.2.60", "active")
	factory.CreateTestMonitorWithType(t, scoreMonitor, "promo-log-score.test", accountID, "192.0.2.61", "active", "score")

	insertFor := func(logMonitorID, serverID int64, ago, op, oldStatus, newStatus string) {
		t.Helper()
		// NULLIF: insert and delete rows have one status missing
		_, err := tdb.Exec(tdb.Context(), `
			INSERT INTO server_scores_status_log (server_id, monitor_id, ts, op, old_status, new_status)
			VALUES ($1, $2, NOW() - $3::interval, $4,
				NULLIF($5, '')::server_scores_status, NULLIF($6, '')::server_scores_status)`,
			serverID, logMonitorID, ago, op, oldStatus, newStatus)
		if err != nil {
			t.Fatalf("insert status log row: %v", err)
		}
	}
	insert := func(serverID int64, ago, op, oldStatus, newStatus string) {
		t.Helper()
		insertFor(monitorID, serverID, ago, op, oldStatus, newStatus)
	}

	insert(serverID, "1 hour", "update", "candidate", "testing")
	insert(serverID, "23 hours", "update", "candidate", "testing")
	insert(serverID, "25 hours", "update", "candidate", "testing") // outside the window
	insert(serverID, "2 hours", "update", "active", "testing")     // a swap's demotion
	insert(serverID, "2 hours", "update", "testing", "active")
	insert(serverID, "2 hours", "update", "testing", "candidate") // a demotion
	insert(serverID, "3 hours", "snapshot", "", "testing")
	insert(serverID, "3 hours", "insert", "", "testing")
	insert(otherServerID, "1 hour", "update", "candidate", "testing")
	insert(otherServerID, "1 hour", "update", "testing", "active")
	// a scorer monitor's promotions never count
	insertFor(scoreMonitor, serverID, "1 hour", "update", "candidate", "testing")
	insertFor(scoreMonitor, serverID, "1 hour", "update", "testing", "active")

	tests := []struct {
		name        string
		window      float64
		wantTesting int64
		wantActive  int64
		forServerID int64
	}{
		{"24h", 24 * 3600, 2, 1, serverID},
		{"26h", 26 * 3600, 3, 1, serverID},
		{"1h30m", 90 * 60, 1, 0, serverID},
		{"other server", 24 * 3600, 1, 1, otherServerID},
		{"server without rows", 24 * 3600, 0, 0, 90129},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := tdb.Queries().GetServerPromotionCounts(tdb.Context(), ntpdb.GetServerPromotionCountsParams{
				ServerID:      tt.forServerID,
				WindowSeconds: tt.window,
			})
			if err != nil {
				t.Fatalf("GetServerPromotionCounts() error = %v", err)
			}
			testutil.AssertEqual(t, tt.wantTesting, got.IntoTesting, "IntoTesting")
			testutil.AssertEqual(t, tt.wantActive, got.IntoActive, "IntoActive")
		})
	}
}

// A testing -> active swap writes two log rows: the promotion into active
// and the demotion of the active monitor to testing. Only the first is a
// promotion.
func TestGetServerPromotionCountsSwap(t *testing.T) {
	tdb := testutil.NewTestDB(t)
	defer tdb.Close()

	factory := testutil.NewDataFactory(tdb)

	const (
		accountID    = 90130
		serverID     = 90131
		activeID     = 90132
		testingID    = 90133
		candidateID  = 90134
		windowSecond = 24 * 3600
	)

	cleanupServerAccount(t, tdb, serverID, accountID)
	defer cleanupServerAccount(t, tdb, serverID, accountID)

	factory.CreateTestAccount(t, accountID, "promotioncounts@example.com")
	factory.CreateTestServer(t, serverID, "192.0.2.50", "v4", nil)
	factory.CreateTestMonitor(t, activeID, "promo-active.test", accountID, "192.0.2.51", "active")
	factory.CreateTestMonitor(t, testingID, "promo-testing.test", accountID, "192.0.2.52", "active")
	factory.CreateTestMonitor(t, candidateID, "promo-candidate.test", accountID, "192.0.2.53", "active")
	factory.CreateTestServerScore(t, serverID, activeID, "active", 20)
	factory.CreateTestServerScore(t, serverID, testingID, "testing", 20)
	factory.CreateTestServerScore(t, serverID, candidateID, "candidate", 20)

	setStatus := func(monitorID int64, status ntpdb.ServerScoresStatus) {
		t.Helper()
		setServerScoreStatus(t, tdb, serverID, monitorID, status)
	}
	counts := func() ntpdb.GetServerPromotionCountsRow {
		t.Helper()
		got, err := tdb.Queries().GetServerPromotionCounts(tdb.Context(), ntpdb.GetServerPromotionCountsParams{
			ServerID:      serverID,
			WindowSeconds: windowSecond,
		})
		if err != nil {
			t.Fatalf("GetServerPromotionCounts() error = %v", err)
		}
		return got
	}

	// the inserts above are not promotions
	if got := counts(); got.IntoTesting != 0 || got.IntoActive != 0 {
		t.Fatalf("counts after inserts = %+v, want zero", got)
	}

	// the swap
	setStatus(activeID, ntpdb.ServerScoresStatusTesting)
	setStatus(testingID, ntpdb.ServerScoresStatusActive)
	if got := counts(); got.IntoTesting != 0 || got.IntoActive != 1 {
		t.Errorf("counts after a swap = %+v, want into testing 0, into active 1", got)
	}

	// a candidate promoted into testing counts for the other boundary
	setStatus(candidateID, ntpdb.ServerScoresStatusTesting)
	if got := counts(); got.IntoTesting != 1 || got.IntoActive != 1 {
		t.Errorf("counts after a candidate promotion = %+v, want 1 and 1", got)
	}
}
