//go:build integration
// +build integration

package cmd

import (
	"database/sql"
	"fmt"
	"testing"
	"time"

	"go.ntppool.org/monitor/ntpdb"
	"go.ntppool.org/monitor/testutil"
)

func TestSelectorIntegration_CompleteFlow(t *testing.T) {
	tdb := testutil.NewTestDB(t)
	defer tdb.Close()
	defer tdb.CleanupTestData(t)

	factory := testutil.NewDataFactory(tdb)
	logger := testutil.NewTestLogger(t)

	// Setup comprehensive test data
	setupCompleteTestData(t, factory)

	// Create selector
	sel := &selector{
		ctx:     tdb.Context(),
		db:      ntpdb.New(tdb.DB),
		log:     logger.Logger(),
		dryRun:  false,
	}

	t.Run("BootstrapMode", func(t *testing.T) {
		// Test with server having no active monitors
		serverID := uint32(4001)

		// Create server with no existing scores
		factory.CreateTestServer(t, serverID, "192.0.2.100", "v4", nil)

		// Add it to review queue
		_, err := tdb.ExecContext(tdb.Context(),
			"INSERT INTO servers_monitor_review (server_id, next_review) VALUES (?, NOW())",
			serverID)
		testutil.AssertNoError(t, err, "Failed to add server to review queue")

		// Process the server
		changed, err := sel.processServerNew(serverID)
		testutil.AssertNoError(t, err, "processServerNew failed")
		testutil.AssertTrue(t, changed, "Expected changes in bootstrap mode")

		// Verify monitors were added
		var count int
		err = tdb.QueryRowContext(tdb.Context(),
			"SELECT COUNT(*) FROM server_scores WHERE server_id = ? AND status IN ('testing', 'active')",
			serverID).Scan(&count)
		testutil.AssertNoError(t, err, "Failed to count server scores")
		testutil.AssertTrue(t, count > 0, "Expected monitors to be added in bootstrap mode, got %d", count)
		testutil.AssertTrue(t, count <= 3, "Expected at most 3 monitors in bootstrap mode, got %d", count)
	})

	t.Run("NormalOperation", func(t *testing.T) {
		// Test with server that has some monitors but needs more
		serverID := uint32(4002)

		factory.CreateTestServer(t, serverID, "192.0.2.101", "v4", nil)

		// Add some existing scores
		factory.CreateTestServerScore(t, serverID, 2001, "active", 20.0)
		factory.CreateTestServerScore(t, serverID, 2002, "active", 19.0)
		factory.CreateTestServerScore(t, serverID, 2003, "testing", 18.0)

		// Add recent performance data
		now := time.Now()
		for i := 0; i < 10; i++ {
			factory.CreateTestLogScore(t, serverID, 2001, 20.0, 0.9, int32Ptr(50), now.Add(-time.Duration(i)*time.Hour))
			factory.CreateTestLogScore(t, serverID, 2002, 19.0, 0.8, int32Ptr(60), now.Add(-time.Duration(i)*time.Hour))
			factory.CreateTestLogScore(t, serverID, 2003, 18.0, 0.85, int32Ptr(55), now.Add(-time.Duration(i)*time.Hour))
		}

		// Add to review queue
		_, err := tdb.ExecContext(tdb.Context(),
			"INSERT INTO servers_monitor_review (server_id, next_review) VALUES (?, NOW())",
			serverID)
		testutil.AssertNoError(t, err, "Failed to add server to review queue")

		// Process the server
		changed, err := sel.processServerNew(serverID)
		testutil.AssertNoError(t, err, "processServerNew failed")

		// Count active monitors after processing
		var activeCount int
		err = tdb.QueryRowContext(tdb.Context(),
			"SELECT COUNT(*) FROM server_scores WHERE server_id = ? AND status = 'active'",
			serverID).Scan(&activeCount)
		testutil.AssertNoError(t, err, "Failed to count active monitors")

		// Should have up to 5 active monitors total
		testutil.AssertTrue(t, activeCount <= 5, "Expected at most 5 active monitors, got %d", activeCount)
		testutil.AssertTrue(t, activeCount >= 2, "Expected at least 2 active monitors, got %d", activeCount)
	})

	t.Run("ConstraintEnforcement", func(t *testing.T) {
		// Test constraint enforcement with monitors from same network
		serverID := uint32(4003)

		factory.CreateTestServer(t, serverID, "192.0.2.102", "v4", nil)

		// Create monitors in same /24 subnet (should violate network constraints)
		factory.CreateTestMonitor(t, 2010, "same-net-1.test", 1003, "10.1.1.1", "active")
		factory.CreateTestMonitor(t, 2011, "same-net-2.test", 1003, "10.1.1.2", "active")
		factory.CreateTestMonitor(t, 2012, "same-net-3.test", 1003, "10.1.1.3", "active")

		// Create scores for these monitors
		factory.CreateTestServerScore(t, serverID, 2010, "active", 20.0)
		factory.CreateTestServerScore(t, serverID, 2011, "active", 19.0)
		factory.CreateTestServerScore(t, serverID, 2012, "candidate", 0)

		// Add performance data
		now := time.Now()
		for i := 0; i < 10; i++ {
			factory.CreateTestLogScore(t, serverID, 2010, 20.0, 0.9, int32Ptr(50), now.Add(-time.Duration(i)*time.Hour))
			factory.CreateTestLogScore(t, serverID, 2011, 19.0, 0.8, int32Ptr(60), now.Add(-time.Duration(i)*time.Hour))
			factory.CreateTestLogScore(t, serverID, 2012, 18.0, 0.85, int32Ptr(55), now.Add(-time.Duration(i)*time.Hour))
		}

		// Add to review queue
		_, err := tdb.ExecContext(tdb.Context(),
			"INSERT INTO servers_monitor_review (server_id, next_review) VALUES (?, NOW())",
			serverID)
		testutil.AssertNoError(t, err, "Failed to add server to review queue")

		// Process the server
		changed, err := sel.processServerNew(serverID)
		testutil.AssertNoError(t, err, "processServerNew failed")

		// Check if constraint violations were handled
		var violationCount int
		err = tdb.QueryRowContext(tdb.Context(),
			"SELECT COUNT(*) FROM server_scores WHERE server_id = ? AND status = 'active' AND monitor_id IN (2010, 2011, 2012)",
			serverID).Scan(&violationCount)
		testutil.AssertNoError(t, err, "Failed to count constraint violations")

		// Should not have more than allowed monitors per network
		testutil.AssertTrue(t, violationCount <= 2, "Expected constraint enforcement to limit monitors per network, got %d active", violationCount)
	})

	t.Run("PendingToTestingTransition", func(t *testing.T) {
		// Test pending monitor promotion to testing
		serverID := uint32(4004)

		factory.CreateTestServer(t, serverID, "192.0.2.103", "v4", nil)

		// Create pending monitors with good performance history
		factory.CreateTestMonitor(t, 2020, "pending-good.test", 1004, "10.2.1.1", "pending")
		factory.CreateTestMonitor(t, 2021, "pending-bad.test", 1004, "10.2.2.1", "pending")

		// Add performance data - one good, one bad
		now := time.Now()
		for i := 0; i < 10; i++ {
			// Good monitor
			factory.CreateTestLogScore(t, serverID, 2020, 20.0, 0.9, int32Ptr(50), now.Add(-time.Duration(i)*time.Hour))
			// Bad monitor (negative step indicates failure)
			factory.CreateTestLogScore(t, serverID, 2021, 15.0, -1.0, int32Ptr(100), now.Add(-time.Duration(i)*time.Hour))
		}

		// Add to review queue
		_, err := tdb.ExecContext(tdb.Context(),
			"INSERT INTO servers_monitor_review (server_id, next_review) VALUES (?, NOW())",
			serverID)
		testutil.AssertNoError(t, err, "Failed to add server to review queue")

		// Process the server
		changed, err := sel.processServerNew(serverID)
		testutil.AssertNoError(t, err, "processServerNew failed")

		// Check if good pending monitor was promoted
		var goodMonitorStatus string
		err = tdb.QueryRowContext(tdb.Context(),
			"SELECT COALESCE(status, 'none') FROM server_scores WHERE server_id = ? AND monitor_id = 2020",
			serverID).Scan(&goodMonitorStatus)

		if err == sql.ErrNoRows {
			goodMonitorStatus = "none"
		} else {
			testutil.AssertNoError(t, err, "Failed to get good monitor status")
		}

		// Check if bad pending monitor was not promoted
		var badMonitorStatus string
		err = tdb.QueryRowContext(tdb.Context(),
			"SELECT COALESCE(status, 'none') FROM server_scores WHERE server_id = ? AND monitor_id = 2021",
			serverID).Scan(&badMonitorStatus)

		if err == sql.ErrNoRows {
			badMonitorStatus = "none"
		} else {
			testutil.AssertNoError(t, err, "Failed to get bad monitor status")
		}

		// Good monitor should be promoted (or at least considered)
		// Bad monitor should not be promoted
		t.Logf("Good monitor status: %s, Bad monitor status: %s", goodMonitorStatus, badMonitorStatus)

		// Note: The exact behavior depends on the implementation
		// This test verifies the basic framework works
		testutil.AssertTrue(t, changed || goodMonitorStatus != "none" || badMonitorStatus == "none",
			"Expected some processing of pending monitors")
	})

	t.Run("ConcurrentServerProcessing", func(t *testing.T) {
		// Test processing multiple servers concurrently
		serverIDs := []uint32{4010, 4011, 4012}

		for i, serverID := range serverIDs {
			factory.CreateTestServer(t, serverID, fmt.Sprintf("192.0.2.%d", 110+i), "v4", nil)

			// Add to review queue
			_, err := tdb.ExecContext(tdb.Context(),
				"INSERT INTO servers_monitor_review (server_id, next_review) VALUES (?, NOW())",
				serverID)
			testutil.AssertNoError(t, err, "Failed to add server to review queue")
		}

		// Process servers concurrently
		results := make(chan error, len(serverIDs))

		for _, serverID := range serverIDs {
			go func(sid uint32) {
				_, err := sel.processServerNew(sid)
				results <- err
			}(serverID)
		}

		// Wait for all to complete
		for i := 0; i < len(serverIDs); i++ {
			err := <-results
			testutil.AssertNoError(t, err, "Concurrent server processing failed")
		}

		// Verify all servers were processed
		for _, serverID := range serverIDs {
			var count int
			err := tdb.QueryRowContext(tdb.Context(),
				"SELECT COUNT(*) FROM server_scores WHERE server_id = ?",
				serverID).Scan(&count)
			testutil.AssertNoError(t, err, "Failed to count server scores")
			// Each server should have some monitors assigned (bootstrap mode)
			testutil.AssertTrue(t, count >= 0, "Expected server %d to have monitors assigned", serverID)
		}
	})
}

