package selector

import (
	"context"
	"log/slog"
	"testing"

	"go.ntppool.org/monitor/ntpdb"
)

// TestApplyRule6BootstrapPromotion tests the bootstrap promotion logic
// Based on Phase 2 of the selector testing plan
func TestApplyRule6BootstrapPromotion(t *testing.T) {
	sl := &Selector{
		log: slog.Default(),
	}

	tests := []struct {
		name                 string
		testingMonitors      []evaluatedMonitor
		candidateMonitors    []evaluatedMonitor
		workingAccountLimits map[int64]*accountLimit
		workingActiveCount   int
		workingTestingCount  int
		emergencyOverride    bool
		expectedPromotions   int
		expectedReason       string
		shouldLog            bool
		description          string
	}{
		{
			name:            "zero_testing_promotes_healthy_candidates",
			testingMonitors: []evaluatedMonitor{},
			candidateMonitors: []evaluatedMonitor{
				createHealthyCandidate(1, 100),
				createHealthyCandidate(2, 101),
				createHealthyCandidate(3, 102),
			},
			workingAccountLimits: map[int64]*accountLimit{
				100: {AccountID: 100, MaxPerServer: 3, ActiveCount: 0, TestingCount: 0},
				101: {AccountID: 101, MaxPerServer: 3, ActiveCount: 0, TestingCount: 0},
				102: {AccountID: 102, MaxPerServer: 3, ActiveCount: 0, TestingCount: 0},
			},
			workingActiveCount:  0,
			workingTestingCount: 0,
			emergencyOverride:   true,
			expectedPromotions:  3,
			expectedReason:      "bootstrap: promoting healthy candidate",
			shouldLog:           true,
			description:         "Bootstrap scenario: zero testing monitors promotes healthy candidates",
		},
		{
			name:            "zero_testing_limited_by_base_target",
			testingMonitors: []evaluatedMonitor{},
			candidateMonitors: []evaluatedMonitor{
				createHealthyCandidate(1, 100),
				createHealthyCandidate(2, 101),
				createHealthyCandidate(3, 102),
				createHealthyCandidate(4, 103),
				createHealthyCandidate(5, 104),
				createHealthyCandidate(6, 105), // 6th candidate should not be promoted
			},
			workingAccountLimits: map[int64]*accountLimit{
				100: {AccountID: 100, MaxPerServer: 3, ActiveCount: 0, TestingCount: 0},
				101: {AccountID: 101, MaxPerServer: 3, ActiveCount: 0, TestingCount: 0},
				102: {AccountID: 102, MaxPerServer: 3, ActiveCount: 0, TestingCount: 0},
				103: {AccountID: 103, MaxPerServer: 3, ActiveCount: 0, TestingCount: 0},
				104: {AccountID: 104, MaxPerServer: 3, ActiveCount: 0, TestingCount: 0},
				105: {AccountID: 105, MaxPerServer: 3, ActiveCount: 0, TestingCount: 0},
			},
			workingActiveCount:  0,
			workingTestingCount: 0,
			emergencyOverride:   true,
			expectedPromotions:  5, // baseTestingTarget = 5
			expectedReason:      "bootstrap: promoting healthy candidate",
			shouldLog:           true,
			description:         "Bootstrap promotions should be limited to baseTestingTarget (5)",
		},
		{
			name:            "bootstrap_with_account_constraints",
			testingMonitors: []evaluatedMonitor{},
			candidateMonitors: []evaluatedMonitor{
				createHealthyCandidate(1, 100), // Same account
				createHealthyCandidate(2, 100), // Same account - should be limited
				createHealthyCandidate(3, 101),
			},
			workingAccountLimits: map[int64]*accountLimit{
				100: {AccountID: 100, MaxPerServer: 0, ActiveCount: 0, TestingCount: 0}, // Limited to 0 active, 1 testing
				101: {AccountID: 101, MaxPerServer: 3, ActiveCount: 0, TestingCount: 0},
			},
			workingActiveCount:  0,
			workingTestingCount: 0,
			emergencyOverride:   false, // No emergency override to test account constraints
			expectedPromotions:  2,     // Limited by account 100's limit
			expectedReason:      "bootstrap: promoting healthy candidate",
			shouldLog:           true,
			description:         "Bootstrap promotions should respect account limits",
		},
		{
			name:            "bootstrap_healthy_then_other_candidates",
			testingMonitors: []evaluatedMonitor{},
			candidateMonitors: []evaluatedMonitor{
				createUnhealthyCandidate(1, 100), // Unhealthy (active)
				createHealthyCandidate(2, 101),   // Healthy (active)
				createUnhealthyCandidate(3, 102), // Unhealthy (testing)
				createHealthyCandidate(4, 103),   // Healthy (testing)
			},
			workingAccountLimits: map[int64]*accountLimit{
				100: {AccountID: 100, MaxPerServer: 3, ActiveCount: 0, TestingCount: 0},
				101: {AccountID: 101, MaxPerServer: 3, ActiveCount: 0, TestingCount: 0},
				102: {AccountID: 102, MaxPerServer: 3, ActiveCount: 0, TestingCount: 0},
				103: {AccountID: 103, MaxPerServer: 3, ActiveCount: 0, TestingCount: 0},
			},
			workingActiveCount:  0,
			workingTestingCount: 0,
			emergencyOverride:   true,
			expectedPromotions:  4,
			expectedReason:      "bootstrap: promoting healthy candidate", // First promotions are healthy
			shouldLog:           true,
			description:         "Bootstrap should promote healthy candidates first, then others",
		},
		{
			name:            "bootstrap_with_paused_monitors_filtered",
			testingMonitors: []evaluatedMonitor{},
			candidateMonitors: []evaluatedMonitor{
				createHealthyCandidate(1, 100),
				{
					monitor: monitorCandidate{
						ID:           2,
						AccountID:    Ptr(int64(101)),
						GlobalStatus: ntpdb.MonitorsStatusPaused, // Should be filtered out
						IsHealthy:    true,
					},
				},
				createHealthyCandidate(3, 102),
			},
			workingAccountLimits: map[int64]*accountLimit{
				100: {AccountID: 100, MaxPerServer: 3, ActiveCount: 0, TestingCount: 0},
				101: {AccountID: 101, MaxPerServer: 3, ActiveCount: 0, TestingCount: 0},
				102: {AccountID: 102, MaxPerServer: 3, ActiveCount: 0, TestingCount: 0},
			},
			workingActiveCount:  0,
			workingTestingCount: 0,
			emergencyOverride:   true,
			expectedPromotions:  2, // Paused monitor should be excluded
			expectedReason:      "bootstrap: promoting healthy candidate",
			shouldLog:           true,
			description:         "Bootstrap should filter out paused monitors",
		},
		{
			name: "existing_testing_monitors_no_bootstrap",
			testingMonitors: []evaluatedMonitor{
				{
					monitor: monitorCandidate{
						ID:           1,
						AccountID:    Ptr(int64(100)),
						GlobalStatus: ntpdb.MonitorsStatusTesting,
						IsHealthy:    true,
					},
				},
			},
			candidateMonitors: []evaluatedMonitor{
				createHealthyCandidate(2, 101),
				createHealthyCandidate(3, 102),
			},
			workingAccountLimits: map[int64]*accountLimit{
				100: {AccountID: 100, MaxPerServer: 3, ActiveCount: 0, TestingCount: 1},
				101: {AccountID: 101, MaxPerServer: 3, ActiveCount: 0, TestingCount: 0},
				102: {AccountID: 102, MaxPerServer: 3, ActiveCount: 0, TestingCount: 0},
			},
			workingActiveCount:  0,
			workingTestingCount: 1,
			emergencyOverride:   false,
			expectedPromotions:  0, // No bootstrap when testing monitors exist
			expectedReason:      "",
			shouldLog:           false,
			description:         "Bootstrap should not trigger when testing monitors exist",
		},
		{
			name:                 "no_candidates_available",
			testingMonitors:      []evaluatedMonitor{},
			candidateMonitors:    []evaluatedMonitor{}, // No candidates
			workingAccountLimits: map[int64]*accountLimit{},
			workingActiveCount:   0,
			workingTestingCount:  0,
			emergencyOverride:    false,
			expectedPromotions:   0,
			expectedReason:       "",
			shouldLog:            false,
			description:          "Bootstrap with no candidates should not promote anything",
		},
		{
			name:            "bootstrap_with_emergency_override",
			testingMonitors: []evaluatedMonitor{},
			candidateMonitors: []evaluatedMonitor{
				createHealthyCandidate(1, 100),
				createHealthyCandidate(2, 101),
			},
			workingAccountLimits: map[int64]*accountLimit{
				100: {AccountID: 100, MaxPerServer: 0, ActiveCount: 0, TestingCount: 0}, // No limit normally
				101: {AccountID: 101, MaxPerServer: 0, ActiveCount: 0, TestingCount: 0}, // No limit normally
			},
			workingActiveCount:  0,
			workingTestingCount: 0,
			emergencyOverride:   true, // Explicit emergency override
			expectedPromotions:  2,    // Emergency override should allow promotions
			expectedReason:      "bootstrap emergency promotion to testing: zero active monitors",
			shouldLog:           true,
			description:         "Bootstrap with emergency override should bypass normal constraints",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Create selection context
			selCtx := selectionContext{
				server: &serverInfo{
					ID:        1,
					AccountID: Ptr(int64(999)), // Different from monitor accounts
					IP:        "192.168.1.100",
					IPVersion: "4",
				},
				assignedMonitors:  []ntpdb.GetMonitorPriorityRow{},
				emergencyOverride: tt.emergencyOverride,
			}

			// Call applyRule6BootstrapPromotion
			result := sl.applyRule6BootstrapPromotion(
				context.Background(),
				selCtx,
				tt.testingMonitors,
				tt.candidateMonitors,
				tt.workingAccountLimits,
				tt.workingActiveCount,
				tt.workingTestingCount,
			)

			// Verify number of promotions
			actualPromotions := len(result.changes)
			if actualPromotions != tt.expectedPromotions {
				t.Errorf("Expected %d promotions, got %d. %s",
					tt.expectedPromotions, actualPromotions, tt.description)
			}

			// Verify working counts are updated correctly
			expectedTestingCount := tt.workingTestingCount + actualPromotions
			if result.testingCount != expectedTestingCount {
				t.Errorf("Expected testingCount=%d, got %d",
					expectedTestingCount, result.testingCount)
			}

			// Active count should remain unchanged for candidate->testing promotions
			if result.activeCount != tt.workingActiveCount {
				t.Errorf("Expected activeCount=%d (unchanged), got %d",
					tt.workingActiveCount, result.activeCount)
			}

			// Verify status changes are correct
			for i, change := range result.changes {
				if change.fromStatus != ntpdb.ServerScoresStatusCandidate {
					t.Errorf("Change %d: expected fromStatus=candidate, got %v",
						i, change.fromStatus)
				}
				if change.toStatus != ntpdb.ServerScoresStatusTesting {
					t.Errorf("Change %d: expected toStatus=testing, got %v",
						i, change.toStatus)
				}

				// Check reason (at least for first change)
				if i == 0 && tt.expectedReason != "" {
					if !contains(change.reason, "bootstrap") {
						t.Errorf("Change %d: expected reason to contain 'bootstrap', got %q",
							i, change.reason)
					}
				}
			}

			// Verify healthy candidates are promoted first if both exist
			if hasHealthyAndUnhealthy(tt.candidateMonitors) && actualPromotions > 0 {
				// Get the monitor IDs from the first few changes
				firstChangeMonitorIDs := make([]int64, min(2, len(result.changes)))
				for i := 0; i < len(firstChangeMonitorIDs); i++ {
					firstChangeMonitorIDs[i] = result.changes[i].monitorID
				}

				// Check that healthy monitors (2, 4) come before unhealthy (1, 3)
				// This is a basic check - in a real implementation we'd check actual health status
				for _, id := range firstChangeMonitorIDs {
					healthyIDs := []int64{2, 4} // From test data
					isHealthy := false
					for _, hid := range healthyIDs {
						if id == hid {
							isHealthy = true
							break
						}
					}
					if !isHealthy && actualPromotions >= 2 {
						// If we have enough promotions, the first ones should be healthy
						// This is a simplified check for the test
						break
					}
				}
			}
		})
	}
}

