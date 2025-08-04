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
				Priority:     int(10 + i*5),     // Priority matches RTT for consistency
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
				Priority:     int(testingRTTs[i]), // Priority matches RTT for consistency
				IsHealthy:    true,
			},
			recommendedState: candidateIn,
			currentViolation: &constraintViolation{Type: violationNone},
		})
	}

	// 3 candidate monitors with significantly better performance than some testing monitors
	candidateRTTs := []float64{45, 55, 75}
	candidatePriorities := []int{25, 30, 35} // Significantly better than testing priorities 50, 60, 70
	for i := 0; i < 3; i++ {
		evaluatedMonitors = append(evaluatedMonitors, evaluatedMonitor{
			monitor: monitorCandidate{
				ID:           uint32(i + 20),
				ServerStatus: ntpdb.ServerScoresStatusCandidate,
				GlobalStatus: ntpdb.MonitorsStatusActive,
				RTT:          candidateRTTs[i],
				Priority:     candidatePriorities[i], // 50%+ better than corresponding testing monitors
				IsHealthy:    true,
				Count:        int64(minCountForTesting), // Ensure candidates meet minimum data points requirement
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
				Count:        int64(minCountForTesting), // Ensure candidates meet minimum data points requirement
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
			Priority:     30, // Priority matches RTT
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
		name              string
		candidateHealthy  bool
		candidatePriority int
		testingHealthy    bool
		testingPriority   int
		expectedResult    bool
	}{
		{
			name:              "healthy candidate vs unhealthy testing",
			candidateHealthy:  true,
			candidatePriority: 100,
			testingHealthy:    false,
			testingPriority:   50,
			expectedResult:    true, // Health trumps priority
		},
		{
			name:              "unhealthy candidate vs healthy testing",
			candidateHealthy:  false,
			candidatePriority: 50,
			testingHealthy:    true,
			testingPriority:   100,
			expectedResult:    false, // Health trumps priority
		},
		{
			name:              "both healthy, significant improvement",
			candidateHealthy:  true,
			candidatePriority: 50,
			testingHealthy:    true,
			testingPriority:   100,
			expectedResult:    true, // 50% improvement and 50 point difference
		},
		{
			name:              "both healthy, below percentage threshold",
			candidateHealthy:  true,
			candidatePriority: 96,
			testingHealthy:    true,
			testingPriority:   100,
			expectedResult:    false, // Only 4% improvement
		},
		{
			name:              "both healthy, below point threshold",
			candidateHealthy:  true,
			candidatePriority: 95,
			testingHealthy:    true,
			testingPriority:   99,
			expectedResult:    false, // Only 4 point difference
		},
		{
			name:              "both healthy, equal priority",
			candidateHealthy:  true,
			candidatePriority: 50,
			testingHealthy:    true,
			testingPriority:   50,
			expectedResult:    false, // Equal performance, no replacement
		},
		{
			name:              "both healthy, candidate worse",
			candidateHealthy:  true,
			candidatePriority: 100,
			testingHealthy:    true,
			testingPriority:   50,
			expectedResult:    false, // Candidate is worse
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			candidate := evaluatedMonitor{
				monitor: monitorCandidate{
					IsHealthy: tc.candidateHealthy,
					Priority:  tc.candidatePriority,
				},
			}
			testing := evaluatedMonitor{
				monitor: monitorCandidate{
					IsHealthy: tc.testingHealthy,
					Priority:  tc.testingPriority,
				},
			}

			result := s.candidateOutperformsTestingMonitor(context.Background(), candidate, testing)
			if result != tc.expectedResult {
				t.Errorf("Expected %v, got %v", tc.expectedResult, result)
			}
		})
	}
}

// TestRule5_RealWorldAccount351Scenario tests the exact scenario from production data
// where high-performing candidates (monitor 106 priority 1, monitor 160 priority 11)
// from account 351 aren't being promoted despite outperforming testing monitors
func TestRule5_RealWorldAccount351Scenario(t *testing.T) {
	ctx := context.Background()
	s := &Selector{
		log: slog.Default(),
	}

	// Recreate the scenario from the SQL query data for server 2200
	var evaluatedMonitors []evaluatedMonitor

	// Account 351 (monitors_per_server_limit: 2) - the problematic account
	account351 := uint32(351)

	// Monitor 145 - existing active monitor from account 351 (priority 10, active)
	evaluatedMonitors = append(evaluatedMonitors, evaluatedMonitor{
		monitor: monitorCandidate{
			ID:           145,
			AccountID:    &account351,
			ServerStatus: ntpdb.ServerScoresStatusActive,
			GlobalStatus: ntpdb.MonitorsStatusActive,
			Priority:     10,
			RTT:          9.17, // From SQL data
			IsHealthy:    true,
			Count:        131, // From SQL data
		},
		recommendedState: candidateIn,
		currentViolation: &constraintViolation{Type: violationNone},
	})

	// Monitor 106 - high-performing candidate from account 351 (priority 1, candidate)
	// This should be promoted but isn't in real scenario
	evaluatedMonitors = append(evaluatedMonitors, evaluatedMonitor{
		monitor: monitorCandidate{
			ID:           106,
			AccountID:    &account351,
			ServerStatus: ntpdb.ServerScoresStatusCandidate,
			GlobalStatus: ntpdb.MonitorsStatusActive,
			Priority:     1,    // Excellent performance
			RTT:          0.32, // From SQL data
			IsHealthy:    true,
			Count:        249, // From SQL data - well above minCountForTesting
		},
		recommendedState: candidateIn,
		currentViolation: &constraintViolation{Type: violationNone},
	})

	// Monitor 160 - another high-performing candidate from account 351 (priority 11, candidate)
	evaluatedMonitors = append(evaluatedMonitors, evaluatedMonitor{
		monitor: monitorCandidate{
			ID:           160,
			AccountID:    &account351,
			ServerStatus: ntpdb.ServerScoresStatusCandidate,
			GlobalStatus: ntpdb.MonitorsStatusActive,
			Priority:     11,
			RTT:          10.14, // From SQL data
			IsHealthy:    true,
			Count:        12, // From SQL data - meets minCountForTesting (9)
		},
		recommendedState: candidateIn,
		currentViolation: &constraintViolation{Type: violationNone},
	})

	// Add some testing monitors from other accounts that perform worse
	// These should be replaceable by monitors 106 and 160

	// Account 1 monitors (limit 3 per server from SQL data)
	account1 := uint32(1)

	// Monitor 108 - testing monitor from account 1 (priority 9, active in SQL but let's make it testing for the test)
	evaluatedMonitors = append(evaluatedMonitors, evaluatedMonitor{
		monitor: monitorCandidate{
			ID:           108,
			AccountID:    &account1,
			ServerStatus: ntpdb.ServerScoresStatusTesting,
			GlobalStatus: ntpdb.MonitorsStatusActive,
			Priority:     20,   // Made worse than monitor 160 to trigger replacement
			RTT:          7.76, // From SQL data
			IsHealthy:    true,
			Count:        235,
		},
		recommendedState: candidateIn,
		currentViolation: &constraintViolation{Type: violationNone},
	})

	// Account 375 monitor (no specific limit in SQL)
	account375 := uint32(375)

	// Monitor 116 - testing monitor from account 375 (priority 16, testing)
	evaluatedMonitors = append(evaluatedMonitors, evaluatedMonitor{
		monitor: monitorCandidate{
			ID:           116,
			AccountID:    &account375,
			ServerStatus: ntpdb.ServerScoresStatusTesting,
			GlobalStatus: ntpdb.MonitorsStatusActive,
			Priority:     16,
			RTT:          14.60, // From SQL data
			IsHealthy:    true,
			Count:        221,
		},
		recommendedState: candidateIn,
		currentViolation: &constraintViolation{Type: violationNone},
	})

	// Monitor 104 - testing monitor from account 1 (priority 17, testing)
	evaluatedMonitors = append(evaluatedMonitors, evaluatedMonitor{
		monitor: monitorCandidate{
			ID:           104,
			AccountID:    &account1,
			ServerStatus: ntpdb.ServerScoresStatusTesting,
			GlobalStatus: ntpdb.MonitorsStatusActive,
			Priority:     17,
			RTT:          15.89, // From SQL data
			IsHealthy:    true,
			Count:        71,
		},
		recommendedState: candidateIn,
		currentViolation: &constraintViolation{Type: violationNone},
	})

	// Add more testing monitors to simulate testing pool at capacity
	// This forces Rule 5 to use performance-based replacement instead of capacity-based promotion

	// Account 394 monitors (limit 2 per server from SQL data)
	account394 := uint32(394)

	// Add 6 active monitors from account 394 to reach target of 7 active (total: 1 + 6 = 7)
	for i := 0; i < 6; i++ {
		evaluatedMonitors = append(evaluatedMonitors, evaluatedMonitor{
			monitor: monitorCandidate{
				ID:           uint32(200 + i),
				AccountID:    &account394,
				ServerStatus: ntpdb.ServerScoresStatusActive,
				GlobalStatus: ntpdb.MonitorsStatusActive,
				Priority:     10 + i*2,
				RTT:          float64(10 + i*2),
				IsHealthy:    true,
				Count:        100,
			},
			recommendedState: candidateIn,
			currentViolation: &constraintViolation{Type: violationNone},
		})
	}

	// Add 2 more testing monitors to fill testing pool to capacity (5 total)
	// These will have worse performance than monitors 106 and 160, forcing replacement
	account593 := uint32(593)

	evaluatedMonitors = append(evaluatedMonitors, evaluatedMonitor{
		monitor: monitorCandidate{
			ID:           84, // From SQL data
			AccountID:    &account593,
			ServerStatus: ntpdb.ServerScoresStatusTesting,
			GlobalStatus: ntpdb.MonitorsStatusActive,
			Priority:     25,   // Worse than monitor 160 (priority 11)
			RTT:          9.82, // From SQL data
			IsHealthy:    true,
			Count:        324,
		},
		recommendedState: candidateIn,
		currentViolation: &constraintViolation{Type: violationNone},
	})

	evaluatedMonitors = append(evaluatedMonitors, evaluatedMonitor{
		monitor: monitorCandidate{
			ID:           98, // From SQL data
			AccountID:    &account394,
			ServerStatus: ntpdb.ServerScoresStatusTesting,
			GlobalStatus: ntpdb.MonitorsStatusActive,
			Priority:     30,    // Much worse than monitor 106 (priority 1)
			RTT:          16.76, // From SQL data
			IsHealthy:    true,
			Count:        71,
		},
		recommendedState: candidateIn,
		currentViolation: &constraintViolation{Type: violationNone},
	})

	server := &serverInfo{ID: 2200} // Server from SQL query

	// Set up account limits exactly as shown in SQL data
	accountLimits := map[uint32]*accountLimit{
		351: { // Account 351 - the problematic account
			AccountID:    351,
			MaxPerServer: 2, // monitors_per_server_limit from SQL
			ActiveCount:  1, // Monitor 145 is active
			TestingCount: 0, // No testing monitors currently
		},
		1: {
			AccountID:    1,
			MaxPerServer: 3, // monitors_per_server_limit from SQL
			ActiveCount:  0,
			TestingCount: 2, // Monitors 108, 104 are testing
		},
		375: {
			AccountID:    375,
			MaxPerServer: 2, // Default since no limit in SQL data
			ActiveCount:  0,
			TestingCount: 1, // Monitor 116 is testing
		},
		394: {
			AccountID:    394,
			MaxPerServer: 2, // monitors_per_server_limit from SQL
			ActiveCount:  6, // The active monitors we added
			TestingCount: 1, // Monitor 98 is testing
		},
		593: {
			AccountID:    593,
			MaxPerServer: 2, // Default
			ActiveCount:  0,
			TestingCount: 1, // Monitor 84 is testing
		},
	}

	changes := s.applySelectionRules(ctx, evaluatedMonitors, server, accountLimits, nil)

	// Debug logging
	t.Logf("=== Real World Account 351 Scenario Test ===")
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
	t.Logf("Initial state: %d active, %d testing, %d candidates", activeCount, testingCount, candidateCount)

	// Log account 351 monitors specifically
	t.Logf("=== Account 351 monitors ===")
	for _, em := range evaluatedMonitors {
		if em.monitor.AccountID != nil && *em.monitor.AccountID == 351 {
			t.Logf("Monitor %d: Status=%s, Priority=%d, RTT=%.2f, Count=%d",
				em.monitor.ID, em.monitor.ServerStatus, em.monitor.Priority, em.monitor.RTT, em.monitor.Count)
		}
	}

	// Log all changes
	t.Logf("=== Status Changes ===")
	t.Logf("Total changes: %d", len(changes))
	for i, change := range changes {
		t.Logf("Change %d: monitor %d from %s to %s, reason: %s",
			i, change.monitorID, change.fromStatus, change.toStatus, change.reason)
	}

	// Key assertions based on expected behavior
	monitor106Promoted := false
	monitor160Promoted := false
	replacementPromotions := 0
	replacementDemotions := 0

	for _, change := range changes {
		// Track Monitor 106 (should be promoted due to excellent performance)
		if change.monitorID == 106 &&
			change.fromStatus == ntpdb.ServerScoresStatusCandidate &&
			change.toStatus == ntpdb.ServerScoresStatusTesting {
			monitor106Promoted = true
		}

		// Track Monitor 160 (should potentially be promoted)
		if change.monitorID == 160 &&
			change.fromStatus == ntpdb.ServerScoresStatusCandidate &&
			change.toStatus == ntpdb.ServerScoresStatusTesting {
			monitor160Promoted = true
		}

		// Count replacement operations
		if change.reason == "replacement promotion" {
			replacementPromotions++
		}
		if change.reason == "replaced by better candidate" {
			replacementDemotions++
		}
	}

	// Primary assertion: Monitor 106 should be promoted via performance replacement
	// It has priority 1 which significantly outperforms testing monitors with priority 16, 17, 20
	if !monitor106Promoted {
		t.Errorf("Monitor 106 (priority 1) should have been promoted to testing via performance replacement, but wasn't")
		t.Logf("This indicates the Rule 5 performance replacement logic is not working as expected")
	}

	// Check that replacements are balanced (equal promotions and demotions)
	if replacementPromotions != replacementDemotions {
		t.Errorf("Unbalanced replacements: %d promotions vs %d demotions",
			replacementPromotions, replacementDemotions)
	}

	// Log the outcome
	t.Logf("=== Test Results ===")
	t.Logf("Monitor 106 promoted: %v", monitor106Promoted)
	t.Logf("Monitor 160 promoted: %v", monitor160Promoted)
	t.Logf("Replacement operations: %d promotions, %d demotions", replacementPromotions, replacementDemotions)

	// If monitor 106 wasn't promoted, this test exposes the bug we need to fix
	if !monitor106Promoted {
		t.Logf("🔍 BUG REPRODUCED: High-performing candidate not promoted despite outperforming testing monitors")
		t.Logf("This test case can be used to debug and fix the Rule 5 performance replacement logic")
	}
}