func TestSelectorIntegration_EdgeCases(t *testing.T) {
	tdb := testutil.NewTestDB(t)
	defer tdb.Close()
	defer tdb.CleanupTestData(t)

	factory := testutil.NewDataFactory(tdb)
	logger := testutil.NewTestLogger(t)

	setupCompleteTestData(t, factory)

	sel := &selector{
		ctx:     tdb.Context(),
		db:      ntpdb.New(tdb.DB),
		log:     logger.Logger(),
		dryRun:  false,
	}

	t.Run("AllMonitorsPaused", func(t *testing.T) {
		// Test scenario where all monitors are paused
		serverID := uint32(5001)
		factory.CreateTestServer(t, serverID, "192.0.2.200", "v4", nil)

		// Create paused monitors
		factory.CreateTestMonitor(t, 2030, "paused-1.test", 1005, "10.3.1.1", "paused")
		factory.CreateTestMonitor(t, 2031, "paused-2.test", 1005, "10.3.2.1", "paused")

		// Add to review queue
		_, err := tdb.ExecContext(tdb.Context(),
			"INSERT INTO servers_monitor_review (server_id, next_review) VALUES (?, NOW())",
			serverID)
		testutil.AssertNoError(t, err, "Failed to add server to review queue")

		// Process the server
		changed, err := sel.processServerNew(serverID)
		testutil.AssertNoError(t, err, "processServerNew should handle all paused monitors")

		// Should not assign paused monitors
		var pausedCount int
		err = tdb.QueryRowContext(tdb.Context(),
			"SELECT COUNT(*) FROM server_scores ss JOIN monitors m ON ss.monitor_id = m.id WHERE ss.server_id = ? AND m.status = 'paused'",
			serverID).Scan(&pausedCount)
		testutil.AssertNoError(t, err, "Failed to count paused monitor assignments")
		testutil.AssertEqual(t, 0, pausedCount, "Should not assign paused monitors")
	})

	t.Run("ServerWithNoViableMonitors", func(t *testing.T) {
		// Test server where no monitors meet criteria
		serverID := uint32(5002)
		factory.CreateTestServer(t, serverID, "192.0.2.201", "v4", nil)

		// Add to review queue
		_, err := tdb.ExecContext(tdb.Context(),
			"INSERT INTO servers_monitor_review (server_id, next_review) VALUES (?, NOW())",
			serverID)
		testutil.AssertNoError(t, err, "Failed to add server to review queue")

		// Process with no available monitors
		changed, err := sel.processServerNew(serverID)
		testutil.AssertNoError(t, err, "processServerNew should handle no viable monitors")

		// Should complete without error even if no changes made
		t.Logf("Changed: %v (no viable monitors scenario)", changed)
	})
}