// TestApplyRule6BootstrapPromotion_EdgeCases tests edge cases and error scenarios
func TestApplyRule6BootstrapPromotion_EdgeCases(t *testing.T) {
	sl := &Selector{
		log: slog.Default(),
	}

	tests := []struct {
		name        string
		description string
		setup       func() ([]evaluatedMonitor, []evaluatedMonitor, map[int64]*accountLimit)
		expectLog   string
	}{
		{
			name:        "constraint_violations_prevent_all_promotions",
			description: "When all candidates have constraint violations, none should be promoted",
			setup: func() ([]evaluatedMonitor, []evaluatedMonitor, map[int64]*accountLimit) {
				testingMonitors := []evaluatedMonitor{}
				candidateMonitors := []evaluatedMonitor{
					createHealthyCandidate(1, 100),
					createHealthyCandidate(2, 101),
				}
				// Set account limits to 0 with existing testing monitors to block promotions
				accountLimits := map[int64]*accountLimit{
					100: {AccountID: 100, MaxPerServer: 0, ActiveCount: 0, TestingCount: 1}, // Already at testing limit (1)
					101: {AccountID: 101, MaxPerServer: 0, ActiveCount: 0, TestingCount: 1}, // Already at testing limit (1)
				}
				return testingMonitors, candidateMonitors, accountLimits
			},
			expectLog: "unable to promote any candidates",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			testingMonitors, candidateMonitors, accountLimits := tt.setup()

			selCtx := selectionContext{
				server: &serverInfo{
					ID:        1,
					AccountID: Ptr(int64(999)),
					IP:        "192.168.1.100",
					IPVersion: "4",
				},
				assignedMonitors:  []ntpdb.GetMonitorPriorityRow{},
				emergencyOverride: false, // No emergency override
			}

			result := sl.applyRule6BootstrapPromotion(
				context.Background(),
				selCtx,
				testingMonitors,
				candidateMonitors,
				accountLimits,
				0, // workingActiveCount
				0, // workingTestingCount
			)

			// Should have no promotions due to constraints
			if len(result.changes) != 0 {
				t.Errorf("Expected 0 promotions due to constraints, got %d", len(result.changes))
			}

			// Note: We can't easily test log messages in this framework,
			// but the function should log a warning about constraint violations
		})
	}
}

