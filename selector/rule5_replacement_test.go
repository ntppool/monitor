package selector

import (
	"context"
	"log/slog"
	"testing"

	"go.ntppool.org/monitor/ntpdb"
)

// TestRule5_CandidateToTestingReplacement tests that Rule 5 replaces worse-performing
// testing monitors with better candidates when testing pool is at capacity
func TestRule5_CandidateToTestingReplacement(t *testing.T) {
	ctx := context.Background()
	s := &Selector{
		log: slog.Default(),
	}

	// Create scenario: 7 active monitors (at target), 5 testing monitors (at base target),
	// and candidates with better performance than some testing monitors
	var evaluatedMonitors []evaluatedMonitor

	// 7 active monitors (at target)
	for i := 0; i < 7; i++ {
		evaluatedMonitors = append(evaluatedMonitors, evaluatedMonitor{
			monitor: monitorCandidate{
				ID:           uint32(i + 1),
				ServerStatus: ntpdb.ServerScoresStatusActive,
				GlobalStatus: ntpdb.MonitorsStatusActive,
				RTT:          float64(10 + i*5), // RTT 10, 15, 20, 25, 30, 35, 40
				IsHealthy:    true,
			},
			recommendedState: candidateIn,
			currentViolation: &constraintViolation{Type: violationNone},
		})
	}

	// 5 testing monitors (at base target) with varying performance
	testingRTTs := []float64{50, 60, 70, 80, 90} // Worse performance than active monitors
	for i := 0; i < 5; i++ {
		evaluatedMonitors = append(evaluatedMonitors, evaluatedMonitor{
			monitor: monitorCandidate{
				ID:           uint32(i + 10),
				ServerStatus: ntpdb.ServerScoresStatusTesting,
				GlobalStatus: ntpdb.MonitorsStatusActive,
				RTT:          testingRTTs[i],
				IsHealthy:    true,
			},
			recommendedState: candidateIn,
			currentViolation: &constraintViolation{Type: violationNone},
		})
	}

	// 3 candidate monitors with better performance than some testing monitors
	candidateRTTs := []float64{45, 55, 75} // Better than testing monitors 10, 11, and 12
	for i := 0; i < 3; i++ {
		evaluatedMonitors = append(evaluatedMonitors, evaluatedMonitor{
			monitor: monitorCandidate{
				ID:           uint32(i + 20),
				ServerStatus: ntpdb.ServerScoresStatusCandidate,
				GlobalStatus: ntpdb.MonitorsStatusActive,
				RTT:          candidateRTTs[i],
				IsHealthy:    true,
			},
			recommendedState: candidateIn,
			currentViolation: &constraintViolation{Type: violationNone},
		})
	}

	server := &serverInfo{ID: 1982}
	accountLimits := make(map[uint32]*accountLimit)

	changes := s.applySelectionRules(ctx, evaluatedMonitors, server, accountLimits, nil)

	// Debug: Log scenario setup
	activeCount := 0
	testingCount := 0
	candidateCount := 0
	for _, em := range evaluatedMonitors {
		switch em.monitor.ServerStatus {
		case ntpdb.ServerScoresStatusActive:
			activeCount++
		case ntpdb.ServerScoresStatusTesting:
			testingCount++
		case ntpdb.ServerScoresStatusCandidate:
			candidateCount++
		}
	}
	t.Logf("Scenario: %d active, %d testing, %d candidates", activeCount, testingCount, candidateCount)

	// Debug: Check monitor health and recommended states
	for i, em := range evaluatedMonitors {
		t.Logf("Monitor %d: ID=%d, Status=%s, Healthy=%v, RecState=%s, RTT=%.1f",
			i, em.monitor.ID, em.monitor.ServerStatus, em.monitor.IsHealthy, em.recommendedState, em.monitor.RTT)
	}

	// Debug: Log all changes
	t.Logf("Total changes: %d", len(changes))
	for i, change := range changes {
		t.Logf("Change %d: monitor %d from %s to %s, reason: %s", i, change.monitorID, change.fromStatus, change.toStatus, change.reason)
	}

	// Should have replacements: better candidates promoted, worse testing monitors demoted
	promotions := 0
	demotions := 0
	replacementPromotions := 0
	replacementDemotions := 0

	for _, change := range changes {
		if change.fromStatus == ntpdb.ServerScoresStatusCandidate && change.toStatus == ntpdb.ServerScoresStatusTesting {
			promotions++
			if change.reason == "replacement promotion" {
				replacementPromotions++
			}
		}
		if change.fromStatus == ntpdb.ServerScoresStatusTesting && change.toStatus == ntpdb.ServerScoresStatusCandidate {
			demotions++
			if change.reason == "replaced by better candidate" {
				replacementDemotions++
			}
		}
	}

	// Should have equal number of replacement promotions and demotions
	if replacementPromotions != replacementDemotions {
		t.Errorf("Expected equal replacement promotions and demotions, got %d promotions and %d demotions",
			replacementPromotions, replacementDemotions)
	}

	// Should have at least some replacements (candidates are better than worst testing monitors)
	if replacementPromotions == 0 {
		t.Errorf("Expected some replacement promotions, got 0")
	}

	t.Logf("Replacement operations: %d promotions, %d demotions", replacementPromotions, replacementDemotions)
}

