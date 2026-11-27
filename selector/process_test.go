package selector

import (
	"context"
	"log/slog"
	"testing"

	"go.ntppool.org/monitor/monitorsettings"
	"go.ntppool.org/monitor/ntpdb"
)

func TestProcessServerTypes(t *testing.T) {
	// This is a compilation test to ensure the processServer implementation
	// compiles correctly with all its dependencies

	sl := &Selector{
		log: slog.Default(),
		ctx: context.Background(),
	}

	// Test that evaluatedMonitor struct is properly defined
	em := evaluatedMonitor{
		monitor: monitorCandidate{
			ID:           1,
			GlobalStatus: ntpdb.MonitorsStatusActive,
			ServerStatus: ntpdb.ServerScoresStatusActive,
		},
		currentViolation: &constraintViolation{
			Type: violationNone,
		},
		recommendedState: candidateIn,
	}

	// Test that statusChange struct is properly defined
	sc := statusChange{
		monitorID:  1,
		fromStatus: ntpdb.ServerScoresStatusActive,
		toStatus:   ntpdb.ServerScoresStatusTesting,
		reason:     "test",
	}

	// Verify types compile
	_ = em
	_ = sc
	_ = sl
}

func TestSelectionHelpers(t *testing.T) {
	sl := &Selector{
		log: slog.Default(),
	}

	// Test countHealthy
	monitors := []evaluatedMonitor{
		{
			monitor:          monitorCandidate{IsHealthy: true},
			recommendedState: candidateIn,
		},
		{
			monitor:          monitorCandidate{IsHealthy: true},
			recommendedState: candidateOut,
		},
		{
			monitor:          monitorCandidate{IsHealthy: false},
			recommendedState: candidateIn,
		},
	}

	if count := sl.countHealthy(monitors); count != 1 {
		t.Errorf("countHealthy() = %d, want 1", count)
	}

	// Test countGloballyActive
	monitors = []evaluatedMonitor{
		{
			monitor: monitorCandidate{GlobalStatus: ntpdb.MonitorsStatusActive},
		},
		{
			monitor: monitorCandidate{GlobalStatus: ntpdb.MonitorsStatusTesting},
		},
		{
			monitor: monitorCandidate{GlobalStatus: ntpdb.MonitorsStatusActive},
		},
	}

	if count := sl.countGloballyActive(monitors); count != 2 {
		t.Errorf("countGloballyActive() = %d, want 2", count)
	}
}

func TestCalculateNeededCandidates(t *testing.T) {
	sl := &Selector{
		log: slog.Default(),
	}

	tests := []struct {
		name       string
		active     int
		testing    int
		candidates int
		want       int
	}{
		{
			name:       "need_candidates",
			active:     5,
			testing:    3,
			candidates: 2,
			want:       10, // targetActive(7) + targetTesting(5) - current(2) = 10
		},
		{
			name:       "enough_candidates",
			active:     7,
			testing:    5,
			candidates: 12,
			want:       0,
		},
		{
			name:       "exactly_enough",
			active:     7,
			testing:    5,
			candidates: 12,
			want:       0,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := sl.calculateNeededCandidates(tt.active, tt.testing, tt.candidates)
			if got != tt.want {
				t.Errorf("calculateNeededCandidates(%d, %d, %d) = %d, want %d",
					tt.active, tt.testing, tt.candidates, got, tt.want)
			}
		})
	}
}

func TestHandleOutOfOrder(t *testing.T) {
	sl := &Selector{
		log: slog.Default(),
	}

	active := []evaluatedMonitor{
		{
			monitor: monitorCandidate{
				ID:           1,
				ServerStatus: ntpdb.ServerScoresStatusActive,
			},
			recommendedState: candidateIn,
		},
	}

	testing := []evaluatedMonitor{
		{
			monitor: monitorCandidate{
				ID:           2,
				ServerStatus: ntpdb.ServerScoresStatusTesting,
			},
			recommendedState: candidateIn,
		},
	}

	changes := []statusChange{}

	// Test that handleOutOfOrder builds the proper newStatusList
	result := sl.handleOutOfOrder(active, testing, changes)

	// Should create out-of-order swap if testing monitor should replace active
	// But since we need IsOutOfOrder to work, this is just a compilation test
	_ = result
}

