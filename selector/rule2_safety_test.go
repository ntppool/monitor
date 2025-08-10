package selector

import (
	"context"
	"log/slog"
	"testing"

	"go.ntppool.org/monitor/ntpdb"
)

// TestRule2SafetyThreshold tests the safety check that prevents removing monitors
// when we're below the safe threshold (target - 2)
func TestRule2SafetyThreshold(t *testing.T) {
	ctx := context.Background()
	sl := &Selector{
		log: slog.Default(),
	}

	tests := []struct {
		name                     string
		currentActiveMonitors    int
		currentTestingMonitors   int
		targetNumber             int
		unhealthyActiveMonitors  int
		unhealthyTestingMonitors int
		expectedActiveRemovals   int
		expectedTestingRemovals  int
		expectActiveWarning      bool
		expectTestingWarning     bool
	}{
		{
			name:                     "below active threshold - no removals",
			currentActiveMonitors:    5, // target - 2
			currentTestingMonitors:   5,
			targetNumber:             targetActiveMonitors, // 7
			unhealthyActiveMonitors:  5,                    // all unhealthy
			unhealthyTestingMonitors: 2,
			expectedActiveRemovals:   0, // safety check prevents removal
			expectedTestingRemovals:  2, // testing can still be removed
			expectActiveWarning:      true,
			expectTestingWarning:     false,
		},
		{
			name:                     "at active threshold - no removals",
			currentActiveMonitors:    5, // target - 2
			currentTestingMonitors:   5,
			targetNumber:             targetActiveMonitors,
			unhealthyActiveMonitors:  3,
			unhealthyTestingMonitors: 1,
			expectedActiveRemovals:   0, // at threshold, no removals
			expectedTestingRemovals:  1,
			expectActiveWarning:      true,
			expectTestingWarning:     false,
		},
		{
			name:                     "above active threshold - limited removals",
			currentActiveMonitors:    6, // target - 1
			currentTestingMonitors:   5,
			targetNumber:             targetActiveMonitors,
			unhealthyActiveMonitors:  4,
			unhealthyTestingMonitors: 2,
			expectedActiveRemovals:   1, // can remove 1 to reach threshold
			expectedTestingRemovals:  2,
			expectActiveWarning:      false,
			expectTestingWarning:     false,
		},
		{
			name:                     "well above threshold - normal removals",
			currentActiveMonitors:    10,
			currentTestingMonitors:   8,
			targetNumber:             targetActiveMonitors,
			unhealthyActiveMonitors:  5,
			unhealthyTestingMonitors: 3,
			expectedActiveRemovals:   2, // respects normal limit
			expectedTestingRemovals:  2, // respects normal limit
			expectActiveWarning:      false,
			expectTestingWarning:     false,
		},
		{
			name:                     "testing threshold with active demotions",
			currentActiveMonitors:    7,
			currentTestingMonitors:   3, // baseTestingTarget - 2
			targetNumber:             targetActiveMonitors,
			unhealthyActiveMonitors:  2,
			unhealthyTestingMonitors: 3, // all unhealthy
			expectedActiveRemovals:   2, // can still remove active (they become testing)
			expectedTestingRemovals:  2, // after active demotion: 3 + 2 = 5, threshold is 3, so can remove 2
			expectActiveWarning:      false,
			expectTestingWarning:     false,
		},
		{
			name:                     "zero active with testing above threshold",
			currentActiveMonitors:    0,
			currentTestingMonitors:   6,
			targetNumber:             targetActiveMonitors,
			unhealthyActiveMonitors:  0,
			unhealthyTestingMonitors: 4,
			expectedActiveRemovals:   0,
			expectedTestingRemovals:  2, // can remove down to threshold (6 - 2 = 4, but limit is 2)
			expectActiveWarning:      true,
			expectTestingWarning:     false,
		},
		{
			name:                     "very low counts - minimum protection",
			currentActiveMonitors:    2,
			currentTestingMonitors:   2,
			targetNumber:             targetActiveMonitors,
			unhealthyActiveMonitors:  2,
			unhealthyTestingMonitors: 2,
			expectedActiveRemovals:   0, // below threshold
			expectedTestingRemovals:  0, // below threshold
			expectActiveWarning:      true,
			expectTestingWarning:     true,
		},
		{
			name:                     "testing at threshold no active demotions",
			currentActiveMonitors:    6, // above threshold, can be removed
			currentTestingMonitors:   3, // at threshold
			targetNumber:             targetActiveMonitors,
			unhealthyActiveMonitors:  0, // no unhealthy active
			unhealthyTestingMonitors: 3, // all testing unhealthy
			expectedActiveRemovals:   0, // none unhealthy
			expectedTestingRemovals:  0, // at threshold, safety prevents removal
			expectActiveWarning:      false,
			expectTestingWarning:     true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Create active monitors
			activeMonitors := make([]evaluatedMonitor, tt.currentActiveMonitors)
			for i := 0; i < tt.currentActiveMonitors; i++ {
				state := candidateIn
				if i < tt.unhealthyActiveMonitors {
					state = candidateOut
				}
				activeMonitors[i] = evaluatedMonitor{
					monitor: monitorCandidate{
						ID:           uint32(i + 1),
						ServerStatus: ntpdb.ServerScoresStatusActive,
					},
					recommendedState: state,
				}
			}

			// Create testing monitors
			testingMonitors := make([]evaluatedMonitor, tt.currentTestingMonitors)
			for i := 0; i < tt.currentTestingMonitors; i++ {
				state := candidateIn
				if i < tt.unhealthyTestingMonitors {
					state = candidateOut
				}
				testingMonitors[i] = evaluatedMonitor{
					monitor: monitorCandidate{
						ID:           uint32(i + 100),
						ServerStatus: ntpdb.ServerScoresStatusTesting,
					},
					recommendedState: state,
				}
			}

			// Create selection context
			selCtx := selectionContext{
				targetNumber: tt.targetNumber,
				limits: changeLimits{
					activeRemovals:  2, // default limit
					testingRemovals: 2, // default limit
				},
			}

			// Apply Rule 2
			changes := sl.applyRule2GradualConstraintRemoval(
				ctx,
				selCtx,
				activeMonitors,
				testingMonitors,
				tt.currentActiveMonitors,
			)

			// Count actual removals
			activeRemovals := 0
			testingRemovals := 0
			for _, change := range changes {
				if change.fromStatus == ntpdb.ServerScoresStatusActive {
					activeRemovals++
				} else if change.fromStatus == ntpdb.ServerScoresStatusTesting {
					testingRemovals++
				}
			}

			// Verify results
			if activeRemovals != tt.expectedActiveRemovals {
				t.Errorf("expected %d active removals, got %d", tt.expectedActiveRemovals, activeRemovals)
			}
			if testingRemovals != tt.expectedTestingRemovals {
				t.Errorf("expected %d testing removals, got %d", tt.expectedTestingRemovals, testingRemovals)
			}

			// Log the changes for debugging
			t.Logf("Changes made: %d active removals, %d testing removals", activeRemovals, testingRemovals)
			for _, change := range changes {
				t.Logf("  Monitor %d: %s -> %s (%s)", change.monitorID, change.fromStatus, change.toStatus, change.reason)
			}
		})
	}
}

