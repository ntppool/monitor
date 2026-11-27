//go:build integration
// +build integration

package ntpdb_test

import (
	"slices"
	"testing"

	"go.ntppool.org/monitor/testutil"
)

// GetServersMonitorReview returns the servers that are due, and leaves out
// servers that are deleted.
func TestGetServersMonitorReview_SkipsDeleted(t *testing.T) {
	tdb := testutil.NewTestDB(t)
	defer tdb.Close()

	factory := testutil.NewDataFactory(tdb)

	// Ids outside 1000-9999 and distinct from the other ntpdb integration
	// tests.
	const (
		liveServer    = 90140
		deletedServer = 90141
		futureServer  = 90142
		notDueServer  = 90143
	)
	ids := []int64{liveServer, deletedServer, futureServer, notDueServer}

	cleanup := func() {
		for _, sql := range []string{
			"DELETE FROM servers_monitor_review WHERE server_id = ANY($1)",
			"DELETE FROM servers WHERE id = ANY($1)",
		} {
			if _, err := tdb.Exec(tdb.Context(), sql, ids); err != nil {
				t.Logf("cleanup %q: %v", sql, err)
			}
		}
	}
	cleanup()
	defer cleanup()

	factory.CreateTestServer(t, liveServer, "192.0.2.60", "v4", nil)
	factory.CreateTestServer(t, deletedServer, "192.0.2.61", "v4", nil)
	factory.CreateTestServer(t, futureServer, "192.0.2.62", "v4", nil)
	factory.CreateTestServer(t, notDueServer, "192.0.2.63", "v4", nil)

	if _, err := tdb.Exec(tdb.Context(),
		"UPDATE servers SET deletion_on = CURRENT_DATE - 1 WHERE id = $1", deletedServer); err != nil {
		t.Fatalf("set deletion_on: %v", err)
	}
	// A deletion in the future doesn't stop reviews
	if _, err := tdb.Exec(tdb.Context(),
		"UPDATE servers SET deletion_on = CURRENT_DATE + 30 WHERE id = $1", futureServer); err != nil {
		t.Fatalf("set deletion_on: %v", err)
	}

	// The query returns at most 10 servers ordered by next_review and the
	// database is shared, so make our due rows the first ones.
	for _, id := range ids {
		nextReview := "'1970-01-01'::timestamptz"
		if id == notDueServer {
			nextReview = "NOW() + interval '1 hour'"
		}
		if _, err := tdb.Exec(tdb.Context(),
			"INSERT INTO servers_monitor_review (server_id, next_review) VALUES ($1, "+nextReview+")", id); err != nil {
			t.Fatalf("insert review row: %v", err)
		}
	}

	got, err := tdb.Queries().GetServersMonitorReview(tdb.Context())
	if err != nil {
		t.Fatalf("GetServersMonitorReview() error = %v", err)
	}

	for _, tt := range []struct {
		name string
		id   int64
		want bool
	}{
		{"live", liveServer, true},
		{"deleted", deletedServer, false},
		{"deletion in the future", futureServer, true},
		{"not due", notDueServer, false},
	} {
		if has := slices.Contains(got, tt.id); has != tt.want {
			t.Errorf("%s: server %d returned = %v, want %v (got %v)", tt.name, tt.id, has, tt.want, got)
		}
	}
}