// TestRule5_NoReplacementWhenTestingBetter tests that Rule 5 doesn't replace
// testing monitors when all candidates are worse
func TestRule5_NoReplacementWhenTestingBetter(t *testing.T) {
	ctx := context.Background()
	s := &Selector{
		log: slog.Default(),
	}

	// Create scenario: testing monitors are all better than candidates
	var evaluatedMonitors []evaluatedMonitor

	// 7 active monitors (at target)
	for i := 0; i < 7; i++ {
		evaluatedMonitors = append(evaluatedMonitors, evaluatedMonitor{
			monitor: monitorCandidate{
				ID:           uint32(i + 1),
				ServerStatus: ntpdb.ServerScoresStatusActive,
				GlobalStatus: ntpdb.MonitorsStatusActive,
				RTT:          float64(10 + i*5),
				IsHealthy:    true,
			},
			recommendedState: candidateIn,
			currentViolation: &constraintViolation{Type: violationNone},
		})
	}

	// 5 testing monitors with good performance
	for i := 0; i < 5; i++ {
		evaluatedMonitors = append(evaluatedMonitors, evaluatedMonitor{
			monitor: monitorCandidate{
				ID:           uint32(i + 10),
				ServerStatus: ntpdb.ServerScoresStatusTesting,
				GlobalStatus: ntpdb.MonitorsStatusActive,
				RTT:          float64(50 + i*5), // RTT 50, 55, 60, 65, 70
				IsHealthy:    true,
			},
			recommendedState: candidateIn,
			currentViolation: &constraintViolation{Type: violationNone},
		})
	}

	// 3 candidate monitors with worse performance than testing monitors
	for i := 0; i < 3; i++ {
		evaluatedMonitors = append(evaluatedMonitors, evaluatedMonitor{
			monitor: monitorCandidate{
				ID:           uint32(i + 20),
				ServerStatus: ntpdb.ServerScoresStatusCandidate,
				GlobalStatus: ntpdb.MonitorsStatusActive,
				RTT:          float64(80 + i*10), // RTT 80, 90, 100 - worse than all testing
				IsHealthy:    true,
			},
			recommendedState: candidateIn,
			currentViolation: &constraintViolation{Type: violationNone},
		})
	}

	server := &serverInfo{ID: 1982}
	accountLimits := make(map[uint32]*accountLimit)

	changes := s.applySelectionRules(ctx, evaluatedMonitors, server, accountLimits, nil)

	// Should have no replacements since candidates are worse than testing monitors
	replacementPromotions := 0
	replacementDemotions := 0

	for _, change := range changes {
		if change.reason == "replacement promotion" {
			replacementPromotions++
		}
		if change.reason == "replaced by better candidate" {
			replacementDemotions++
		}
	}

	if replacementPromotions > 0 || replacementDemotions > 0 {
		t.Errorf("Expected no replacements when candidates are worse, got %d promotions and %d demotions",
			replacementPromotions, replacementDemotions)
	}

	t.Logf("No replacements occurred as expected (candidates were worse than testing monitors)")
}