// TestRule2SafetyWithEmergency tests that safety checks work correctly in emergency situations
func TestRule2SafetyWithEmergency(t *testing.T) {
	ctx := context.Background()
	sl := &Selector{
		log: slog.Default(),
	}

	// Scenario: All monitors are unhealthy, but we're at the safety threshold
	// We should not remove any monitors to avoid leaving servers completely unmonitored
	activeMonitors := make([]evaluatedMonitor, 5) // target - 2
	for i := 0; i < 5; i++ {
		activeMonitors[i] = evaluatedMonitor{
			monitor: monitorCandidate{
				ID:           uint32(i + 1),
				ServerStatus: ntpdb.ServerScoresStatusActive,
				IsHealthy:    false,
			},
			recommendedState: candidateOut, // all marked for removal
		}
	}

	testingMonitors := make([]evaluatedMonitor, 3) // baseTestingTarget - 2
	for i := 0; i < 3; i++ {
		testingMonitors[i] = evaluatedMonitor{
			monitor: monitorCandidate{
				ID:           uint32(i + 100),
				ServerStatus: ntpdb.ServerScoresStatusTesting,
				IsHealthy:    false,
			},
			recommendedState: candidateOut, // all marked for removal
		}
	}

	selCtx := selectionContext{
		targetNumber: targetActiveMonitors,
		limits: changeLimits{
			activeRemovals:  5, // high limit to test safety
			testingRemovals: 5, // high limit to test safety
		},
	}

	changes := sl.applyRule2GradualConstraintRemoval(
		ctx,
		selCtx,
		activeMonitors,
		testingMonitors,
		5, // current active count
	)

	// Should have no changes due to safety threshold
	if len(changes) != 0 {
		t.Errorf("expected no changes due to safety threshold, got %d changes", len(changes))
		for _, change := range changes {
			t.Logf("Unexpected change: Monitor %d: %s -> %s", change.monitorID, change.fromStatus, change.toStatus)
		}
	}
}

