package selector

import (
	"context"
	"log/slog"
	"testing"

	"go.ntppool.org/monitor/ntpdb"
)

// TestRule15_ActiveExcessDemotion tests that Rule 1.5 (Active Excess Demotion) demotes excess active monitors
func TestRule15_ActiveExcessDemotion(t *testing.T) {
	ctx := context.Background()
	s := &Selector{
		log: slog.Default(),
	}

	// Create scenario: 9 active monitors (target 7), no testing monitors
	activeMonitors := make([]evaluatedMonitor, 9)
	for i := 0; i < 9; i++ {
		activeMonitors[i] = evaluatedMonitor{
			monitor: monitorCandidate{
				ID:           uint32(i + 1),
				ServerStatus: ntpdb.ServerScoresStatusActive,
				GlobalStatus: ntpdb.MonitorsStatusActive,
				RTT:          float64(i + 1), // Monitors 8-9 have highest RTT (worst performance)
				IsHealthy:    true,           // Make sure monitors are healthy
			},
			recommendedState: candidateIn, // All healthy
			currentViolation: &constraintViolation{Type: violationNone},
		}
	}

	server := &serverInfo{ID: 1982}
	accountLimits := make(map[uint32]*accountLimit)

	changes := s.applySelectionRules(ctx, activeMonitors, server, accountLimits, nil)

	// Debug: Log all changes
	t.Logf("Total changes: %d", len(changes))
	for i, change := range changes {
		t.Logf("Change %d: monitor %d from %s to %s, reason: %s", i, change.monitorID, change.fromStatus, change.toStatus, change.reason)
	}

	// Should demote 2 excess active monitors (9 -> 7)
	activeDemotions := 0
	for _, change := range changes {
		if change.fromStatus == ntpdb.ServerScoresStatusActive && change.toStatus == ntpdb.ServerScoresStatusTesting {
			activeDemotions++
			t.Logf("Demoted monitor %d: %s", change.monitorID, change.reason)
		}
	}

	if activeDemotions != 2 {
		t.Errorf("Expected 2 active demotions, got %d", activeDemotions)
	}

	// Should demote worst performers (monitors 8 and 9 with highest RTT)
	expectedDemotions := []uint32{8, 9}
	actualDemotions := []uint32{}
	for _, change := range changes {
		if change.fromStatus == ntpdb.ServerScoresStatusActive && change.toStatus == ntpdb.ServerScoresStatusTesting {
			actualDemotions = append(actualDemotions, change.monitorID)
		}
	}

	for _, expected := range expectedDemotions {
		found := false
		for _, actual := range actualDemotions {
			if actual == expected {
				found = true
				break
			}
		}
		if !found {
			t.Errorf("Expected monitor %d to be demoted, but it wasn't", expected)
		}
	}
}

// TestRule15_EmergencyOverrideBlocked tests that Rule 1.5 (Active Excess Demotion) is blocked during emergency override
func TestRule15_EmergencyOverrideBlocked(t *testing.T) {
	ctx := context.Background()
	s := &Selector{
		log: slog.Default(),
	}

	// Create scenario: 0 active, 9 testing monitors (should trigger emergency override)
	testingMonitors := make([]evaluatedMonitor, 9)
	for i := 0; i < 9; i++ {
		testingMonitors[i] = evaluatedMonitor{
			monitor: monitorCandidate{
				ID:           uint32(i + 1),
				ServerStatus: ntpdb.ServerScoresStatusTesting,
				GlobalStatus: ntpdb.MonitorsStatusActive,
			},
			recommendedState: candidateIn,
		}
	}

	server := &serverInfo{ID: 1982}
	accountLimits := make(map[uint32]*accountLimit)

	changes := s.applySelectionRules(ctx, testingMonitors, server, accountLimits, nil)

	// Should have promotions (emergency override) but no active→testing demotions from Rule 1.5 (Active Excess Demotion)
	promotions := 0
	activeDemotions := 0
	for _, change := range changes {
		if change.fromStatus == ntpdb.ServerScoresStatusTesting && change.toStatus == ntpdb.ServerScoresStatusActive {
			promotions++
		}
		if change.fromStatus == ntpdb.ServerScoresStatusActive && change.toStatus == ntpdb.ServerScoresStatusTesting &&
			change.reason == "excess active demotion" {
			activeDemotions++
		}
	}

	if promotions == 0 {
		t.Error("Expected emergency promotions, got none")
	}

	if activeDemotions > 0 {
		t.Errorf("Rule 1.5 (Active Excess Demotion) should be blocked during emergency override, but got %d demotions", activeDemotions)
	}
}

