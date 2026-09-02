//go:build integration
// +build integration

package scorer

import (
	"testing"
	"time"

	"github.com/jackc/pgx/v5/pgtype"
	"github.com/prometheus/client_golang/prometheus"
	dto "github.com/prometheus/client_model/go"
	"go.ntppool.org/monitor/ntpdb"
	"go.ntppool.org/monitor/testutil"
)

// counterValue reads the current value of a Prometheus counter without
// pulling in prometheus/client_golang/prometheus/testutil, which would
// require updating go.mod / vendoring and isn't reachable from a
// build-tagged test file via `go mod tidy`.
func counterValue(t *testing.T, c prometheus.Counter) float64 {
	t.Helper()
	var m dto.Metric
	if err := c.Write(&m); err != nil {
		t.Fatalf("read counter: %v", err)
	}
	return m.GetCounter().GetValue()
}

func TestScorerRunner_FullCycle(t *testing.T) {
	tdb := testutil.NewTestDB(t)
	defer tdb.Close()
	defer tdb.CleanupTestData(t)

	factory := testutil.NewDataFactory(tdb)
	logger := testutil.NewTestLogger(t)

	// Setup test data
	setupScorerTestData(t, tdb, factory)

	// Create scorer runner
	reg := prometheus.NewRegistry()
	runner, err := New(logger.Logger(), tdb.Pool, reg)
	testutil.AssertNoError(t, err, "Failed to create scorer runner")

	t.Run("ProcessBacklog", func(t *testing.T) {
		// Insert test log scores using regular monitors (not scorers)
		now := time.Now()
		for i := 0; i < 100; i++ {
			factory.CreateTestLogScore(t, 3001, 2003, 20.0, 0.8, nil, now.Add(-time.Duration(i)*time.Minute))
		}

		// Run scorer
		count, err := runner.Run(tdb.Context())
		testutil.AssertNoError(t, err, "Scorer run failed")
		testutil.AssertTrue(t, count > 0, "Expected to process some log scores, got %d", count)

		// Verify server scores were updated (scorer 2001 processes data from monitor 2003)
		scores, err := tdb.Queries().GetServerScore(tdb.Context(), ntpdb.GetServerScoreParams{
			ServerID:  3001,
			MonitorID: 2001,
		})
		testutil.AssertNoError(t, err, "Failed to get server score")
		testutil.AssertEqual(t, "active", string(scores.Status), "Expected server score status to be active")
	})

	t.Run("NoDataToProcess", func(t *testing.T) {
		// Run scorer with no new data
		count, err := runner.Run(tdb.Context())
		testutil.AssertNoError(t, err, "Scorer run failed")
		testutil.AssertEqual(t, 0, count, "Expected no log scores to process")
	})

	t.Run("ConcurrentScorers", func(t *testing.T) {
		// This test verifies that multiple scorer instances don't interfere
		// Create second scorer
		reg2 := prometheus.NewRegistry()
		runner2, err := New(logger.Logger(), tdb.Pool, reg2)
		testutil.AssertNoError(t, err, "Failed to create second scorer runner")

		// Add more test data using regular monitor
		now := time.Now()
		for i := 0; i < 50; i++ {
			factory.CreateTestLogScore(t, 3002, 2004, 19.5, 0.7, nil, now.Add(-time.Duration(i)*time.Minute))
		}

		// Run both scorers concurrently
		done1 := make(chan error, 1)
		done2 := make(chan error, 1)

		go func() {
			_, err := runner.Run(tdb.Context())
			done1 <- err
		}()

		go func() {
			_, err := runner2.Run(tdb.Context())
			done2 <- err
		}()

		// Wait for both to complete
		err1 := <-done1
		err2 := <-done2

		testutil.AssertNoError(t, err1, "First scorer failed")
		testutil.AssertNoError(t, err2, "Second scorer failed")

		// Verify data consistency (scorer 2001 should have processed data from monitor 2004)
		scores, err := tdb.Queries().GetServerScore(tdb.Context(), ntpdb.GetServerScoreParams{
			ServerID:  3002,
			MonitorID: 2001,
		})
		testutil.AssertNoError(t, err, "Failed to get server score")
		testutil.AssertEqual(t, "active", string(scores.Status), "Expected server score status to be active")
	})

	t.Run("SkipsIterationsWhenLagging", func(t *testing.T) {
		// Insert 6 log_scores for a fresh server at 50-second intervals
		// (5 intervals × 50s = 250s, well within the 5-min mild-lag window),
		// all scores within 5% of each other, with the oldest ts ~30 min
		// ago so the scorer sees them as lagging. The first iteration
		// populates the batch-local shadow; the next 5 should skip.
		serverID := int64(3010)
		monitorID := int64(2003)

		// FK setup: server row + active server_score for the data-producing
		// monitor so GetScorerRecentScores finds the log_scores below.
		factory.CreateTestServer(t, serverID, "192.0.2.10", "v4", nil)
		factory.CreateTestServerScore(t, serverID, monitorID, "active", 20.0)

		base := time.Now().Add(-30 * time.Minute)
		for i := 0; i < 6; i++ {
			factory.CreateTestLogScore(
				t, serverID, monitorID,
				20.0+float64(i)*0.05, // mild drift, all within 5%
				0.5,
				nil,
				base.Add(time.Duration(i)*50*time.Second),
			)
		}

		skippedCounter, err := runner.m.skipped.GetMetricWithLabelValues(mainScorer)
		testutil.AssertNoError(t, err, "get skipped counter")
		before := counterValue(t, skippedCounter)

		count, err := runner.Run(tdb.Context())
		testutil.AssertNoError(t, err, "scorer run")
		testutil.AssertTrue(t, count >= 6, "processed at least 6 log_scores, got %d", count)

		after := counterValue(t, skippedCounter)
		skipped := int(after - before)
		if skipped < 5 {
			t.Errorf("expected at least 5 skipped iterations (5 of 6 after first compute), got %d", skipped)
		}
	})
}