// TestRule2MixedConstraintAndPerformance tests that performance removals are blocked correctly
// when constraint removals would bring us below threshold
func TestRule2MixedConstraintAndPerformance(t *testing.T) {
	ctx := context.Background()
	sl := &Selector{
		log: slog.Default(),
	}

	// Scenario: 7 active monitors (at target), 2 with constraints, 2 with performance issues
	// After removing 2 constraint violations, we'd be at threshold (5)
	// So performance removals should be blocked
	activeMonitors := make([]evaluatedMonitor, 7)
	for i := 0; i < 7; i++ {
		violation := &constraintViolation{Type: violationNone}
		state := candidateIn
		if i < 2 { // First 2 have constraint violations
			violation = &constraintViolation{Type: violationLimit}
			state = candidateOut
		} else if i < 4 { // Next 2 have performance issues
			state = candidateOut
		}
		activeMonitors[i] = evaluatedMonitor{
			monitor: monitorCandidate{
				ID:           uint32(i + 1),
				ServerStatus: ntpdb.ServerScoresStatusActive,
				IsHealthy:    i >= 4, // Last 3 are healthy
			},
			recommendedState: state,
			currentViolation: violation,
		}
	}

	testingMonitors := make([]evaluatedMonitor, 5)
	for i := 0; i < 5; i++ {
		testingMonitors[i] = evaluatedMonitor{
			monitor: monitorCandidate{
				ID:           uint32(i + 100),
				ServerStatus: ntpdb.ServerScoresStatusTesting,
				IsHealthy:    true,
			},
			recommendedState: candidateIn,
			currentViolation: &constraintViolation{Type: violationNone},
		}
	}

	selCtx := selectionContext{
		targetNumber: targetActiveMonitors,
		limits: changeLimits{
			activeRemovals:  4, // High limit to test safety logic
			testingRemovals: 2,
		},
	}

	changes := sl.applyRule2GradualConstraintRemoval(
		ctx,
		selCtx,
		activeMonitors,
		testingMonitors,
		7, // current active count
	)

	// Should only remove the 2 constraint violations, not the performance issues
	// because after removing constraints we'd be at threshold
	activeRemovals := 0
	constraintRemovals := 0
	for _, change := range changes {
		if change.fromStatus == ntpdb.ServerScoresStatusActive {
			activeRemovals++
			// Check if it's a constraint removal (monitors 1 and 2)
			if change.monitorID <= 2 {
				constraintRemovals++
			}
		}
	}

	if activeRemovals != 2 {
		t.Errorf("expected 2 active removals (constraints only), got %d", activeRemovals)
	}
	if constraintRemovals != 2 {
		t.Errorf("expected 2 constraint removals, got %d", constraintRemovals)
	}

	// Verify no performance-based removals (monitors 3 and 4)
	for _, change := range changes {
		if change.monitorID == 3 || change.monitorID == 4 {
			t.Errorf("incorrectly removed performance-based monitor %d when at safety threshold", change.monitorID)
		}
	}
}