// TestCalculateSafetyLimits tests the critical safety logic functions
// Based on Phase 1 of the selector testing plan
func TestCalculateSafetyLimits_EmergencyConditions(t *testing.T) {
	sl := &Selector{
		log: slog.Default(),
	}

	tests := []struct {
		name                string
		targetNumber        int
		totalMonitors       int
		healthyActive       int
		activeCount         int
		limits              changeLimits
		expectedMaxRemovals int
		expectEmergency     bool
		description         string
	}{
		{
			name:                "emergency_not_enough_monitors",
			targetNumber:        7,
			totalMonitors:       4,
			healthyActive:       2,
			activeCount:         3,
			limits:              changeLimits{activeRemovals: 2},
			expectedMaxRemovals: 0,
			expectEmergency:     true,
			description:         "Emergency condition: target > total monitors AND healthy < active",
		},
		{
			name:                "safety_below_target_unhealthy",
			targetNumber:        7,
			totalMonitors:       10,
			healthyActive:       2,
			activeCount:         3,
			limits:              changeLimits{activeRemovals: 2},
			expectedMaxRemovals: 0,
			expectEmergency:     false,
			description:         "Safety condition: at/below target with insufficient healthy monitors",
		},
		{
			name:                "normal_operation_above_target",
			targetNumber:        7,
			totalMonitors:       10,
			healthyActive:       8,
			activeCount:         8,
			limits:              changeLimits{activeRemovals: 2},
			expectedMaxRemovals: 2,
			expectEmergency:     false,
			description:         "Normal operation: above target with sufficient healthy monitors",
		},
		{
			name:                "emergency_never_remove_all_active",
			targetNumber:        5,
			totalMonitors:       8,
			healthyActive:       6, // More than target to avoid safety limit
			activeCount:         3,
			limits:              changeLimits{activeRemovals: 5}, // Would remove all
			expectedMaxRemovals: 2,                               // activeCount - 1
			expectEmergency:     false,
			description:         "Emergency safeguard: never remove all active monitors",
		},
		{
			name:                "edge_case_zero_active_count",
			targetNumber:        7,
			totalMonitors:       10,
			healthyActive:       0,
			activeCount:         0,
			limits:              changeLimits{activeRemovals: 2},
			expectedMaxRemovals: 2,
			expectEmergency:     false,
			description:         "Edge case: zero active monitors should not trigger safety limits",
		},
		{
			name:                "edge_case_single_active_monitor",
			targetNumber:        7,
			totalMonitors:       10,
			healthyActive:       1,
			activeCount:         1,
			limits:              changeLimits{activeRemovals: 2},
			expectedMaxRemovals: 0, // Would remove the single active monitor
			expectEmergency:     false,
			description:         "Edge case: single active monitor should be protected",
		},
		{
			name:                "boundary_exact_target_sufficient_healthy",
			targetNumber:        5,
			totalMonitors:       8,
			healthyActive:       5,
			activeCount:         5,
			limits:              changeLimits{activeRemovals: 1},
			expectedMaxRemovals: 1,
			expectEmergency:     false,
			description:         "Boundary: exactly at target with sufficient healthy monitors",
		},
		{
			name:                "boundary_exact_target_insufficient_healthy",
			targetNumber:        5,
			totalMonitors:       8,
			healthyActive:       3,
			activeCount:         5,
			limits:              changeLimits{activeRemovals: 2},
			expectedMaxRemovals: 0,
			expectEmergency:     false,
			description:         "Boundary: exactly at target but insufficient healthy monitors",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Create initial working state
			state := workingState{
				healthyActive:  tt.healthyActive,
				activeCount:    tt.activeCount,
				testingCount:   0,
				healthyTesting: 0,
				blockedCount:   0,
				maxRemovals:    0,
			}

			// Create mock evaluated monitors with the specified count
			evaluatedMonitors := make([]evaluatedMonitor, tt.totalMonitors)
			for i := 0; i < tt.totalMonitors; i++ {
				evaluatedMonitors[i] = evaluatedMonitor{
					monitor: monitorCandidate{
						ID:           int64(i + 1),
						GlobalStatus: ntpdb.MonitorsStatusActive,
						IsHealthy:    i < tt.healthyActive, // First N monitors are healthy
					},
				}
			}

			// Call calculateSafetyLimits
			result := sl.calculateSafetyLimits(
				context.Background(),
				state,
				tt.targetNumber,
				tt.limits,
				evaluatedMonitors,
			)

			// Verify maxRemovals
			if result.maxRemovals != tt.expectedMaxRemovals {
				t.Errorf("Expected maxRemovals=%d, got %d. %s",
					tt.expectedMaxRemovals, result.maxRemovals, tt.description)
			}

			// Verify other state fields are preserved
			if result.healthyActive != tt.healthyActive {
				t.Errorf("healthyActive should be preserved: expected %d, got %d",
					tt.healthyActive, result.healthyActive)
			}
			if result.activeCount != tt.activeCount {
				t.Errorf("activeCount should be preserved: expected %d, got %d",
					tt.activeCount, result.activeCount)
			}
		})
	}
}

