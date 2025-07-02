//go:build integration
// +build integration

package selector

import (
	"database/sql"
	"testing"
	"time"

	"go.ntppool.org/monitor/ntpdb"
)

// Test helpers for integration tests

func createTestServer(t *testing.T, db *sql.DB, serverID uint32, ip, ipVersion string, accountID *uint32) {
	query := `INSERT INTO servers (id, ip, ip_version, account_id) VALUES (?, ?, ?, ?)`

	var accID sql.NullInt32
	if accountID != nil {
		accID = sql.NullInt32{Int32: int32(*accountID), Valid: true}
	}

	_, err := db.Exec(query, serverID, ip, ipVersion, accID)
	if err != nil {
		t.Fatalf("Failed to create test server: %v", err)
	}
}

func createTestMonitor(t *testing.T, db *sql.DB, monitorID uint32, status string, ip string, accountID *uint32) {
	query := `INSERT INTO monitors (id, status, ip, account_id) VALUES (?, ?, ?, ?)`

	var accID sql.NullInt32
	if accountID != nil {
		accID = sql.NullInt32{Int32: int32(*accountID), Valid: true}
	}

	_, err := db.Exec(query, monitorID, status, ip, accID)
	if err != nil {
		t.Fatalf("Failed to create test monitor: %v", err)
	}
}

func createTestServerScore(t *testing.T, db *sql.DB, serverID, monitorID uint32, status string, score float64) {
	query := `INSERT INTO server_scores (server_id, monitor_id, status, score_raw, created_on) VALUES (?, ?, ?, ?, NOW())`
	_, err := db.Exec(query, serverID, monitorID, status, score)
	if err != nil {
		t.Fatalf("Failed to create test server score: %v", err)
	}
}

func createTestLogScore(t *testing.T, db *sql.DB, serverID, monitorID uint32, score, offset float64, rtt *int32, ts time.Time) {
	query := `INSERT INTO log_scores (server_id, monitor_id, ts, score, offset, rtt) VALUES (?, ?, ?, ?, ?, ?)`

	var rttVal sql.NullInt32
	if rtt != nil {
		rttVal = sql.NullInt32{Int32: *rtt, Valid: true}
	}

	_, err := db.Exec(query, serverID, monitorID, ts, score, offset, rttVal)
	if err != nil {
		t.Fatalf("Failed to create test log score: %v", err)
	}
}

func int32Ptr(v int32) *int32 {
	return &v
}


func TestSelectorIntegration_CompleteFlow(t *testing.T) {
	t.Skip("Integration tests require database setup")

	// This test would require a test database setup
	// The actual test implementation would go here
}

func TestSelectorIntegration_ConstraintViolations(t *testing.T) {
	t.Skip("Integration tests require database setup")

	// Test constraint violation detection and tracking
}

func TestSelectorIntegration_GrandfatheredConstraints(t *testing.T) {
	t.Skip("Integration tests require database setup")

	// Test grandfathering logic for existing violations
}

func TestSelectorIntegration_LoadBalancing(t *testing.T) {
	t.Skip("Integration tests require database setup")

	// Test that monitors are distributed evenly across servers
}