// TestRule2ConstraintViolationsAlwaysRemoved tests that constraint violations are removed even at safety threshold
func TestRule2ConstraintViolationsAlwaysRemoved(t *testing.T) {
	ctx := context.Background()
	sl := &Selector{
		log: slog.Default(),
	}

	// Scenario: At safety threshold but monitors have constraint violations
	// Constraint violations should still be removed
	activeMonitors := make([]evaluatedMonitor, 5) // at threshold (target - 2)
	for i := 0; i < 5; i++ {
		violation := &constraintViolation{Type: violationNone}
		state := candidateIn
		if i < 2 { // 2 with constraint violations
			violation = &constraintViolation{Type: violationLimit}
			state = candidateOut
		}
		activeMonitors[i] = evaluatedMonitor{
			monitor: monitorCandidate{
				ID:           uint32(i + 1),
				ServerStatus: ntpdb.ServerScoresStatusActive,
			},
			recommendedState: state,
			currentViolation: violation,
		}
	}

	testingMonitors := make([]evaluatedMonitor, 3) // at threshold
	for i := 0; i < 3; i++ {
		violation := &constraintViolation{Type: violationNone}
		state := candidateIn
		if i < 1 { // 1 with constraint violation
			violation = &constraintViolation{Type: violationNetworkSameSubnet}
			state = candidateOut
		}
		testingMonitors[i] = evaluatedMonitor{
			monitor: monitorCandidate{
				ID:           uint32(i + 100),
				ServerStatus: ntpdb.ServerScoresStatusTesting,
			},
			recommendedState: state,
			currentViolation: violation,
		}
	}

	selCtx := selectionContext{
		targetNumber: targetActiveMonitors,
		limits: changeLimits{
			activeRemovals:  2,
			testingRemovals: 2,
		},
	}

	changes := sl.applyRule2GradualConstraintRemoval(
		ctx,
		selCtx,
		activeMonitors,
		testingMonitors,
		5, // current active count
	)

	// Should remove the constraint violations despite being at threshold
	activeRemovals := 0
	testingRemovals := 0
	for _, change := range changes {
		if change.fromStatus == ntpdb.ServerScoresStatusActive {
			activeRemovals++
		} else if change.fromStatus == ntpdb.ServerScoresStatusTesting {
			testingRemovals++
		}
	}

	if activeRemovals != 2 {
		t.Errorf("expected 2 active removals for constraint violations at threshold, got %d", activeRemovals)
	}
	if testingRemovals != 1 {
		t.Errorf("expected 1 testing removal for constraint violation at threshold, got %d", testingRemovals)
	}

	// Verify that the removed monitors are the ones with violations
	for _, change := range changes {
		if change.fromStatus == ntpdb.ServerScoresStatusActive {
			if change.monitorID > 2 {
				t.Errorf("removed wrong active monitor: %d (expected 1 or 2)", change.monitorID)
			}
		} else if change.fromStatus == ntpdb.ServerScoresStatusTesting {
			if change.monitorID != 100 {
				t.Errorf("removed wrong testing monitor: %d (expected 100)", change.monitorID)
			}
		}
	}
}

// TestRule2NormalOperationNotAffected tests that normal operation is not affected by safety checks
func TestRule2NormalOperationNotAffected(t *testing.T) {
	ctx := context.Background()
	sl := &Selector{
		log: slog.Default(),
	}

	// Scenario: Plenty of monitors, some unhealthy
	// Normal removal should proceed as before
	activeMonitors := make([]evaluatedMonitor, 10)
	for i := 0; i < 10; i++ {
		state := candidateIn
		if i < 3 { // 3 unhealthy
			state = candidateOut
		}
		activeMonitors[i] = evaluatedMonitor{
			monitor: monitorCandidate{
				ID:           uint32(i + 1),
				ServerStatus: ntpdb.ServerScoresStatusActive,
			},
			recommendedState: state,
		}
	}

	testingMonitors := make([]evaluatedMonitor, 8)
	for i := 0; i < 8; i++ {
		state := candidateIn
		if i < 2 { // 2 unhealthy
			state = candidateOut
		}
		testingMonitors[i] = evaluatedMonitor{
			monitor: monitorCandidate{
				ID:           uint32(i + 100),
				ServerStatus: ntpdb.ServerScoresStatusTesting,
			},
			recommendedState: state,
		}
	}

	selCtx := selectionContext{
		targetNumber: targetActiveMonitors,
		limits: changeLimits{
			activeRemovals:  2,
			testingRemovals: 2,
		},
	}

	changes := sl.applyRule2GradualConstraintRemoval(
		ctx,
		selCtx,
		activeMonitors,
		testingMonitors,
		10, // current active count
	)

	// Should remove up to the limit (2 active, 2 testing)
	activeRemovals := 0
	testingRemovals := 0
	for _, change := range changes {
		if change.fromStatus == ntpdb.ServerScoresStatusActive {
			activeRemovals++
		} else if change.fromStatus == ntpdb.ServerScoresStatusTesting {
			testingRemovals++
		}
	}

	if activeRemovals != 2 {
		t.Errorf("expected 2 active removals in normal operation, got %d", activeRemovals)
	}
	if testingRemovals != 2 {
		t.Errorf("expected 2 testing removals in normal operation, got %d", testingRemovals)
	}
}