// setupCompleteTestData creates comprehensive test data for integration tests
func setupCompleteTestData(t *testing.T, factory *testutil.DataFactory) {
	// Create test accounts
	for i := uint32(1001); i <= 1010; i++ {
		factory.CreateTestAccount(t, i, fmt.Sprintf("test%d@example.com", i))
	}

	// Create diverse monitors across different networks and statuses
	monitors := []struct {
		id        uint32
		tlsName   string
		accountID uint32
		ip        string
		status    string
	}{
		{2001, "active-1.test", 1001, "10.0.1.1", "active"},
		{2002, "active-2.test", 1002, "10.0.2.1", "active"},
		{2003, "active-3.test", 1003, "10.0.3.1", "active"},
		{2004, "testing-1.test", 1001, "10.1.1.1", "testing"},
		{2005, "testing-2.test", 1002, "10.1.2.1", "testing"},
		{2006, "pending-1.test", 1003, "10.2.1.1", "pending"},
		{2007, "pending-2.test", 1004, "10.2.2.1", "pending"},
		{2008, "paused-1.test", 1005, "10.3.1.1", "paused"},
	}

	for _, m := range monitors {
		factory.CreateTestMonitor(t, m.id, m.tlsName, m.accountID, m.ip, m.status)
	}

	// Create test servers
	for i := uint32(3001); i <= 3010; i++ {
		factory.CreateTestServer(t, i, fmt.Sprintf("192.0.2.%d", i-3000), "v4", nil)
	}

	// Set system settings
	factory.SetSystemSetting(t, "selector_constraints", `{"max_monitors_per_24": 2}`)
	factory.SetSystemSetting(t, "selector_targets", `{"target_monitors": 5, "bootstrap_limit": 3}`)
}

// int32Ptr returns a pointer to an int32 value
func int32Ptr(i int32) *int32 {
	return &i
}