// TestRule15_SafetyCheck tests that Rule 1.5 (Active Excess Demotion) never reduces active count to 0
func TestRule15_SafetyCheck(t *testing.T) {
	ctx := context.Background()
	s := &Selector{
		log: slog.Default(),
	}

	// Create scenario: 1 active monitor (should not be demoted due to safety check)
	activeMonitors := []evaluatedMonitor{
		{
			monitor: monitorCandidate{
				ID:           1,
				ServerStatus: ntpdb.ServerScoresStatusActive,
				GlobalStatus: ntpdb.MonitorsStatusActive,
			},
			recommendedState: candidateIn,
		},
	}

	server := &serverInfo{ID: 1982}
	accountLimits := make(map[uint32]*accountLimit)

	changes := s.applySelectionRules(ctx, activeMonitors, server, accountLimits, nil)

	// Should not demote the only active monitor
	activeDemotions := 0
	for _, change := range changes {
		if change.fromStatus == ntpdb.ServerScoresStatusActive && change.toStatus == ntpdb.ServerScoresStatusTesting &&
			change.reason == "excess active demotion" {
			activeDemotions++
		}
	}

	if activeDemotions > 0 {
		t.Errorf("Rule 1.5 (Active Excess Demotion) should not reduce active count to 0, but got %d demotions", activeDemotions)
	}
}

// TestRule15_WithCandidatePromotion tests the Server 1486 scenario
func TestRule15_WithCandidatePromotion(t *testing.T) {
	ctx := context.Background()
	s := &Selector{
		log: slog.Default(),
	}

	// Create scenario: 8 active, 4 testing, 2 candidates (like Server 1486)
	activeMonitors := make([]evaluatedMonitor, 8)
	for i := 0; i < 8; i++ {
		activeMonitors[i] = evaluatedMonitor{
			monitor: monitorCandidate{
				ID:           uint32(i + 1),
				ServerStatus: ntpdb.ServerScoresStatusActive,
				GlobalStatus: ntpdb.MonitorsStatusActive,
				RTT:          float64(i + 1),
				IsHealthy:    true,
			},
			recommendedState: candidateIn,
			currentViolation: &constraintViolation{Type: violationNone},
		}
	}

	testingMonitors := make([]evaluatedMonitor, 4)
	for i := 0; i < 4; i++ {
		testingMonitors[i] = evaluatedMonitor{
			monitor: monitorCandidate{
				ID:           uint32(i + 10),
				ServerStatus: ntpdb.ServerScoresStatusTesting,
				GlobalStatus: ntpdb.MonitorsStatusActive,
				RTT:          float64(i + 10),
				IsHealthy:    true,
			},
			recommendedState: candidateIn,
			currentViolation: &constraintViolation{Type: violationNone},
		}
	}

	candidateMonitors := make([]evaluatedMonitor, 2)
	for i := 0; i < 2; i++ {
		candidateMonitors[i] = evaluatedMonitor{
			monitor: monitorCandidate{
				ID:           uint32(i + 20),
				ServerStatus: ntpdb.ServerScoresStatusCandidate,
				GlobalStatus: ntpdb.MonitorsStatusActive,
				IsHealthy:    true,
			},
			recommendedState: candidateIn,
			currentViolation: &constraintViolation{Type: violationNone},
		}
	}

	allMonitors := append(append(activeMonitors, testingMonitors...), candidateMonitors...)
	server := &serverInfo{ID: 1486}
	accountLimits := make(map[uint32]*accountLimit)

	changes := s.applySelectionRules(ctx, allMonitors, server, accountLimits, nil)

	// Count changes by type
	activeDemotions := 0
	candidatePromotions := 0
	testingDemotions := 0

	for _, change := range changes {
		t.Logf("Change: monitor %d from %s to %s, reason: %s",
			change.monitorID, change.fromStatus, change.toStatus, change.reason)

		if change.fromStatus == ntpdb.ServerScoresStatusActive && change.toStatus == ntpdb.ServerScoresStatusTesting {
			activeDemotions++
		}
		if change.fromStatus == ntpdb.ServerScoresStatusCandidate && change.toStatus == ntpdb.ServerScoresStatusTesting {
			candidatePromotions++
		}
		if change.fromStatus == ntpdb.ServerScoresStatusTesting && change.toStatus == ntpdb.ServerScoresStatusCandidate {
			testingDemotions++
		}
	}

	// Expected behavior:
	// - Should demote 1 active (8→7)
	// - Should promote 1 candidate to testing (fill testing pool)
	// - Should demote 1 testing back to candidate (6→5 to respect dynamic target)

	if activeDemotions != 1 {
		t.Errorf("Expected 1 active demotion, got %d", activeDemotions)
	}

	// With working count fix, should not over-promote to testing
	finalTestingCount := 4 + activeDemotions + candidatePromotions - testingDemotions
	if finalTestingCount > 5 {
		t.Errorf("Final testing count %d exceeds dynamic target of 5", finalTestingCount)
	}
}