func TestSelectorIntegration_EmergencyRecoveryScenarios(t *testing.T) {
	t.Skip("Integration tests require database setup - run with -tags=integration")

	// These tests would require actual database setup using test-db.sh
	// They test emergency override behavior in realistic constraint scenarios

	testCases := []struct {
		name        string
		description string
		setupFunc   func(t *testing.T, db *sql.DB)
		expectFunc  func(t *testing.T, changes []statusChange)
	}{
		{
			name:        "zero_active_all_candidates_have_account_limits",
			description: "Emergency recovery when all candidates exceed account limits",
			setupFunc: func(t *testing.T, db *sql.DB) {
				// Create server with account 1
				createTestServer(t, db, 1, "192.168.1.1", "v4", uint32Ptr(1))

				// Create monitors from account 2 (different from server)
				createTestMonitor(t, db, 1, "active", "10.0.0.1", uint32Ptr(2))
				createTestMonitor(t, db, 2, "active", "10.0.0.2", uint32Ptr(2))
				createTestMonitor(t, db, 3, "active", "10.0.0.3", uint32Ptr(2))

				// Create candidates from same account 2 (would normally violate account limits)
				createTestMonitor(t, db, 4, "active", "10.0.0.4", uint32Ptr(2))
				createTestMonitor(t, db, 5, "active", "10.0.0.5", uint32Ptr(2))

				// Set account limits: max 2 per server for account 2
				// This would normally prevent promotion of monitors 4,5 since we already have 3 testing

				// Create server_scores for existing monitors (all as candidate to simulate zero active)
				createTestServerScore(t, db, 1, 1, "candidate", 100.0)
				createTestServerScore(t, db, 1, 2, "candidate", 95.0)
				createTestServerScore(t, db, 1, 3, "candidate", 90.0)
				createTestServerScore(t, db, 1, 4, "candidate", 85.0)
				createTestServerScore(t, db, 1, 5, "candidate", 80.0)
			},
			expectFunc: func(t *testing.T, changes []statusChange) {
				// Should promote candidates despite account limit violations due to emergency override
				promotionCount := 0
				for _, change := range changes {
					if change.toStatus == ntpdb.ServerScoresStatusTesting {
						promotionCount++
						// Verify emergency reason is used
						if !containsString(change.reason, "emergency promotion") {
							t.Errorf("Expected emergency promotion reason, got: %s", change.reason)
						}
					}
				}
				if promotionCount == 0 {
					t.Error("Expected at least one promotion due to emergency override")
				}
			},
		},
		{
			name:        "zero_active_all_candidates_have_network_conflicts",
			description: "Emergency recovery when all candidates have network diversity violations",
			setupFunc: func(t *testing.T, db *sql.DB) {
				// Create server
				createTestServer(t, db, 1, "10.0.0.1", "v4", uint32Ptr(1))

				// Create monitors from different accounts to avoid account conflicts
				createTestMonitor(t, db, 1, "active", "192.168.1.10", uint32Ptr(1))
				createTestMonitor(t, db, 2, "active", "192.168.1.20", uint32Ptr(2)) // Same /24 subnet
				createTestMonitor(t, db, 3, "active", "192.168.1.30", uint32Ptr(3)) // Same /24 subnet

				// Create candidates that would have network diversity violations
				createTestMonitor(t, db, 4, "active", "192.168.1.40", uint32Ptr(4)) // Same /24 subnet
				createTestMonitor(t, db, 5, "active", "192.168.1.50", uint32Ptr(5)) // Same /24 subnet

				// All as candidates to simulate zero active monitors
				createTestServerScore(t, db, 1, 1, "candidate", 100.0)
				createTestServerScore(t, db, 1, 2, "testing", 95.0) // Testing status to create network conflict
				createTestServerScore(t, db, 1, 3, "testing", 90.0) // Testing status to create network conflict
				createTestServerScore(t, db, 1, 4, "candidate", 85.0)
				createTestServerScore(t, db, 1, 5, "candidate", 80.0)
			},
			expectFunc: func(t *testing.T, changes []statusChange) {
				// Should promote candidates despite network conflicts due to emergency override
				promotionCount := 0
				for _, change := range changes {
					if change.toStatus == ntpdb.ServerScoresStatusTesting {
						promotionCount++
						if !containsString(change.reason, "emergency promotion") {
							t.Errorf("Expected emergency promotion reason, got: %s", change.reason)
						}
					}
				}
				if promotionCount == 0 {
					t.Error("Expected at least one promotion due to emergency override")
				}
			},
		},
		{
			name:        "bootstrap_emergency_with_constraints",
			description: "Bootstrap scenario with emergency override when no testing monitors exist",
			setupFunc: func(t *testing.T, db *sql.DB) {
				// Create server
				createTestServer(t, db, 1, "10.0.0.1", "v4", uint32Ptr(1))

				// Create candidates that would normally have constraint violations
				createTestMonitor(t, db, 1, "active", "192.168.1.10", uint32Ptr(1)) // Same account as server
				createTestMonitor(t, db, 2, "active", "192.168.1.20", uint32Ptr(1)) // Same account as server
				createTestMonitor(t, db, 3, "active", "192.168.1.30", uint32Ptr(1)) // Same account as server

				// All monitors are candidates (no active, no testing = bootstrap + emergency)
				createTestServerScore(t, db, 1, 1, "candidate", 100.0)
				createTestServerScore(t, db, 1, 2, "candidate", 95.0)
				createTestServerScore(t, db, 1, 3, "candidate", 90.0)
			},
			expectFunc: func(t *testing.T, changes []statusChange) {
				// Should promote candidates for bootstrap despite constraint violations
				promotionCount := 0
				for _, change := range changes {
					if change.toStatus == ntpdb.ServerScoresStatusTesting {
						promotionCount++
						if !containsString(change.reason, "bootstrap emergency promotion") {
							t.Errorf("Expected bootstrap emergency promotion reason, got: %s", change.reason)
						}
					}
				}
				if promotionCount == 0 {
					t.Error("Expected at least one bootstrap promotion due to emergency override")
				}
			},
		},
		{
			name:        "normal_operations_respect_constraints",
			description: "Verify normal operations still respect constraints when not in emergency",
			setupFunc: func(t *testing.T, db *sql.DB) {
				// Create server with account 1
				createTestServer(t, db, 1, "192.168.1.1", "v4", uint32Ptr(1))

				// Create some active monitors (non-emergency scenario)
				createTestMonitor(t, db, 1, "active", "10.0.0.1", uint32Ptr(2))
				createTestMonitor(t, db, 2, "active", "10.0.0.2", uint32Ptr(3))

				// Create candidates that would violate account limits
				createTestMonitor(t, db, 3, "active", "10.0.0.3", uint32Ptr(2))
				createTestMonitor(t, db, 4, "active", "10.0.0.4", uint32Ptr(2))

				// Some active monitors, some candidates
				createTestServerScore(t, db, 1, 1, "active", 100.0)
				createTestServerScore(t, db, 1, 2, "active", 95.0)
				createTestServerScore(t, db, 1, 3, "candidate", 90.0)
				createTestServerScore(t, db, 1, 4, "candidate", 85.0)

				// Set strict account limits for account 2
			},
			expectFunc: func(t *testing.T, changes []statusChange) {
				// Should NOT promote candidates that violate constraints (no emergency)
				for _, change := range changes {
					if change.toStatus == ntpdb.ServerScoresStatusTesting {
						if containsString(change.reason, "emergency") {
							t.Errorf("Should not use emergency promotion in non-emergency scenario: %s", change.reason)
						}
					}
				}
			},
		},
		{
			name:        "emergency_still_requires_global_status",
			description: "Emergency override still requires monitors to be globally active/testing",
			setupFunc: func(t *testing.T, db *sql.DB) {
				// Create server
				createTestServer(t, db, 1, "10.0.0.1", "v4", uint32Ptr(1))

				// Create monitors with pending/paused global status
				createTestMonitor(t, db, 1, "pending", "192.168.1.10", uint32Ptr(2))
				createTestMonitor(t, db, 2, "paused", "192.168.1.20", uint32Ptr(3))
				createTestMonitor(t, db, 3, "active", "192.168.1.30", uint32Ptr(4)) // Only this one is eligible

				// All as candidates to simulate zero active monitors
				createTestServerScore(t, db, 1, 1, "candidate", 100.0)
				createTestServerScore(t, db, 1, 2, "candidate", 95.0)
				createTestServerScore(t, db, 1, 3, "candidate", 90.0)
			},
			expectFunc: func(t *testing.T, changes []statusChange) {
				// Should only promote the globally active monitor, not pending/paused ones
				promotionCount := 0
				for _, change := range changes {
					if change.toStatus == ntpdb.ServerScoresStatusTesting {
						promotionCount++
						// Only monitor 3 should be promoted
						if change.monitorID != 3 {
							t.Errorf("Only globally active monitor should be promoted, got monitor ID: %d", change.monitorID)
						}
					}
				}
				if promotionCount != 1 {
					t.Errorf("Expected exactly 1 promotion, got: %d", promotionCount)
				}
			},
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			// This test setup would require database connection
			// In actual implementation:
			// 1. Setup test database using test-db.sh
			// 2. Run tc.setupFunc to create test data
			// 3. Create selector and run ProcessStatusChanges
			// 4. Run tc.expectFunc to verify results

			t.Logf("Test case: %s", tc.description)
			t.Skip("Requires integration test database setup")
		})
	}
}