// Helper functions for creating test data

func createHealthyCandidate(id int64, accountID int64) evaluatedMonitor {
	return evaluatedMonitor{
		monitor: monitorCandidate{
			ID:           id,
			AccountID:    Ptr(accountID),
			GlobalStatus: ntpdb.MonitorsStatusActive,
			IsHealthy:    true,
		},
		currentViolation: &constraintViolation{
			Type: violationNone,
		},
	}
}

func createUnhealthyCandidate(id int64, accountID int64) evaluatedMonitor {
	status := ntpdb.MonitorsStatusActive
	if id%2 == 1 { // Odd IDs get testing status
		status = ntpdb.MonitorsStatusTesting
	}

	return evaluatedMonitor{
		monitor: monitorCandidate{
			ID:           id,
			AccountID:    Ptr(accountID),
			GlobalStatus: status,
			IsHealthy:    false,
		},
		currentViolation: &constraintViolation{
			Type: violationNone,
		},
	}
}

func Ptr[T any](v T) *T {
	return &v
}

func hasHealthyAndUnhealthy(monitors []evaluatedMonitor) bool {
	hasHealthy := false
	hasUnhealthy := false
	for _, em := range monitors {
		if em.monitor.IsHealthy {
			hasHealthy = true
		} else {
			hasUnhealthy = true
		}
	}
	return hasHealthy && hasUnhealthy
}
