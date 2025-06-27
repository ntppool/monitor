package selector

import (
	"context"
	"log/slog"
	"os"
	"testing"

	"go.ntppool.org/monitor/ntpdb"
)

func TestDynamicTestingPoolSizing(t *testing.T) {
	tests := []struct {
		name            string
		activeCount     int
		testingCount    int
		expectedTarget  int
		expectedDemoted int
	}{
		{
			name:            "at_target_active_base_testing",
			activeCount:     7, // at target
			testingCount:    5, // at base
			expectedTarget:  5, // base only
			expectedDemoted: 0,
		},
		{
			name:            "below_target_need_larger_testing_pool",
			activeCount:     3, // 4 below target
			testingCount:    11,
			expectedTarget:  9, // base(5) + gap(4) = 9
			expectedDemoted: 2, // 11 - 9 = 2
		},
		{
			name:            "below_target_testing_at_dynamic_target",
			activeCount:     4, // 3 below target
			testingCount:    8, // exactly at dynamic target
			expectedTarget:  8, // base(5) + gap(3) = 8
			expectedDemoted: 0,
		},
		{
			name:            "no_active_bootstrap_case",
			activeCount:     0, // 7 below target
			testingCount:    15,
			expectedTarget:  12, // base(5) + gap(7) = 12
			expectedDemoted: 3,  // 15 - 12 = 3
		},
		{
			name:            "above_target_active_minimal_testing",
			activeCount:     8, // 1 above target
			testingCount:    7,
			expectedTarget:  5, // just base target
			expectedDemoted: 2, // 7 - 5 = 2
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			sl := &Selector{
				log: slog.New(slog.NewTextHandler(os.Stderr, &slog.HandlerOptions{Level: slog.LevelDebug})),
			}
			ctx := context.Background()

			// Create test monitors
			monitors := make([]evaluatedMonitor, tt.activeCount+tt.testingCount)

			// Add active monitors
			for i := 0; i < tt.activeCount; i++ {
				monitors[i] = evaluatedMonitor{
					monitor: monitorCandidate{
						ID:           uint32(i + 1),
						ServerStatus: ntpdb.ServerScoresStatusActive,
						GlobalStatus: ntpdb.MonitorsStatusActive,
						IsHealthy:    true,
						HasMetrics:   true,
					},
					currentViolation: &constraintViolation{Type: violationNone},
					recommendedState: candidateIn,
				}
			}

			// Add testing monitors (with priority values so worst performers are last)
			for i := 0; i < tt.testingCount; i++ {
				monitors[tt.activeCount+i] = evaluatedMonitor{
					monitor: monitorCandidate{
						ID:           uint32(tt.activeCount + i + 1),
						ServerStatus: ntpdb.ServerScoresStatusTesting,
						GlobalStatus: ntpdb.MonitorsStatusActive,
						IsHealthy:    true,
						HasMetrics:   true,
						RTT:          float64(10 + i), // ascending RTT (higher = worse performance)
					},
					currentViolation: &constraintViolation{Type: violationNone},
					recommendedState: candidateIn,
				}
			}

			server := &serverInfo{ID: 1}
			accountLimits := make(map[uint32]*accountLimit)

			// Run the selection rules
			changes := sl.applySelectionRules(ctx, monitors, server, accountLimits, []ntpdb.GetMonitorPriorityRow{})

			// Count demotions from testing to candidate
			demotions := 0
			for _, change := range changes {
				if change.fromStatus == ntpdb.ServerScoresStatusTesting && change.toStatus == ntpdb.ServerScoresStatusCandidate {
					demotions++
				}
			}

			if demotions != tt.expectedDemoted {
				t.Errorf("expected %d demotions, got %d", tt.expectedDemoted, demotions)
			}

			// Verify the demoted monitors are the worst performers
			if tt.expectedDemoted > 0 {
				demotedIDs := make(map[uint32]bool)
				for _, change := range changes {
					if change.fromStatus == ntpdb.ServerScoresStatusTesting && change.toStatus == ntpdb.ServerScoresStatusCandidate {
						demotedIDs[change.monitorID] = true
					}
				}

				// Worst performers should be the ones with highest monitor IDs (added last with highest priority)
				expectedWorstIDs := make(map[uint32]bool)
				for i := 0; i < tt.expectedDemoted; i++ {
					// Worst performers are the last testing monitors added
					monitorID := uint32(tt.activeCount + tt.testingCount - i)
					expectedWorstIDs[monitorID] = true
				}

				for expectedID := range expectedWorstIDs {
					if !demotedIDs[expectedID] {
						t.Errorf("expected worst performer %d to be demoted, but it wasn't", expectedID)
					}
				}
			}

			t.Logf("Test %s: active=%d, testing=%d, target=%d, demoted=%d",
				tt.name, tt.activeCount, tt.testingCount, tt.expectedTarget, demotions)
		})
	}
}