// TestCalculateSafetyLimits_ConstraintInteractions tests that safety limits
// don't interfere with constraint processing
func TestCalculateSafetyLimits_ConstraintInteractions(t *testing.T) {
	sl := &Selector{
		log: slog.Default(),
	}

	tests := []struct {
		name                string
		scenario            string
		targetNumber        int
		totalMonitors       int
		healthyActive       int
		activeCount         int
		limits              changeLimits
		expectedMaxRemovals int
		shouldAllowChanges  bool
	}{
		{
			name:                "constraint_cleanup_allowed_in_emergency",
			scenario:            "Emergency condition should not block constraint demotions",
			targetNumber:        7,
			totalMonitors:       4, // Emergency: target > total
			healthyActive:       2,
			activeCount:         3,
			limits:              changeLimits{activeRemovals: 1},
			expectedMaxRemovals: 0, // Emergency sets maxRemovals to 0
			shouldAllowChanges:  false,
		},
		{
			name:                "normal_constraint_processing",
			scenario:            "Normal operations should allow constraint processing",
			targetNumber:        5,
			totalMonitors:       10,
			healthyActive:       6,
			activeCount:         7,
			limits:              changeLimits{activeRemovals: 2},
			expectedMaxRemovals: 2,
			shouldAllowChanges:  true,
		},
		{
			name:                "safety_preserves_minimum_monitors",
			scenario:            "Safety limits should prevent removing too many monitors",
			targetNumber:        3,
			totalMonitors:       5,
			healthyActive:       2,
			activeCount:         3,
			limits:              changeLimits{activeRemovals: 2},
			expectedMaxRemovals: 0, // Safety: healthyActive < targetNumber
			shouldAllowChanges:  false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			state := workingState{
				healthyActive:  tt.healthyActive,
				activeCount:    tt.activeCount,
				testingCount:   0,
				healthyTesting: 0,
				blockedCount:   0,
				maxRemovals:    0,
			}

			evaluatedMonitors := make([]evaluatedMonitor, tt.totalMonitors)
			for i := 0; i < tt.totalMonitors; i++ {
				evaluatedMonitors[i] = evaluatedMonitor{
					monitor: monitorCandidate{
						ID:           int64(i + 1),
						GlobalStatus: ntpdb.MonitorsStatusActive,
						IsHealthy:    i < tt.healthyActive,
					},
				}
			}

			result := sl.calculateSafetyLimits(
				context.Background(),
				state,
				tt.targetNumber,
				tt.limits,
				evaluatedMonitors,
			)

			if result.maxRemovals != tt.expectedMaxRemovals {
				t.Errorf("Expected maxRemovals=%d, got %d for scenario: %s",
					tt.expectedMaxRemovals, result.maxRemovals, tt.scenario)
			}

			// Test interpretation: maxRemovals > 0 means changes are allowed
			actualAllowsChanges := result.maxRemovals > 0
			if actualAllowsChanges != tt.shouldAllowChanges {
				t.Errorf("Expected shouldAllowChanges=%v, got %v for scenario: %s",
					tt.shouldAllowChanges, actualAllowsChanges, tt.scenario)
			}
		})
	}
}