// TestRule5_CapacityPromotionStillWorks tests that Rule 5 still does capacity-based
// promotion when testing pool is under target and Rule 3 doesn't consume all budget
func TestRule5_CapacityPromotionStillWorks(t *testing.T) {
	ctx := context.Background()
	s := &Selector{
		log: slog.Default(),
	}

	// Create scenario: testing pool is under capacity but no active gap
	// so Rule 3 won't consume budget
	var evaluatedMonitors []evaluatedMonitor

	// 7 active monitors (at target) - no gap for Rule 3 to consume budget
	for i := 0; i < 7; i++ {
		evaluatedMonitors = append(evaluatedMonitors, evaluatedMonitor{
			monitor: monitorCandidate{
				ID:           uint32(i + 1),
				ServerStatus: ntpdb.ServerScoresStatusActive,
				GlobalStatus: ntpdb.MonitorsStatusActive,
				RTT:          float64(10 + i*5),
				IsHealthy:    true,
			},
			recommendedState: candidateIn,
			currentViolation: &constraintViolation{Type: violationNone},
		})
	}

	// 2 testing monitors (under capacity - should allow more)
	for i := 0; i < 2; i++ {
		evaluatedMonitors = append(evaluatedMonitors, evaluatedMonitor{
			monitor: monitorCandidate{
				ID:           uint32(i + 10),
				ServerStatus: ntpdb.ServerScoresStatusTesting,
				GlobalStatus: ntpdb.MonitorsStatusActive,
				RTT:          float64(50 + i*5),
				IsHealthy:    true,
			},
			recommendedState: candidateIn,
			currentViolation: &constraintViolation{Type: violationNone},
		})
	}

	// 3 candidate monitors
	for i := 0; i < 3; i++ {
		evaluatedMonitors = append(evaluatedMonitors, evaluatedMonitor{
			monitor: monitorCandidate{
				ID:           uint32(i + 20),
				ServerStatus: ntpdb.ServerScoresStatusCandidate,
				GlobalStatus: ntpdb.MonitorsStatusActive,
				RTT:          float64(60 + i*5),
				IsHealthy:    true,
			},
			recommendedState: candidateIn,
			currentViolation: &constraintViolation{Type: violationNone},
		})
	}

	server := &serverInfo{ID: 1982}
	accountLimits := make(map[uint32]*accountLimit)

	changes := s.applySelectionRules(ctx, evaluatedMonitors, server, accountLimits, nil)

	// Should have capacity-based promotions (testing pool has room)
	capacityPromotions := 0
	for _, change := range changes {
		if change.fromStatus == ntpdb.ServerScoresStatusCandidate &&
			change.toStatus == ntpdb.ServerScoresStatusTesting &&
			change.reason == "candidate to testing" {
			capacityPromotions++
		}
	}

	// Should promote some candidates due to available capacity
	// dynamicTestingTarget = baseTestingTarget(5) + activeGap(0) = 5
	// current testing = 2, so capacity = 3
	// But limited by change limits (typically 2) and hard limit of 2 in Rule 5
	if capacityPromotions == 0 {
		t.Errorf("Expected some capacity-based promotions, got 0")
	}

	t.Logf("Capacity-based promotions: %d", capacityPromotions)
}