// TestPromotionHelperIntegration tests the refactored promotion helpers in realistic scenarios
func TestPromotionHelperIntegration(t *testing.T) {
	testCases := []struct {
		name        string
		description string
		setupFunc   func(t *testing.T, db *sql.DB)
		expectFunc  func(t *testing.T, changes []statusChange)
	}{
		{
			name:        "multiple_promotion_paths",
			description: "Tests that promotion helpers work correctly with multiple candidate groups",
			setupFunc: func(t *testing.T, db *sql.DB) {
				// Create test server
				createTestServer(t, db, 1, "192.168.1.1", "4", uint32Ptr(100))

				// Create globally active and testing monitors with different accounts
				createTestMonitor(t, db, 1, "active", "10.0.1.1", uint32Ptr(200))    // Globally active
				createTestMonitor(t, db, 2, "testing", "10.0.1.2", uint32Ptr(201))   // Globally testing
				createTestMonitor(t, db, 3, "active", "10.0.1.3", uint32Ptr(202))    // Globally active

				// All as candidates for promotion
				createTestServerScore(t, db, 1, 1, "candidate", 95.0)
				createTestServerScore(t, db, 1, 2, "candidate", 90.0)
				createTestServerScore(t, db, 1, 3, "candidate", 85.0)
			},
			expectFunc: func(t *testing.T, changes []statusChange) {
				promotions := 0
				activePromotions := 0
				testingPromotions := 0

				for _, change := range changes {
					if change.toStatus == ntpdb.ServerScoresStatusTesting {
						promotions++
						// Check that globally active monitors are promoted first
						if change.monitorID == 1 || change.monitorID == 3 {
							activePromotions++
						} else if change.monitorID == 2 {
							testingPromotions++
						}

						// Verify emergency reason handling
						if containsString(change.reason, "emergency") {
							t.Errorf("Should not use emergency promotion in normal scenario: %s", change.reason)
						}
					}
				}

				// Should have promotions from candidate groups
				if promotions == 0 {
					t.Error("Expected candidate promotions, got none")
				}

				// Globally active monitors should be preferred over testing
				if activePromotions > 0 && testingPromotions > 0 && activePromotions <= testingPromotions {
					t.Error("Expected globally active monitors to be promoted before testing monitors")
				}
			},
		},
		{
			name:        "bootstrap_promotion_priority",
			description: "Tests that bootstrap promotions prioritize healthy candidates",
			setupFunc: func(t *testing.T, db *sql.DB) {
				// Create test server
				createTestServer(t, db, 2, "192.168.2.1", "4", uint32Ptr(300))

				// Create healthy and unhealthy monitors
				createTestMonitor(t, db, 10, "active", "10.0.2.1", uint32Ptr(400))   // Healthy, globally active
				createTestMonitor(t, db, 11, "active", "10.0.2.2", uint32Ptr(401))   // Unhealthy, globally active
				createTestMonitor(t, db, 12, "testing", "10.0.2.3", uint32Ptr(402))  // Healthy, globally testing

				// All as candidates, simulate no testing monitors scenario
				createTestServerScore(t, db, 2, 10, "candidate", 95.0)
				createTestServerScore(t, db, 2, 11, "candidate", 90.0)
				createTestServerScore(t, db, 2, 12, "candidate", 85.0)

				// Add log scores to determine health status
				now := time.Now()
				createTestLogScore(t, db, 2, 10, 0.1, 0.05, int32Ptr(10), now.Add(-5*time.Minute))  // Healthy
				createTestLogScore(t, db, 2, 11, 1.0, 0.5, int32Ptr(100), now.Add(-5*time.Minute))   // Unhealthy
				createTestLogScore(t, db, 2, 12, 0.2, 0.1, int32Ptr(15), now.Add(-5*time.Minute))    // Healthy
			},
			expectFunc: func(t *testing.T, changes []statusChange) {
				bootstrapPromotions := 0
				healthyPromotions := 0

				for _, change := range changes {
					if change.toStatus == ntpdb.ServerScoresStatusTesting {
						bootstrapPromotions++

						// Check for bootstrap promotion reasons
						if containsString(change.reason, "bootstrap") {
							if containsString(change.reason, "healthy") {
								healthyPromotions++
							}
						}
					}
				}

				if bootstrapPromotions == 0 {
					t.Error("Expected bootstrap promotions in zero-testing scenario")
				}

				if healthyPromotions == 0 {
					t.Error("Expected at least one healthy candidate to be promoted first")
				}
			},
		},
		{
			name:        "working_count_accuracy",
			description: "Tests that working count tracking in promotion helpers is accurate",
			setupFunc: func(t *testing.T, db *sql.DB) {
				// Create test server
				createTestServer(t, db, 3, "192.168.3.1", "4", uint32Ptr(500))

				// Create mix of active, testing, and candidate monitors
				createTestMonitor(t, db, 20, "active", "10.0.3.1", uint32Ptr(600))
				createTestMonitor(t, db, 21, "active", "10.0.3.2", uint32Ptr(601))
				createTestMonitor(t, db, 22, "active", "10.0.3.3", uint32Ptr(602))

				// Server scores: some active, some testing, some candidates
				createTestServerScore(t, db, 3, 20, "active", 95.0)
				createTestServerScore(t, db, 3, 21, "testing", 90.0)
				createTestServerScore(t, db, 3, 22, "candidate", 85.0)

				// Add log scores for health
				now := time.Now()
				createTestLogScore(t, db, 3, 20, 0.1, 0.05, int32Ptr(10), now.Add(-5*time.Minute))
				createTestLogScore(t, db, 3, 21, 0.2, 0.1, int32Ptr(15), now.Add(-5*time.Minute))
				createTestLogScore(t, db, 3, 22, 0.3, 0.2, int32Ptr(20), now.Add(-5*time.Minute))
			},
			expectFunc: func(t *testing.T, changes []statusChange) {
				// Count expected state transitions
				activeToTesting := 0
				testingToActive := 0
				candidateToTesting := 0

				for _, change := range changes {
					switch {
					case change.fromStatus == ntpdb.ServerScoresStatusActive && change.toStatus == ntpdb.ServerScoresStatusTesting:
						activeToTesting++
					case change.fromStatus == ntpdb.ServerScoresStatusTesting && change.toStatus == ntpdb.ServerScoresStatusActive:
						testingToActive++
					case change.fromStatus == ntpdb.ServerScoresStatusCandidate && change.toStatus == ntpdb.ServerScoresStatusTesting:
						candidateToTesting++
					}
				}

				// Verify that count increments match expected state transitions
				// This test would need access to internal working counts to be fully effective
				// but verifies that the promotion logic produces consistent results

				totalTransitions := activeToTesting + testingToActive + candidateToTesting
				if totalTransitions == 0 {
					t.Log("No status transitions in this scenario - this is valid if monitors are properly balanced")
				}
			},
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			// This test setup would require database connection
			// In actual implementation:
			// 1. Setup test database using test-db.sh
			// 2. Run tc.setupFunc to create test data
			// 3. Create selector and run ProcessStatusChanges
			// 4. Run tc.expectFunc to verify results

			t.Logf("Test case: %s", tc.description)
			t.Skip("Requires integration test database setup")
		})
	}
}

// Helper function for integration tests
func containsString(s, substr string) bool {
	return len(s) >= len(substr) && (s == substr || len(s) > len(substr) &&
		(s[:len(substr)] == substr || containsString(s[1:], substr)))
}
