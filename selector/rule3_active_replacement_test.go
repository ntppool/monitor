package selector

import (
	"context"
	"log/slog"
	"testing"

	"go.ntppool.org/monitor/ntpdb"
)

// TestRule3_ActiveTestingReplacement tests that Rule 3 now supports performance-based replacement
// This demonstrates the new functionality where better testing monitors can replace worse active monitors
func TestRule3_ActiveTestingReplacement(t *testing.T) {
	ctx := context.Background()
	s := &Selector{
		log: slog.Default(),
	}

	// Create scenario: 7 active monitors (at target), 5 testing monitors,
	// where some testing monitors significantly outperform active monitors
	var evaluatedMonitors []evaluatedMonitor

	// 7 active monitors with varying performance (some poor performers)
	activePriorities := []int{10, 15, 20, 25, 30, 100, 120} // Last two are poor performers
	for i := 0; i < 7; i++ {
		evaluatedMonitors = append(evaluatedMonitors, evaluatedMonitor{
			monitor: monitorCandidate{
				ID:           int64(i + 1),
				ServerStatus: ntpdb.ServerScoresStatusActive,
				GlobalStatus: ntpdb.MonitorsStatusActive,
				Priority:     activePriorities[i],
				IsHealthy:    true,
			},
			recommendedState: candidateIn,
			currentViolation: &constraintViolation{Type: violationNone},
		})
	}

	// 5 testing monitors with some excellent performers
	testingPriorities := []int{40, 45, 50, 55, 60} // First two are much better than worst active
	for i := 0; i < 5; i++ {
		evaluatedMonitors = append(evaluatedMonitors, evaluatedMonitor{
			monitor: monitorCandidate{
				ID:           int64(i + 10),
				ServerStatus: ntpdb.ServerScoresStatusTesting,
				GlobalStatus: ntpdb.MonitorsStatusActive,
				Priority:     testingPriorities[i],
				IsHealthy:    true,
				Count:        int64(minCountForActive), // Ensure testing monitors can be promoted to active
			},
			recommendedState: candidateIn,
			currentViolation: &constraintViolation{Type: violationNone},
		})
	}

	server := &serverInfo{ID: 1982}
	accountLimits := make(map[int64]*accountLimit)

	selCtx := selectionContext{
		evaluatedMonitors: evaluatedMonitors,
		server:            server,
		accountLimits:     accountLimits,
		assignedMonitors:  nil,
		limits:            changeLimits{promotions: 4, activeRemovals: 2, testingRemovals: 2},
		targetNumber:      7,
		emergencyOverride: false,
	}

	// Separate monitors by status
	var activeMonitors, testingMonitors []evaluatedMonitor
	for _, em := range evaluatedMonitors {
		if em.monitor.ServerStatus == ntpdb.ServerScoresStatusActive {
			activeMonitors = append(activeMonitors, em)
		} else if em.monitor.ServerStatus == ntpdb.ServerScoresStatusTesting {
			testingMonitors = append(testingMonitors, em)
		}
	}

	t.Logf("Scenario: %d active, %d testing", len(activeMonitors), len(testingMonitors))
	for i, em := range evaluatedMonitors {
		t.Logf("Monitor %d: ID=%d, Status=%s, Priority=%d",
			i, em.monitor.ID, em.monitor.ServerStatus, em.monitor.Priority)
	}

	// Call Rule 3 - this should now perform active-testing swaps in Phase 2
	result := s.applyRule3TestingToActivePromotion(
		ctx, selCtx, testingMonitors, activeMonitors, accountLimits, 7, 5,
	)

	t.Logf("Total changes: %d", len(result.changes))
	for i, change := range result.changes {
		t.Logf("Change %d: monitor %d from %s to %s, reason: %s",
			i, change.monitorID, change.fromStatus, change.toStatus, change.reason)
	}

	// Verify that we got some active-testing swaps
	swaps := 0
	promotions := 0
	demotions := 0
	for _, change := range result.changes {
		if change.reason == "active-testing swap (promote)" {
			promotions++
		} else if change.reason == "active-testing swap (demote)" {
			demotions++
		}
	}
	swaps = promotions // Each swap consists of one promotion and one demotion

	if swaps == 0 {
		t.Errorf("Expected some active-testing swaps, got 0")
		return
	}

	if promotions != demotions {
		t.Errorf("Expected equal promotions and demotions for swaps, got %d promotions, %d demotions",
			promotions, demotions)
	}

	t.Logf("Active-testing swaps: %d swaps (%d promotions, %d demotions)", swaps, promotions, demotions)
	t.Logf("Successfully demonstrated Rule 3 Phase 2 active-testing replacement functionality!")
}