// budgetScenario builds a server with the given number of healthy active
// monitors, testing monitors at testingPriority and candidates at
// candidatePriority (lower is better). Candidates and testing monitors have
// enough samples to be promoted.
func budgetScenario(active, testing, candidates, testingPriority, candidatePriority int) []evaluatedMonitor {
	var rows []evaluatedMonitor
	add := func(id int64, status ntpdb.ServerScoresStatus, priority int, count int64) {
		rows = append(rows, evaluatedMonitor{
			monitor: monitorCandidate{
				ID:           id,
				ServerStatus: status,
				GlobalStatus: ntpdb.MonitorsStatusActive,
				Priority:     priority,
				IsHealthy:    true,
				HasMetrics:   true,
				Count:        count,
			},
			recommendedState: candidateIn,
			currentViolation: &constraintViolation{Type: violationNone},
		})
	}
	for i := 0; i < active; i++ {
		add(int64(i+1), ntpdb.ServerScoresStatusActive, 10+i, 100)
	}
	for i := 0; i < testing; i++ {
		add(int64(i+10), ntpdb.ServerScoresStatusTesting, testingPriority, 20)
	}
	for i := 0; i < candidates; i++ {
		add(int64(i+20), ntpdb.ServerScoresStatusCandidate, candidatePriority, 20)
	}
	return rows
}

func countChanges(changes []statusChange, from, to ntpdb.ServerScoresStatus, reason string) int {
	n := 0
	for _, c := range changes {
		if c.fromStatus == from && c.toStatus == to && (reason == "" || c.reason == reason) {
			n++
		}
	}
	return n
}

func TestSelectionRulesPromotionBudget(t *testing.T) {
	const (
		candidate = ntpdb.ServerScoresStatusCandidate
		testing_  = ntpdb.ServerScoresStatusTesting
	)

	tests := []struct {
		name   string
		rows   []evaluatedMonitor
		counts ntpdb.GetServerPromotionCountsRow
		// candidate -> testing promotions with the given reason
		reason string
		want   int
	}{
		{
			// both candidates are far better than the testing monitors, but a
			// performance replacement is allowed only while nothing was
			// promoted into testing: the second sees the first
			name:   "two performance replacements in one review give one",
			rows:   budgetScenario(7, 5, 2, 100, 50),
			counts: ntpdb.GetServerPromotionCountsRow{},
			reason: "replacement promotion",
			want:   1,
		},
		{
			name:   "performance replacement refused after a promotion in the window",
			rows:   budgetScenario(7, 5, 2, 100, 50),
			counts: ntpdb.GetServerPromotionCountsRow{IntoTesting: 1},
			reason: "replacement promotion",
			want:   0,
		},
		{
			name:   "fill refused at the daily limit",
			rows:   budgetScenario(7, 2, 3, 100, 100),
			counts: ntpdb.GetServerPromotionCountsRow{IntoTesting: 3},
			reason: "candidate to testing",
			want:   0,
		},
		{
			name:   "fill allowed below the daily limit",
			rows:   budgetScenario(7, 2, 3, 100, 100),
			counts: ntpdb.GetServerPromotionCountsRow{IntoTesting: 1},
			reason: "candidate to testing",
			want:   2,
		},
		{
			name:   "fills stop at the limit within one review",
			rows:   budgetScenario(7, 2, 3, 100, 100),
			counts: ntpdb.GetServerPromotionCountsRow{IntoTesting: 2},
			reason: "candidate to testing",
			want:   1,
		},
		{
			name:   "bootstrap mode is not limited",
			rows:   budgetScenario(3, 2, 3, 100, 100),
			counts: ntpdb.GetServerPromotionCountsRow{IntoTesting: 9},
			reason: "candidate to testing",
			want:   2, // Rule 5 fills at most 2 per review
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			sl := &Selector{log: slog.Default()}
			changes := sl.applySelectionRules(
				context.Background(), tt.rows, &serverInfo{ID: 1982},
				map[int64]*accountLimit{}, nil, sl.promotionBudgetFor(tt.counts, tt.rows),
			)
			if got := countChanges(changes, candidate, testing_, tt.reason); got != tt.want {
				t.Errorf("%q promotions = %d, want %d; changes: %+v", tt.reason, got, tt.want, changes)
			}
		})
	}
}