// TestRule5_TestingCapacityLimit tests the Server 1065 scenario
func TestRule5_TestingCapacityLimit(t *testing.T) {
	ctx := context.Background()
	s := &Selector{
		log: slog.Default(),
	}

	// Create scenario: 7 active, 4 testing (after gradual removal), many candidates
	activeMonitors := make([]evaluatedMonitor, 7)
	for i := 0; i < 7; i++ {
		activeMonitors[i] = evaluatedMonitor{
			monitor: monitorCandidate{
				ID:           uint32(i + 1),
				ServerStatus: ntpdb.ServerScoresStatusActive,
				GlobalStatus: ntpdb.MonitorsStatusActive,
				IsHealthy:    true,
			},
			recommendedState: candidateIn,
			currentViolation: &constraintViolation{Type: violationNone},
		}
	}

	// 4 testing monitors (after 1 was removed via gradual removal)
	testingMonitors := make([]evaluatedMonitor, 4)
	for i := 0; i < 4; i++ {
		testingMonitors[i] = evaluatedMonitor{
			monitor: monitorCandidate{
				ID:           uint32(i + 10),
				ServerStatus: ntpdb.ServerScoresStatusTesting,
				GlobalStatus: ntpdb.MonitorsStatusActive,
				IsHealthy:    true,
			},
			recommendedState: candidateIn,
			currentViolation: &constraintViolation{Type: violationNone},
		}
	}

	// 5 candidates available for promotion
	candidateMonitors := make([]evaluatedMonitor, 5)
	for i := 0; i < 5; i++ {
		candidateMonitors[i] = evaluatedMonitor{
			monitor: monitorCandidate{
				ID:           uint32(i + 20),
				ServerStatus: ntpdb.ServerScoresStatusCandidate,
				GlobalStatus: ntpdb.MonitorsStatusActive,
				IsHealthy:    true,
			},
			recommendedState: candidateIn,
			currentViolation: &constraintViolation{Type: violationNone},
		}
	}

	allMonitors := append(append(activeMonitors, testingMonitors...), candidateMonitors...)
	server := &serverInfo{ID: 1065}
	accountLimits := make(map[uint32]*accountLimit)

	changes := s.applySelectionRules(ctx, allMonitors, server, accountLimits, nil)

	// Count changes by type
	candidatePromotions := 0
	testingDemotions := 0

	for _, change := range changes {
		t.Logf("Change: monitor %d from %s to %s, reason: %s",
			change.monitorID, change.fromStatus, change.toStatus, change.reason)

		if change.fromStatus == ntpdb.ServerScoresStatusCandidate && change.toStatus == ntpdb.ServerScoresStatusTesting {
			candidatePromotions++
		}
		if change.fromStatus == ntpdb.ServerScoresStatusTesting && change.toStatus == ntpdb.ServerScoresStatusCandidate {
			testingDemotions++
		}
	}

	// Expected behavior:
	// - Dynamic testing target = 5 base + 0 active gap = 5
	// - Current testing = 4, so capacity = 5 - 4 = 1
	// - Should promote exactly 1 candidate (not 2)
	// - Should not need Rule 2.5 (Testing Pool Management) demotions

	if candidatePromotions != 1 {
		t.Errorf("Expected 1 candidate promotion (testing capacity=1), got %d", candidatePromotions)
	}

	if testingDemotions > 0 {
		t.Errorf("Expected 0 testing demotions (Rule 2.5 Testing Pool Management shouldn't run), got %d", testingDemotions)
	}

	finalTestingCount := 4 + candidatePromotions - testingDemotions
	if finalTestingCount != 5 {
		t.Errorf("Final testing count should be exactly 5, got %d", finalTestingCount)
	}
}
