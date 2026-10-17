//go:build integration
// +build integration

package ntpdb_test

import (
	"maps"
	"testing"

	"go.ntppool.org/monitor/testutil"
)

// GetMonitorServerStatusCounts counts server_scores rows per monitor and
// status, and skips scorer monitors.
func TestGetMonitorServerStatusCounts(t *testing.T) {
	tdb := testutil.NewTestDB(t)
	defer tdb.Close()

	factory := testutil.NewDataFactory(tdb)

	// Ids outside 1000-9999 and distinct from the other ntpdb integration
	// tests; see TestGetMonitorPriority_AllRttNull.
	const (
		accountID = 90100
		serverA   = 90101
		serverB   = 90102
		serverC   = 90103
		monitorA  = 90104
		monitorB  = 90105
		scorerID  = 90106
	)

	cleanup := func() {
		stmts := []struct {
			sql string
			id  int64
		}{
			{"DELETE FROM server_scores WHERE monitor_id IN (SELECT id FROM monitors WHERE account_id = $1)", accountID},
			{"DELETE FROM servers WHERE id BETWEEN $1 AND $1 + 2", serverA},
			{"DELETE FROM monitors WHERE account_id = $1", accountID},
			{"DELETE FROM accounts WHERE id = $1", accountID},
		}
		for _, s := range stmts {
			if _, err := tdb.Exec(tdb.Context(), s.sql, s.id); err != nil {
				t.Logf("cleanup %q: %v", s.sql, err)
			}
		}
	}
	cleanup()
	defer cleanup()

	factory.CreateTestAccount(t, accountID, "statuscounts@example.com")
	factory.CreateTestServer(t, serverA, "192.0.2.30", "v4", nil)
	factory.CreateTestServer(t, serverB, "192.0.2.31", "v4", nil)
	factory.CreateTestServer(t, serverC, "192.0.2.32", "v4", nil)
	factory.CreateTestMonitor(t, monitorA, "statuscounts-a.test", accountID, "192.0.2.33", "active")
	factory.CreateTestMonitor(t, monitorB, "statuscounts-b.test", accountID, "192.0.2.34", "active")
	factory.CreateTestMonitorWithType(t, scorerID, "statuscounts-scorer.test", accountID, "192.0.2.35", "active", "score")

	if _, err := tdb.Exec(tdb.Context(), "UPDATE monitors SET id_token = 'tok-a' WHERE id = $1", monitorA); err != nil {
		t.Fatalf("set id_token: %v", err)
	}

	factory.CreateTestServerScore(t, serverA, monitorA, "active", 20)
	factory.CreateTestServerScore(t, serverB, monitorA, "active", 20)
	factory.CreateTestServerScore(t, serverC, monitorA, "testing", 20)
	factory.CreateTestServerScore(t, serverA, monitorB, "candidate", 20)
	factory.CreateTestServerScore(t, serverA, scorerID, "active", 20)
	factory.CreateTestServerScore(t, serverB, scorerID, "active", 20)

	rows, err := tdb.Queries().GetMonitorServerStatusCounts(tdb.Context())
	if err != nil {
		t.Fatalf("GetMonitorServerStatusCounts() error = %v", err)
	}

	// The database is shared with other tests; only look at our monitors.
	ours := map[string]bool{
		"statuscounts-a.test":      true,
		"statuscounts-b.test":      true,
		"statuscounts-scorer.test": true,
	}

	got := map[string]int64{}
	for _, r := range rows {
		if !ours[r.TlsName.String] {
			continue
		}
		got[r.IDToken.String+"|"+r.TlsName.String+"|"+string(r.Status)] = r.Servers
	}

	want := map[string]int64{
		"tok-a|statuscounts-a.test|active":  2,
		"tok-a|statuscounts-a.test|testing": 1,
		"|statuscounts-b.test|candidate":    1,
	}

	if !maps.Equal(got, want) {
		t.Errorf("got %v, want %v", got, want)
	}
}