func TestDynamicTestingPoolWithConstraints(t *testing.T) {
	// Test that monitors with constraint violations are not considered for count-based demotion
	sl := &Selector{
		log: slog.New(slog.NewTextHandler(os.Stderr, &slog.HandlerOptions{Level: slog.LevelWarn})),
	}
	ctx := context.Background()

	monitors := []evaluatedMonitor{
		// 2 active monitors
		{
			monitor: monitorCandidate{
				ID:           1,
				ServerStatus: ntpdb.ServerScoresStatusActive,
				GlobalStatus: ntpdb.MonitorsStatusActive,
				IsHealthy:    true,
				HasMetrics:   true,
			},
			currentViolation: &constraintViolation{Type: violationNone},
			recommendedState: candidateIn,
		},
		{
			monitor: monitorCandidate{
				ID:           2,
				ServerStatus: ntpdb.ServerScoresStatusActive,
				GlobalStatus: ntpdb.MonitorsStatusActive,
				IsHealthy:    true,
				HasMetrics:   true,
			},
			currentViolation: &constraintViolation{Type: violationNone},
			recommendedState: candidateIn,
		},
		// 3 testing monitors: 1 with violation, 2 healthy
		{
			monitor: monitorCandidate{
				ID:           3,
				ServerStatus: ntpdb.ServerScoresStatusTesting,
				GlobalStatus: ntpdb.MonitorsStatusActive,
				RTT:          10,
			},
			currentViolation: &constraintViolation{Type: violationLimit}, // has violation
			recommendedState: candidateOut,                               // already marked for demotion
		},
		{
			monitor: monitorCandidate{
				ID:           4,
				ServerStatus: ntpdb.ServerScoresStatusTesting,
				GlobalStatus: ntpdb.MonitorsStatusActive,
				IsHealthy:    true,
				HasMetrics:   true,
				RTT:          20,
			},
			currentViolation: &constraintViolation{Type: violationNone},
			recommendedState: candidateIn,
		},
		{
			monitor: monitorCandidate{
				ID:           5,
				ServerStatus: ntpdb.ServerScoresStatusTesting,
				GlobalStatus: ntpdb.MonitorsStatusActive,
				IsHealthy:    true,
				HasMetrics:   true,
				RTT:          30, // worst performer
			},
			currentViolation: &constraintViolation{Type: violationNone},
			recommendedState: candidateIn,
		},
	}

	server := &serverInfo{ID: 1}
	accountLimits := make(map[uint32]*accountLimit)

	// With 2 active (need 5 more), dynamic testing target = 5 + 5 = 10
	// Current testing = 3, so no demotions expected
	// But monitor 3 should be demoted due to violation (handled by constraint logic)

	changes := sl.applySelectionRules(ctx, monitors, server, accountLimits, []ntpdb.GetMonitorPriorityRow{})

	// Count different types of demotions
	constraintDemotions := 0
	countDemotions := 0

	for _, change := range changes {
		if change.fromStatus == ntpdb.ServerScoresStatusTesting && change.toStatus == ntpdb.ServerScoresStatusCandidate {
			if change.reason == "gradual removal (health or constraints)" {
				constraintDemotions++
			} else {
				countDemotions++
			}
		}
	}

	// Should have 1 constraint-based demotion (monitor 3), 0 count-based demotions
	if constraintDemotions != 1 {
		t.Errorf("expected 1 constraint demotion, got %d", constraintDemotions)
	}
	if countDemotions != 0 {
		t.Errorf("expected 0 count-based demotions, got %d", countDemotions)
	}
}