// A testing -> active swap is a promotion into active; the demotion of the
// active monitor does not count as a promotion into testing.
func TestRule3SwapCountsAsPromotionIntoActiveOnly(t *testing.T) {
	sl := &Selector{log: slog.Default()}
	rows := budgetScenario(7, 3, 0, 40, 0)
	for i := range rows {
		// the testing monitors are much better than the active ones
		if rows[i].monitor.ServerStatus == ntpdb.ServerScoresStatusTesting {
			rows[i].monitor.Priority = 5
			rows[i].monitor.Count = monitorsettings.HistoricalMinForActive
		} else {
			rows[i].monitor.Priority = 100 + i
		}
	}
	var active, testing []evaluatedMonitor
	for _, em := range rows {
		if em.monitor.ServerStatus == ntpdb.ServerScoresStatusActive {
			active = append(active, em)
		} else {
			testing = append(testing, em)
		}
	}

	budget := newPromotionBudget(ntpdb.GetServerPromotionCountsRow{}, serverMode{}, 3)
	selCtx := selectionContext{
		budget:            budget,
		evaluatedMonitors: rows,
		server:            &serverInfo{ID: 1982},
		limits:            changeLimits{promotions: 4, activeRemovals: 2, testingRemovals: 2},
		targetNumber:      targetActiveMonitors,
	}

	result := sl.applyRule3TestingToActivePromotion(
		context.Background(), selCtx, testing, active, map[int64]*accountLimit{}, 7, 3,
	)

	swaps := countChanges(result.changes, ntpdb.ServerScoresStatusTesting, ntpdb.ServerScoresStatusActive, "")
	if swaps != 1 {
		t.Fatalf("swaps = %d, want 1 (a second performance replacement is refused); changes: %+v", swaps, result.changes)
	}
	if got := countChanges(result.changes, ntpdb.ServerScoresStatusActive, ntpdb.ServerScoresStatusTesting, ""); got != 1 {
		t.Errorf("active demotions = %d, want 1", got)
	}
	if budget.intoActive != 1 || budget.intoTesting != 0 {
		t.Errorf("budget counts into active %d, into testing %d; want 1 and 0", budget.intoActive, budget.intoTesting)
	}
}

// With no testing monitors both Rule 5 and Rule 6 promote candidates. A
// candidate gets at most one change, and the budget counts it once.
func TestSelectionRulesNoDuplicatePromotion(t *testing.T) {
	sl := &Selector{log: slog.Default()}
	rows := budgetScenario(6, 0, 3, 100, 100)
	budget := sl.promotionBudgetFor(ntpdb.GetServerPromotionCountsRow{}, rows)
	changes := sl.applySelectionRules(
		context.Background(), rows, &serverInfo{ID: 1982},
		map[int64]*accountLimit{}, nil, budget,
	)

	seen := map[int64]int{}
	for _, c := range changes {
		seen[c.monitorID]++
		if seen[c.monitorID] > 1 {
			t.Errorf("monitor %d has %d changes; changes: %+v", c.monitorID, seen[c.monitorID], changes)
		}
	}
	promoted := countChanges(changes, ntpdb.ServerScoresStatusCandidate, ntpdb.ServerScoresStatusTesting, "")
	if promoted != 3 {
		t.Errorf("candidate -> testing promotions = %d, want 3; changes: %+v", promoted, changes)
	}
	if budget.intoTesting != int64(promoted) {
		t.Errorf("budget counts %d promotions into testing, want %d", budget.intoTesting, promoted)
	}
	if r := budget.refused(); len(r) != 0 {
		t.Errorf("refusals = %+v, want none", r)
	}
}
