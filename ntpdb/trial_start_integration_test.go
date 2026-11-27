//go:build integration
// +build integration

package ntpdb_test

import (
	"testing"
	"time"

	"go.ntppool.org/monitor/ntpdb"
	"go.ntppool.org/monitor/testutil"
)

// GetTrialStartedSince is true for a candidate row inserted, or a reserve
// row made a candidate, within the window. A testing row demoted to
// candidate is not a trial, and neither is a scorer's own row.
func TestGetTrialStartedSince(t *testing.T) {
	tdb := testutil.NewTestDB(t)
	defer tdb.Close()

	// The status log has no foreign keys; ids outside 1000-9999 keep other
	// packages' cleanup away from these rows.
	const (
		serverInsert  = 93001
		serverReserve = 93002
		serverDemoted = 93003
		serverNone    = 93004
		serverScorer  = 93005
		monitorID     = 93010
		scorerID      = 93011
		accountID     = 93020
	)
	servers := []int64{serverInsert, serverReserve, serverDemoted, serverNone, serverScorer}
	cleanup := func() {
		for _, id := range servers {
			if _, err := tdb.Exec(tdb.Context(), "DELETE FROM server_scores_status_log WHERE server_id = $1", id); err != nil {
				t.Logf("cleanup server %d: %v", id, err)
			}
		}
		if _, err := tdb.Exec(tdb.Context(), "DELETE FROM monitors WHERE id = $1", scorerID); err != nil {
			t.Logf("cleanup monitor %d: %v", scorerID, err)
		}
		if _, err := tdb.Exec(tdb.Context(), "DELETE FROM accounts WHERE id = $1", accountID); err != nil {
			t.Logf("cleanup account %d: %v", accountID, err)
		}
	}
	cleanup()
	defer cleanup()

	factory := testutil.NewDataFactory(tdb)
	factory.CreateTestAccount(t, accountID, "scorer-account")
	factory.CreateTestMonitorWithType(t, scorerID, "scorer.test", accountID, "192.0.2.93", "active", "score")

	now := time.Now()
	logMonitorRow := func(serverID, monitorID int64, ago time.Duration, op string, oldStatus, newStatus any) {
		t.Helper()
		_, err := tdb.Exec(tdb.Context(), `INSERT INTO server_scores_status_log
			(server_id, monitor_id, ts, op, old_status, new_status)
			VALUES ($1, $2, $3, $4, $5::server_scores_status, $6::server_scores_status)`,
			serverID, monitorID, now.Add(-ago), op, oldStatus, newStatus)
		if err != nil {
			t.Fatalf("insert status log row for server %d: %v", serverID, err)
		}
	}
	logRow := func(serverID int64, ago time.Duration, op string, oldStatus, newStatus any) {
		t.Helper()
		logMonitorRow(serverID, monitorID, ago, op, oldStatus, newStatus)
	}
	logRow(serverInsert, 13*time.Hour, "insert", nil, "candidate")
	logRow(serverReserve, time.Hour, "update", "reserve", "candidate")
	logRow(serverDemoted, time.Hour, "update", "testing", "candidate")
	// other rows of the log don't start a trial
	logRow(serverNone, time.Hour, "update", "candidate", "reserve")
	logRow(serverNone, time.Hour, "insert", nil, "reserve")
	logRow(serverNone, time.Hour, "update", "paused", "reserve")
	// the scorer inserts its own row with the default status candidate
	logMonitorRow(serverScorer, scorerID, time.Hour, "insert", nil, "candidate")

	tests := []struct {
		name     string
		serverID int64
		since    time.Duration
		want     bool
	}{
		{"insert 13h ago, 12h", serverInsert, 12 * time.Hour, false},
		{"insert 13h ago, 14h", serverInsert, 14 * time.Hour, true},
		{"reserve to candidate 1h ago, 12h", serverReserve, 12 * time.Hour, true},
		{"testing to candidate 1h ago, 12h", serverDemoted, 12 * time.Hour, false},
		{"no trial start, 12h", serverNone, 12 * time.Hour, false},
		{"scorer row inserted 1h ago, 12h", serverScorer, 12 * time.Hour, false},
		{"no rows at all", 93099, 12 * time.Hour, false},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := tdb.Queries().GetTrialStartedSince(tdb.Context(), ntpdb.GetTrialStartedSinceParams{
				ServerID:     tt.serverID,
				SinceSeconds: tt.since.Seconds(),
			})
			testutil.AssertNoError(t, err, "GetTrialStartedSince")
			testutil.AssertEqual(t, tt.want, got, "started")
		})
	}
}