func TestScorerRunner_ErrorHandling(t *testing.T) {
	tdb := testutil.NewTestDB(t)
	defer tdb.Close()
	defer tdb.CleanupTestData(t)

	logger := testutil.NewTestLogger(t)

	t.Run("NoScorersConfigured", func(t *testing.T) {
		// Start from a database with no scorers, whatever ran before.
		tdb.CleanupTestData(t)

		reg := prometheus.NewRegistry()
		runner, err := New(logger.Logger(), tdb.Pool, reg)
		testutil.AssertNoError(t, err, "Failed to create scorer runner")

		// Run reports an error rather than quietly processing nothing.
		_, err = runner.Run(tdb.Context())
		testutil.AssertError(t, err, "Expected error when no scorers configured")
	})

	t.Run("DatabaseConnectionLoss", func(t *testing.T) {
		// This test would require more sophisticated database mocking
		// For now, we'll skip it in the basic implementation
		t.Skip("Database connection loss testing requires advanced mocking")
	})
}

func TestScorerRunner_Performance(t *testing.T) {
	if testing.Short() {
		t.Skip("Skipping performance test in short mode")
	}

	tdb := testutil.NewTestDB(t)
	defer tdb.Close()
	defer tdb.CleanupTestData(t)

	factory := testutil.NewDataFactory(tdb)
	logger := testutil.NewTestLogger(t)

	// Setup test data
	setupScorerTestData(t, tdb, factory)

	// Create scorer runner
	reg := prometheus.NewRegistry()
	runner, err := New(logger.Logger(), tdb.Pool, reg)
	testutil.AssertNoError(t, err, "Failed to create scorer runner")

	t.Run("LargeDataset", func(t *testing.T) {
		// Insert large number of log scores
		now := time.Now()
		numScores := 10000

		start := time.Now()
		for i := 0; i < numScores; i++ {
			// Use regular monitor (2003) not scorer (2001) for generating log scores
			factory.CreateTestLogScore(t, 3001, 2003, 20.0+float64(i%100)/100, 0.8, nil, now.Add(-time.Duration(i)*time.Second))
		}
		insertTime := time.Since(start)
		t.Logf("Inserted %d log scores in %v", numScores, insertTime)

		// Run scorer and measure performance
		start = time.Now()
		count, err := runner.Run(tdb.Context())
		processingTime := time.Since(start)

		testutil.AssertNoError(t, err, "Scorer run failed")
		testutil.AssertTrue(t, count > 0, "Expected to process some log scores")

		t.Logf("Processed %d log scores in %v (%.2f scores/sec)", count, processingTime, float64(count)/processingTime.Seconds())

		// Assert reasonable performance (adjust thresholds as needed)
		testutil.AssertTrue(t, processingTime < 30*time.Second, "Processing took too long: %v", processingTime)
	})
}