// TestRule5_ConstraintRespectedInReplacements tests that replacement logic
// respects constraint checking (e.g., account limits)
func TestRule5_ConstraintRespectedInReplacements(t *testing.T) {
	ctx := context.Background()
	s := &Selector{
		log: slog.Default(),
	}

	// Create scenario where replacement would violate account constraints
	var evaluatedMonitors []evaluatedMonitor

	// 7 active monitors (at target)
	for i := 0; i < 7; i++ {
		evaluatedMonitors = append(evaluatedMonitors, evaluatedMonitor{
			monitor: monitorCandidate{
				ID:           uint32(i + 1),
				ServerStatus: ntpdb.ServerScoresStatusActive,
				GlobalStatus: ntpdb.MonitorsStatusActive,
				RTT:          float64(10 + i*5),
				IsHealthy:    true,
			},
			recommendedState: candidateIn,
			currentViolation: &constraintViolation{Type: violationNone},
		})
	}

	// 5 testing monitors
	for i := 0; i < 5; i++ {
		accountID := uint32(100) // Same account
		evaluatedMonitors = append(evaluatedMonitors, evaluatedMonitor{
			monitor: monitorCandidate{
				ID:           uint32(i + 10),
				AccountID:    &accountID,
				ServerStatus: ntpdb.ServerScoresStatusTesting,
				GlobalStatus: ntpdb.MonitorsStatusActive,
				RTT:          float64(50 + i*10),
				IsHealthy:    true,
			},
			recommendedState: candidateIn,
			currentViolation: &constraintViolation{Type: violationNone},
		})
	}

	// 1 candidate from same account (should be blocked by account limits)
	accountID := uint32(100)
	evaluatedMonitors = append(evaluatedMonitors, evaluatedMonitor{
		monitor: monitorCandidate{
			ID:           20,
			AccountID:    &accountID,
			ServerStatus: ntpdb.ServerScoresStatusCandidate,
			GlobalStatus: ntpdb.MonitorsStatusActive,
			RTT:          30, // Better than testing monitors but same account
			IsHealthy:    true,
		},
		recommendedState: candidateIn,
		currentViolation: &constraintViolation{Type: violationNone},
	})

	server := &serverInfo{ID: 1982}
	// Set up account limits that would prevent additional monitors from account 100
	accountLimits := map[uint32]*accountLimit{
		100: {
			AccountID:    100,
			MaxPerServer: 2, // Already has 5 testing monitors - should block promotion
			ActiveCount:  0,
			TestingCount: 5,
		},
	}

	changes := s.applySelectionRules(ctx, evaluatedMonitors, server, accountLimits, nil)

	// Should have no replacements due to account constraint violation
	replacementPromotions := 0
	for _, change := range changes {
		if change.reason == "replacement promotion" && change.monitorID == 20 {
			replacementPromotions++
		}
	}

	if replacementPromotions > 0 {
		t.Errorf("Expected no replacement promotions due to account limits, got %d", replacementPromotions)
	}

	t.Logf("Replacement correctly blocked by account constraints")
}

// TestCandidateOutperformsTestingMonitor tests the performance comparison function
func TestCandidateOutperformsTestingMonitor(t *testing.T) {
	s := &Selector{
		log: slog.Default(),
	}

	tests := []struct {
		name             string
		candidateHealthy bool
		candidateRTT     float64
		testingHealthy   bool
		testingRTT       float64
		expectedResult   bool
	}{
		{
			name:             "healthy candidate vs unhealthy testing",
			candidateHealthy: true,
			candidateRTT:     100,
			testingHealthy:   false,
			testingRTT:       50,
			expectedResult:   true, // Health trumps RTT
		},
		{
			name:             "unhealthy candidate vs healthy testing",
			candidateHealthy: false,
			candidateRTT:     50,
			testingHealthy:   true,
			testingRTT:       100,
			expectedResult:   false, // Health trumps RTT
		},
		{
			name:             "both healthy, candidate better RTT",
			candidateHealthy: true,
			candidateRTT:     50,
			testingHealthy:   true,
			testingRTT:       100,
			expectedResult:   true, // Lower RTT is better
		},
		{
			name:             "both healthy, testing better RTT",
			candidateHealthy: true,
			candidateRTT:     100,
			testingHealthy:   true,
			testingRTT:       50,
			expectedResult:   false, // Higher RTT is worse
		},
		{
			name:             "both healthy, equal RTT",
			candidateHealthy: true,
			candidateRTT:     50,
			testingHealthy:   true,
			testingRTT:       50,
			expectedResult:   false, // Equal performance, no replacement
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			candidate := evaluatedMonitor{
				monitor: monitorCandidate{
					IsHealthy: tc.candidateHealthy,
					RTT:       tc.candidateRTT,
				},
			}
			testing := evaluatedMonitor{
				monitor: monitorCandidate{
					IsHealthy: tc.testingHealthy,
					RTT:       tc.testingRTT,
				},
			}

			result := s.candidateOutperformsTestingMonitor(candidate, testing)
			if result != tc.expectedResult {
				t.Errorf("Expected %v, got %v", tc.expectedResult, result)
			}
		})
	}
}
