//go:build integration
// +build integration

package ntpdb_test

import (
	"testing"
	"time"

	"go.ntppool.org/monitor/ntpdb"
	"go.ntppool.org/monitor/testutil"
)

// server_scores.status_since is set by a trigger when status changes, by
// any writer, and left alone by updates that don't change it.
func TestStatusSince(t *testing.T) {
	tdb := testutil.NewTestDB(t)
	defer tdb.Close()

	factory := testutil.NewDataFactory(tdb)

	// Ids outside 1000-9999 and distinct from the other ntpdb integration
	// tests; see TestGetMonitorPriority_AllRttNull.
	const (
		accountID = 90110
		serverID  = 90111
		monitorID = 90112
	)

	cleanupServerAccount(t, tdb, serverID, accountID)
	defer cleanupServerAccount(t, tdb, serverID, accountID)

	factory.CreateTestAccount(t, accountID, "statussince@example.com")
	factory.CreateTestServer(t, serverID, "192.0.2.40", "v4", nil)
	factory.CreateTestMonitor(t, monitorID, "statussince.test", accountID, "192.0.2.41", "active")
	factory.CreateTestServerScore(t, serverID, monitorID, "candidate", 20)

	statusSince := func() time.Time {
		t.Helper()
		ss, err := tdb.Queries().GetServerScore(tdb.Context(), ntpdb.GetServerScoreParams{
			ServerID:  serverID,
			MonitorID: monitorID,
		})
		if err != nil {
			t.Fatalf("GetServerScore() error = %v", err)
		}
		if !ss.StatusSince.Valid {
			t.Fatal("status_since is NULL")
		}
		return ss.StatusSince.Time
	}

	setStatus := func(status ntpdb.ServerScoresStatus) func() {
		return func() { setServerScoreStatus(t, tdb, serverID, monitorID, status) }
	}

	// Each step runs after the previous one; the status_since checked is
	// compared with the value before the step.
	steps := []struct {
		name        string
		apply       func()
		wantChanged bool
	}{
		{
			name:        "same status",
			apply:       setStatus(ntpdb.ServerScoresStatusCandidate),
			wantChanged: false,
		},
		{
			name: "other column",
			apply: func() {
				_, err := tdb.Exec(tdb.Context(),
					"UPDATE server_scores SET score_raw = 10 WHERE server_id = $1 AND monitor_id = $2",
					serverID, monitorID)
				if err != nil {
					t.Fatalf("update score_raw: %v", err)
				}
			},
			wantChanged: false,
		},
		{
			name:        "candidate to testing",
			apply:       setStatus(ntpdb.ServerScoresStatusTesting),
			wantChanged: true,
		},
		{
			name:        "testing again",
			apply:       setStatus(ntpdb.ServerScoresStatusTesting),
			wantChanged: false,
		},
		{
			name: "upsert with a new status",
			apply: func() {
				factory.CreateTestServerScore(t, serverID, monitorID, "active", 20)
			},
			wantChanged: true,
		},
	}

	prev := statusSince()
	for _, tt := range steps {
		time.Sleep(50 * time.Millisecond)
		tt.apply()

		got := statusSince()
		switch {
		case tt.wantChanged && !got.After(prev):
			t.Errorf("%s: status_since = %v, want later than %v", tt.name, got, prev)
		case !tt.wantChanged && !got.Equal(prev):
			t.Errorf("%s: status_since = %v, want unchanged %v", tt.name, got, prev)
		}
		prev = got
	}
}
