//go:build integration
// +build integration

package server

import (
	"net/netip"
	"testing"
	"time"

	"google.golang.org/protobuf/types/known/durationpb"
	"google.golang.org/protobuf/types/known/timestamppb"

	apiv2 "go.ntppool.org/monitor/gen/monitor/v2"
	"go.ntppool.org/monitor/monitorsettings"
	"go.ntppool.org/monitor/ntpdb"
	"go.ntppool.org/monitor/testutil"
)

// log_scores.status must hold the server_scores status the monitor had
// for the server when it submitted the result.
func TestProcessStatusRecordsServerStatus(t *testing.T) {
	tdb := testutil.NewTestDB(t)
	defer tdb.Close()

	factory := testutil.NewDataFactory(tdb)
	logger := testutil.NewTestLogger(t)

	// The packages' integration tests share one database and run in
	// parallel. testutil.CleanupTestData deletes every row with an id in
	// 1000-9999, so use ids outside that range and delete only our own
	// rows.
	const (
		accountID = 91000
		serverID  = 91001
		monitorID = 91002
		serverIP  = "192.0.2.30"
	)

	cleanup := func() {
		stmts := []struct {
			sql string
			id  int64
		}{
			{"DELETE FROM log_scores WHERE server_id = $1", serverID},
			{"DELETE FROM server_scores_status_log WHERE server_id = $1", serverID},
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
	cleanup()
	defer cleanup()

	ctx := tdb.Context()
	settings, err := monitorsettings.NewWatcher(ctx, tdb.Queries(), logger.Logger())
	testutil.AssertNoError(t, err, "monitor settings watcher")

	srv := &Server{
		ctx:      ctx,
		db:       ntpdb.NewWrappedQuerier(ntpdb.New(tdb.Pool)),
		settings: settings,
	}

	for _, want := range []string{"candidate", "testing", "active"} {
		t.Run(want, func(t *testing.T) {
			// start each case from empty tables; the same ids are reused
			cleanup()

			factory.CreateTestAccount(t, accountID, "test@example.com")
			factory.CreateTestServer(t, serverID, serverIP, "v4", nil)
			factory.CreateTestMonitor(t, monitorID, "status-log.test", accountID, "192.0.2.31", "active")
			factory.CreateTestServerScore(t, serverID, monitorID, want, 10)

			status := &apiv2.ServerStatus{
				IpBytes: netip.MustParseAddr(serverIP).AsSlice(),
				Ts:      timestamppb.Now(),
				Stratum: 2,
				Offset:  durationpb.New(time.Millisecond),
				Rtt:     durationpb.New(10 * time.Millisecond),
			}
			err := srv.processStatus(ctx, &ntpdb.Monitor{ID: monitorID}, status, newSubmitCounters())
			testutil.AssertNoError(t, err, "processStatus")

			var got *string
			err = tdb.QueryRow(ctx,
				"SELECT status::text FROM log_scores WHERE server_id = $1 AND monitor_id = $2 ORDER BY id DESC LIMIT 1",
				serverID, monitorID).Scan(&got)
			testutil.AssertNoError(t, err, "read log_scores.status")
			if got == nil {
				t.Fatalf("log_scores.status is NULL, want %q", want)
			}
			testutil.AssertEqual(t, want, *got, "log_scores.status")
		})
	}
}