// setupScorerTestData creates the basic test data needed for scorer tests
func setupScorerTestData(t *testing.T, tdb *testutil.TestDB, factory *testutil.DataFactory) {
	// Create test accounts
	factory.CreateTestAccount(t, 1001, "test1@example.com")
	factory.CreateTestAccount(t, 1002, "test2@example.com")

	// Create test monitors (scorers) - hostname must match scorer registry keys
	factory.CreateTestMonitorWithType(t, 2001, "recentmedian.test", 1001, "10.0.0.1", "active", "score")
	factory.CreateTestMonitorWithType(t, 2002, "every.test", 1002, "10.0.0.2", "active", "score")

	// Create additional regular monitors for generating log scores to process
	factory.CreateTestMonitor(t, 2003, "monitor1.test", 1001, "10.0.0.3", "active")
	factory.CreateTestMonitor(t, 2004, "monitor2.test", 1002, "10.0.0.4", "active")

	// Set the hostname field to match the scorer names
	var err error
	_, err = tdb.Exec(tdb.Context(), "UPDATE monitors SET hostname = 'recentmedian' WHERE id = 2001")
	if err != nil {
		t.Fatalf("Failed to set recentmedian hostname: %v", err)
	}
	_, err = tdb.Exec(tdb.Context(), "UPDATE monitors SET hostname = 'every' WHERE id = 2002")
	if err != nil {
		t.Fatalf("Failed to set every hostname: %v", err)
	}

	// Create test servers
	factory.CreateTestServer(t, 3001, "192.0.2.1", "v4", nil)
	factory.CreateTestServer(t, 3002, "192.0.2.2", "v4", nil)
	factory.CreateTestServer(t, 3003, "2001:db8::1", "v6", nil)

	// Create initial server scores
	factory.CreateTestServerScore(t, 3001, 2001, "candidate", 0)
	factory.CreateTestServerScore(t, 3002, 2002, "candidate", 0)

	// Create server scores for regular monitors (needed for GetScorerRecentScores query)
	// The recentmedian scorer needs these to find log scores from active monitors
	factory.CreateTestServerScore(t, 3001, 2003, "active", 20.0)
	factory.CreateTestServerScore(t, 3002, 2004, "active", 19.5)
	factory.CreateTestServerScore(t, 3003, 2003, "active", 18.0)

	// Set up system settings
	factory.SetSystemSetting(t, "scorer", `{"batch_size": 100}`)

	// Create an initial log score to reference in scorer_status
	now := time.Now()
	factory.CreateTestLogScore(t, 3001, 2003, 20.0, 0.8, nil, now.Add(-time.Hour))

	// Get the last inserted log score ID
	var logScoreID uint64
	err = tdb.QueryRow(tdb.Context(), "SELECT id FROM log_scores ORDER BY id DESC LIMIT 1").Scan(&logScoreID)
	if err != nil {
		t.Fatalf("Failed to get last insert ID: %v", err)
	}

	// Insert scorer status to enable the scorers (use valid log_score_id)
	// First delete any existing entries for these scorers, then insert
	_, err = tdb.Exec(tdb.Context(), "DELETE FROM scorer_status WHERE scorer_id IN (2001, 2002)")
	if err != nil {
		t.Fatalf("Failed to delete existing scorer status: %v", err)
	}
	insertScorerStatusSQL := `
		INSERT INTO scorer_status (scorer_id, log_score_id)
		VALUES (2001, $1), (2002, $2)
	`
	_, err = tdb.Exec(tdb.Context(), insertScorerStatusSQL, logScoreID, logScoreID)
	if err != nil {
		t.Fatalf("Failed to insert scorer status: %v", err)
	}
}

func TestScorerProcess_DedupesServerUpdates(t *testing.T) {
	tdb := testutil.NewTestDB(t)
	defer tdb.Close()
	defer tdb.CleanupTestData(t)

	factory := testutil.NewDataFactory(tdb)
	logger := testutil.NewTestLogger(t)

	setupScorerTestData(t, tdb, factory)

	reg := prometheus.NewRegistry()
	runner, err := New(logger.Logger(), tdb.Pool, reg)
	testutil.AssertNoError(t, err, "Failed to create scorer runner")

	// Insert n log_scores for the SAME server (3001) from regular monitor 2003,
	// with strictly increasing timestamps. After processing, the mainScorer
	// (recentmedian) should execute exactly ONE UpdateServer against
	// servers.id=3001, and servers.score_ts should equal the maximum timestamp.
	now := time.Now().UTC().Truncate(time.Second)
	const n = 5
	tsByOffset := func(i int) time.Time { return now.Add(time.Duration(i) * time.Minute) }
	for i := 0; i < n; i++ {
		factory.CreateTestLogScore(t, 3001, 2003, 20.0+float64(i)/10, 0.8, nil, tsByOffset(i))
	}
	maxTs := tsByOffset(n - 1)

	count, err := runner.Run(tdb.Context())
	testutil.AssertNoError(t, err, "Scorer run failed")
	testutil.AssertTrue(t, count >= n, "Expected to process at least %d log_scores, got %d", n, count)

	var scoreTs pgtype.Timestamptz
	err = tdb.QueryRow(tdb.Context(),
		"SELECT score_ts FROM servers WHERE id = 3001").Scan(&scoreTs)
	testutil.AssertNoError(t, err, "Failed to read servers.score_ts")
	testutil.AssertTrue(t, scoreTs.Valid, "Expected servers.score_ts to be non-NULL")
	if !scoreTs.Time.UTC().Equal(maxTs) {
		t.Errorf("servers.score_ts mismatch: got %s, want %s", scoreTs.Time.UTC(), maxTs)
	}

	got := counterValue(t, runner.m.sqlUpdates.WithLabelValues("update_server"))
	if got != 1 {
		t.Errorf("update_server counter: got %v, want 1 (dedup failed)", got)
	}

	gotDeduped := counterValue(t, runner.m.sqlUpdates.WithLabelValues("update_server_deduped"))
	if gotDeduped != float64(n-1) {
		t.Errorf("update_server_deduped counter: got %v, want %d", gotDeduped, n-1)
	}
}
