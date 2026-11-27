//go:build integration
// +build integration

package ntpdb_test

import (
	"testing"
	"time"

	"github.com/jackc/pgx/v5/pgtype"

	"go.ntppool.org/monitor/ntpdb"
	"go.ntppool.org/monitor/testutil"
)

// GetServers hands out a row only for the statuses it names, on the
// cadence of that status. interval_candidate paces candidate and paused
// rows; active and testing rows of a down server (score_raw <= -90) stay
// at the fixed 120 minutes whatever interval_candidate is.
func TestGetServers_StatusBranches(t *testing.T) {
	tdb := testutil.NewTestDB(t)
	defer tdb.Close()

	factory := testutil.NewDataFactory(tdb)

	// Ids outside 1000-9999, and only our own rows are deleted; see
	// TestGetMonitorPriority_AllRttNull.
	const (
		accountID = 90010
		monitorID = 90011
		serverID  = 90012
	)

	cleanupServerAccount(t, tdb, serverID, accountID)
	defer cleanupServerAccount(t, tdb, serverID, accountID)

	factory.CreateTestAccount(t, accountID, "get-servers@example.com")
	factory.CreateTestMonitor(t, monitorID, "get-servers.test", accountID, "192.0.2.30", "active")

	const (
		intervalActive  = 9 * time.Minute
		intervalTesting = 25 * time.Minute
	)

	tests := []struct {
		name              string
		status            ntpdb.ServerScoresStatus
		scoreRaw          float64
		queueAge          *time.Duration // nil: queue_ts is NULL
		intervalCandidate time.Duration
		want              bool
		wantDown          bool
	}{
		{"candidate never queued", ntpdb.ServerScoresStatusCandidate, 0, nil, 2 * time.Hour, true, false},
		{"candidate queued 1h ago", ntpdb.ServerScoresStatusCandidate, 0, ptr(time.Hour), 2 * time.Hour, false, false},
		{"candidate queued 2h1m ago", ntpdb.ServerScoresStatusCandidate, 0, ptr(2*time.Hour + time.Minute), 2 * time.Hour, true, false},
		{"paused queued 2h1m ago", ntpdb.ServerScoresStatusPaused, 0, ptr(2*time.Hour + time.Minute), 2 * time.Hour, true, false},
		{"active queued 10m ago", ntpdb.ServerScoresStatusActive, 10, ptr(10 * time.Minute), 2 * time.Hour, true, false},
		{"testing queued 20m ago", ntpdb.ServerScoresStatusTesting, 10, ptr(20 * time.Minute), 2 * time.Hour, false, false},
		{"active down server 2h1m ago, 4h candidate", ntpdb.ServerScoresStatusActive, -95, ptr(2*time.Hour + time.Minute), 4 * time.Hour, true, true},
		{"testing down server 1h ago, 4h candidate", ntpdb.ServerScoresStatusTesting, -95, ptr(time.Hour), 4 * time.Hour, false, false},
		{"candidate on a down server 2h1m ago", ntpdb.ServerScoresStatusCandidate, -95, ptr(2*time.Hour + time.Minute), 2 * time.Hour, true, false},
		{"candidate queued 3h ago, 4h candidate", ntpdb.ServerScoresStatusCandidate, 0, ptr(3 * time.Hour), 4 * time.Hour, false, false},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// One server per row, so each row is checked alone.
			if _, err := tdb.Exec(tdb.Context(), "DELETE FROM server_scores WHERE server_id = $1", serverID); err != nil {
				t.Fatalf("delete server_scores: %v", err)
			}
			factory.CreateTestServer(t, serverID, "192.0.2.31", "v4", nil)
			factory.CreateTestServerScore(t, serverID, monitorID, string(tt.status), tt.scoreRaw)

			var queueTs pgtype.Timestamptz
			if tt.queueAge != nil {
				queueTs = pgtype.Timestamptz{Time: time.Now().Add(-*tt.queueAge).Truncate(time.Microsecond), Valid: true}
			}
			_, err := tdb.Exec(tdb.Context(),
				"UPDATE server_scores SET queue_ts = $1 WHERE server_id = $2 AND monitor_id = $3",
				queueTs, serverID, monitorID)
			testutil.AssertNoError(t, err, "set queue_ts")

			rows, err := tdb.Queries().GetServers(tdb.Context(), ntpdb.GetServersParams{
				MonitorID:                monitorID,
				IpVersion:                ntpdb.ServersIpVersionV4,
				IntervalSeconds:          intervalActive.Seconds(),
				IntervalSecondsTesting:   intervalTesting.Seconds(),
				IntervalSecondsCandidate: tt.intervalCandidate.Seconds(),
				IntervalSecondsAll:       60,
				Limit:                    100,
				Offset:                   0,
			})
			testutil.AssertNoError(t, err, "GetServers")

			if !tt.want {
				testutil.AssertEqual(t, 0, len(rows), "rows handed out")
				return
			}

			testutil.AssertEqual(t, 1, len(rows), "rows handed out")
			got := rows[0]
			testutil.AssertEqual(t, int64(serverID), got.Server.ID, "server id")
			testutil.AssertEqual(t, tt.status, got.Status, "status")
			testutil.AssertEqual(t, tt.wantDown, got.Down, "down")
			testutil.AssertEqual(t, queueTs.Valid, got.PrevQueueTs.Valid, "PrevQueueTs validity")
			if queueTs.Valid {
				testutil.AssertTrue(t, queueTs.Time.Equal(got.PrevQueueTs.Time),
					"PrevQueueTs = %v, want %v", got.PrevQueueTs.Time, queueTs.Time)
			}
		})
	}
}

func ptr[T any](v T) *T { return &v }
