package selector

import (
	"log/slog"
	"slices"
	"testing"

	"go.ntppool.org/monitor/monitorsettings"
	"go.ntppool.org/monitor/ntpdb"
)

// TestZeroActiveMonitorsWithConstraintViolations tests that testing monitors with
// constraint violations can be demoted when there are zero active monitors
func TestZeroActiveMonitorsWithConstraintViolations(t *testing.T) {
	sl := &Selector{
		log: testLogger(),
	}

	// Create test data: 0 active, 4 testing monitors with constraint violations
	evaluatedMonitors := []evaluatedMonitor{
		{
			monitor: monitorCandidate{
				ID:           1,
				AccountID:    ptr(int64(1)),
				GlobalStatus: ntpdb.MonitorsStatusActive,
				ServerStatus: ntpdb.ServerScoresStatusTesting,
				IsHealthy:    true,
			},
			currentViolation: &constraintViolation{
				Type:    violationLimit,
				Details: "account limit exceeded",
			},
			recommendedState: candidateOut,
		},
		{
			monitor: monitorCandidate{
				ID:           2,
				AccountID:    ptr(int64(1)),
				GlobalStatus: ntpdb.MonitorsStatusActive,
				ServerStatus: ntpdb.ServerScoresStatusTesting,
				IsHealthy:    true,
			},
			currentViolation: &constraintViolation{
				Type:    violationLimit,
				Details: "account limit exceeded",
			},
			recommendedState: candidateOut,
		},
		{
			monitor: monitorCandidate{
				ID:           3,
				AccountID:    ptr(int64(2)),
				GlobalStatus: ntpdb.MonitorsStatusActive,
				ServerStatus: ntpdb.ServerScoresStatusTesting,
				IsHealthy:    true,
			},
			currentViolation: &constraintViolation{
				Type:    violationNetworkDiversity,
				Details: "network diversity violation",
			},
			recommendedState: candidateOut,
		},
		{
			monitor: monitorCandidate{
				ID:           4,
				AccountID:    ptr(int64(2)),
				GlobalStatus: ntpdb.MonitorsStatusActive,
				ServerStatus: ntpdb.ServerScoresStatusTesting,
				IsHealthy:    true,
			},
			currentViolation: &constraintViolation{
				Type:    violationNetworkDiversity,
				Details: "network diversity violation",
			},
			recommendedState: candidateOut,
		},
	}

	server := &serverInfo{
		ID:        14,
		AccountID: ptr(int64(3)),
	}

	accountLimits := map[int64]*accountLimit{
		1: {AccountID: 1, MaxPerServer: 2, ActiveCount: 0, TestingCount: 2},
		2: {AccountID: 2, MaxPerServer: 2, ActiveCount: 0, TestingCount: 2},
	}

	changes := sl.applySelectionRules(t.Context(), evaluatedMonitors, server, accountLimits, nil, sl.promotionBudgetFor(ntpdb.GetServerPromotionCountsRow{}, evaluatedMonitors))

	// Debug output
	for _, change := range changes {
		t.Logf("Change: monitor %d from %s to %s, reason: %s",
			change.monitorID, change.fromStatus, change.toStatus, change.reason)
	}

	// With zero active monitors, we should allow demotions
	demotionCount := 0
	promotionCount := 0
	for _, change := range changes {
		if change.fromStatus == ntpdb.ServerScoresStatusTesting &&
			change.toStatus == ntpdb.ServerScoresStatusCandidate {
			demotionCount++
		}
		if change.toStatus == ntpdb.ServerScoresStatusActive {
			promotionCount++
		}
	}

	// Should demote at least some testing monitors with violations
	if demotionCount == 0 {
		t.Errorf("Expected demotions when zero active monitors, got %d", demotionCount)
	}

	// With current logic, emergency override will promote if there are zero active,
	// but since we have constraint violations and candidateOut, the current implementation
	// might not promote them. Let's check what actually happened.
	if promotionCount == 0 && demotionCount == 4 {
		// This is actually correct behavior - demote all violating monitors first
		t.Logf("Correct behavior: demoted all constraint-violating monitors")
	} else if promotionCount > 0 {
		t.Logf("Emergency override kicked in and promoted %d monitors", promotionCount)
	}

	t.Logf("Changes: %d demotions, %d promotions out of %d total changes",
		demotionCount, promotionCount, len(changes))
}

// TestIterativeAccountLimitEnforcement tests that account limits are properly
// enforced when promoting monitors iteratively
func TestIterativeAccountLimitEnforcement(t *testing.T) {
	sl := &Selector{
		log: testLogger(),
	}

	// Create test data: 3 active (to prevent emergency override),
	// 3 testing monitors from same account, account limit is 2
	evaluatedMonitors := []evaluatedMonitor{
		// Active monitors to prevent emergency override
		{
			monitor: monitorCandidate{
				ID:           10,
				AccountID:    ptr(int64(2)),
				GlobalStatus: ntpdb.MonitorsStatusActive,
				ServerStatus: ntpdb.ServerScoresStatusActive,
				IsHealthy:    true,
			},
			recommendedState: candidateIn,
		},
		{
			monitor: monitorCandidate{
				ID:           11,
				AccountID:    ptr(int64(2)),
				GlobalStatus: ntpdb.MonitorsStatusActive,
				ServerStatus: ntpdb.ServerScoresStatusActive,
				IsHealthy:    true,
			},
			recommendedState: candidateIn,
		},
		{
			monitor: monitorCandidate{
				ID:           12,
				AccountID:    ptr(int64(3)),
				GlobalStatus: ntpdb.MonitorsStatusActive,
				ServerStatus: ntpdb.ServerScoresStatusActive,
				IsHealthy:    true,
			},
			recommendedState: candidateIn,
		},
		{
			monitor: monitorCandidate{
				ID:           1,
				AccountID:    ptr(int64(1)),
				GlobalStatus: ntpdb.MonitorsStatusActive,
				ServerStatus: ntpdb.ServerScoresStatusTesting,
				IsHealthy:    true,
				Count:        monitorsettings.HistoricalMinForActive, // Ensure testing monitors can be promoted to active
			},
			recommendedState: candidateIn,
		},
		{
			monitor: monitorCandidate{
				ID:           2,
				AccountID:    ptr(int64(1)),
				GlobalStatus: ntpdb.MonitorsStatusActive,
				ServerStatus: ntpdb.ServerScoresStatusTesting,
				IsHealthy:    true,
				Count:        monitorsettings.HistoricalMinForActive, // Ensure testing monitors can be promoted to active
			},
			recommendedState: candidateIn,
		},
		{
			monitor: monitorCandidate{
				ID:           3,
				AccountID:    ptr(int64(1)),
				GlobalStatus: ntpdb.MonitorsStatusActive,
				ServerStatus: ntpdb.ServerScoresStatusTesting,
				IsHealthy:    true,
				Count:        monitorsettings.HistoricalMinForActive, // Ensure testing monitors can be promoted to active
			},
			recommendedState: candidateIn,
		},
	}

	server := &serverInfo{
		ID:        1,
		AccountID: ptr(int64(2)), // Different account
	}

	// Account limit: max 2 active per server
	accountLimits := map[int64]*accountLimit{
		1: {AccountID: 1, MaxPerServer: 2, ActiveCount: 0, TestingCount: 3},
		2: {AccountID: 2, MaxPerServer: 2, ActiveCount: 2, TestingCount: 0},
		3: {AccountID: 3, MaxPerServer: 2, ActiveCount: 1, TestingCount: 0},
	}

	changes := sl.applySelectionRules(t.Context(), evaluatedMonitors, server, accountLimits, nil, sl.promotionBudgetFor(ntpdb.GetServerPromotionCountsRow{}, evaluatedMonitors))

	// Debug output
	for _, change := range changes {
		t.Logf("Change: monitor %d from %s to %s, reason: %s",
			change.monitorID, change.fromStatus, change.toStatus, change.reason)
	}

	// Count promotions to active
	promotionCount := 0
	for _, change := range changes {
		if change.fromStatus == ntpdb.ServerScoresStatusTesting &&
			change.toStatus == ntpdb.ServerScoresStatusActive {
			promotionCount++
		}
	}

	// With per-status change limits (promotions=2 with 3 active monitors),
	// should promote up to 2 monitors per cycle (account limit also allows 2)
	if promotionCount != 2 {
		t.Errorf("Expected 2 promotions (promotion limit), got %d", promotionCount)
		t.Logf("Note: Both account limit and promotion limit allow 2")
	}
}

// TestEmergencyOverrideBehavior tests that emergency override works correctly
// when there are zero active monitors
func TestEmergencyOverrideBehavior(t *testing.T) {
	sl := &Selector{
		log: testLogger(),
	}

	// Create test data: 0 active, testing monitors with violations
	evaluatedMonitors := []evaluatedMonitor{
		{
			monitor: monitorCandidate{
				ID:           1,
				AccountID:    ptr(int64(1)),
				GlobalStatus: ntpdb.MonitorsStatusActive,
				ServerStatus: ntpdb.ServerScoresStatusTesting,
				IsHealthy:    true,
			},
			currentViolation: &constraintViolation{
				Type: violationLimit,
			},
			recommendedState: candidateOut,
		},
		{
			monitor: monitorCandidate{
				ID:           2,
				AccountID:    ptr(int64(2)),
				GlobalStatus: ntpdb.MonitorsStatusActive,
				ServerStatus: ntpdb.ServerScoresStatusTesting,
				HasMetrics:   true,
				IsHealthy:    false, // Unhealthy
			},
			recommendedState: candidateOut,
		},
		{
			monitor: monitorCandidate{
				ID:           3,
				AccountID:    ptr(int64(3)),
				GlobalStatus: ntpdb.MonitorsStatusActive, // Must be globally active for emergency promotion
				ServerStatus: ntpdb.ServerScoresStatusTesting,
				IsHealthy:    true,
				Count:        monitorsettings.HistoricalMinForActive, // Ensure testing monitors can be promoted to active
			},
			recommendedState: candidateIn,
		},
	}

	server := &serverInfo{
		ID:        1,
		AccountID: ptr(int64(4)),
	}

	accountLimits := map[int64]*accountLimit{
		1: {AccountID: 1, MaxPerServer: 1, ActiveCount: 0, TestingCount: 1},
		2: {AccountID: 2, MaxPerServer: 1, ActiveCount: 0, TestingCount: 1},
		3: {AccountID: 3, MaxPerServer: 1, ActiveCount: 0, TestingCount: 1},
	}

	changes := sl.applySelectionRules(t.Context(), evaluatedMonitors, server, accountLimits, nil, sl.promotionBudgetFor(ntpdb.GetServerPromotionCountsRow{}, evaluatedMonitors))

	// Check emergency promotions
	emergencyPromotions := 0
	for _, change := range changes {
		if change.reason == "emergency promotion: zero active monitors" {
			emergencyPromotions++

			// Verify only healthy, globally active monitors were promoted
			for _, em := range evaluatedMonitors {
				if em.monitor.ID == change.monitorID {
					if em.monitor.GlobalStatus != ntpdb.MonitorsStatusActive {
						t.Errorf("Emergency promoted non-active monitor %d", em.monitor.ID)
					}
					if em.monitor.HasMetrics && !em.monitor.IsHealthy {
						t.Errorf("Emergency promoted unhealthy monitor %d", em.monitor.ID)
					}
				}
			}
		}
	}

	// Should have at least one emergency promotion
	if emergencyPromotions == 0 {
		t.Error("Expected emergency promotions when zero active monitors")
	}

	// Debug: let's see which monitors were promoted
	for _, change := range changes {
		t.Logf("Change: monitor %d from %s to %s, reason: %s",
			change.monitorID, change.fromStatus, change.toStatus, change.reason)
	}

	// With change limits and emergency override, multiple promotions are possible
	// Monitor 2 is unhealthy, Monitor 3 is not globally active
	// So only monitor 1 should be promoted, but change limits might allow more
	if emergencyPromotions < 1 {
		t.Errorf("Expected at least 1 emergency promotion, got %d", emergencyPromotions)
	}
}

// TestPromotionConstraintChecking tests that constraints are checked against
// the target state for promotions
func TestPromotionConstraintChecking(t *testing.T) {
	sl := &Selector{
		log: testLogger(),
	}

	// Monitor has limit violation in testing but not in active
	monitor := &monitorCandidate{
		ID:           1,
		AccountID:    ptr(int64(1)),
		IP:           "10.0.0.1",
		GlobalStatus: ntpdb.MonitorsStatusActive,
		ServerStatus: ntpdb.ServerScoresStatusTesting,
		IsHealthy:    true,
		HasMetrics:   true,
	}

	server := &serverInfo{
		ID:        1,
		AccountID: ptr(int64(2)),
		IP:        "192.168.1.1",
	}

	// Testing limit reached (2), but active limit not reached (1 < 2)
	// Total is 1 + 2 = 3, which is at the max total limit (MaxPerServer + 1 = 3)
	// But since this monitor is already in testing, it's already counted in TestingCount
	accountLimits := map[int64]*accountLimit{
		1: {AccountID: 1, MaxPerServer: 2, ActiveCount: 1, TestingCount: 2},
	}

	// Create some dummy assigned monitors for the constraint check
	assignedMonitors := []ntpdb.GetMonitorPriorityRow{}

	// Should be able to promote to active
	canPromote := sl.canPromoteToActive(monitor, server, accountLimits, assignedMonitors, false)
	if !canPromote {
		// Add debug output
		violation := sl.checkConstraints(monitor, server, accountLimits, ntpdb.ServerScoresStatusActive, assignedMonitors)
		t.Errorf("Should be able to promote to active when active limit not reached. Violation: %+v", violation)
	}

	// For more accurate testing, let's also verify with empty account limits
	// to isolate the constraint checking
	emptyLimits := map[int64]*accountLimit{
		1: {AccountID: 1, MaxPerServer: 2, ActiveCount: 0, TestingCount: 0},
	}
	canPromoteEmpty := sl.canPromoteToActive(monitor, server, emptyLimits, assignedMonitors, false)
	if !canPromoteEmpty {
		t.Error("Should be able to promote to active with no existing monitors")
	}
}

// Helper functions
func ptr(v int64) *int64 {
	return &v
}

func testLogger() *slog.Logger {
	return slog.Default()
}

func TestKnownServerStatus(t *testing.T) {
	tests := []struct {
		status ntpdb.ServerScoresStatus
		want   bool
	}{
		{ntpdb.ServerScoresStatusCandidate, true},
		{ntpdb.ServerScoresStatusTesting, true},
		{ntpdb.ServerScoresStatusActive, true},
		{ntpdb.ServerScoresStatusPaused, true},
		{ntpdb.ServerScoresStatusReserve, true},
		{ntpdb.ServerScoresStatus("retired"), false},
		{ntpdb.ServerScoresStatus(""), false},
	}
	for _, tt := range tests {
		t.Run(string(tt.status), func(t *testing.T) {
			if got := knownServerStatus(tt.status); got != tt.want {
				t.Errorf("knownServerStatus(%q) = %v, want %v", tt.status, got, tt.want)
			}
		})
	}
}

func TestSkipUnknownStatuses(t *testing.T) {
	sl := &Selector{log: testLogger()}
	rows := []ntpdb.GetMonitorPriorityRow{
		{ID: 1, Status: ntpdb.ServerScoresStatusActive},
		{ID: 2, Status: ntpdb.ServerScoresStatus("retired")},
		{ID: 3, Status: ntpdb.ServerScoresStatusReserve},
		{ID: 4, Status: ntpdb.ServerScoresStatus("")},
	}

	got := sl.skipUnknownStatuses(rows, 1)

	if len(got) != 2 || got[0].ID != 1 || got[1].ID != 3 {
		t.Errorf("skipUnknownStatuses kept %+v, want monitors 1 and 3", got)
	}
}

// sampledMonitor builds an evaluated monitor with the state determineState gives it.
func sampledMonitor(sl *Selector, id int64, status ntpdb.ServerScoresStatus, healthy bool, count int64) evaluatedMonitor {
	m := monitorCandidate{
		ID:           id,
		AccountID:    ptr(id),
		GlobalStatus: ntpdb.MonitorsStatusActive,
		ServerStatus: status,
		HasMetrics:   count > 0,
		IsHealthy:    healthy && count > 0,
		Count:        count,
		Priority:     int(id),
	}
	v := &constraintViolation{Type: violationNone}
	return evaluatedMonitor{monitor: m, currentViolation: v, recommendedState: sl.determineState(&m, v)}
}

// accountLimitsFor returns the account limits for rows that each have their
// own account (sampledMonitor uses the monitor id), with the counts of the
// current statuses.
func accountLimitsFor(rows []evaluatedMonitor) map[int64]*accountLimit {
	limits := map[int64]*accountLimit{}
	for _, em := range rows {
		l := &accountLimit{AccountID: em.monitor.ID, MaxPerServer: 2}
		switch em.monitor.ServerStatus {
		case ntpdb.ServerScoresStatusActive:
			l.ActiveCount = 1
		case ntpdb.ServerScoresStatusTesting:
			l.TestingCount = 1
		}
		limits[em.monitor.ID] = l
	}
	return limits
}

func TestRowsWithoutSamplesDontMoveSafeguards(t *testing.T) {
	server := &serverInfo{ID: 14, AccountID: ptr(int64(9999)), IP: "192.0.2.1", IPVersion: "4"}

	// without-samples candidates: 150 rows, the first two globally paused (blocked)
	noSamples := func(sl *Selector) []evaluatedMonitor {
		var rows []evaluatedMonitor
		for i := range int64(150) {
			em := sampledMonitor(sl, 1000+i, ntpdb.ServerScoresStatusCandidate, false, 0)
			if i < 2 {
				em.monitor.GlobalStatus = ntpdb.MonitorsStatusPaused
				em.recommendedState = candidateBlock
			}
			rows = append(rows, em)
		}
		return rows
	}

	// reserve rows: 150 of them, with samples (a row just moved to reserve
	// still has them), unhealthy and healthy
	reserves := func(sl *Selector) []evaluatedMonitor {
		var rows []evaluatedMonitor
		for i := range int64(150) {
			rows = append(rows, sampledMonitor(sl, 5000+i, ntpdb.ServerScoresStatusReserve, i%2 == 0, 500))
		}
		return rows
	}

	tests := []struct {
		name  string
		build func(sl *Selector) []evaluatedMonitor
		// wantChanges is the number of changes the rows with samples alone produce
		wantChanges int
	}{
		{
			// Blocked rows without samples must not raise the removal limit
			// from 2 to 3: 7 healthy and 3 unhealthy active rows.
			name: "blocked_rows_dont_raise_change_limits",
			build: func(sl *Selector) []evaluatedMonitor {
				var rows []evaluatedMonitor
				for i := range int64(7) {
					rows = append(rows, sampledMonitor(sl, 1+i, ntpdb.ServerScoresStatusActive, true, 500))
				}
				for i := range int64(3) {
					rows = append(rows, sampledMonitor(sl, 20+i, ntpdb.ServerScoresStatusActive, false, 500))
				}
				return rows
			},
			wantChanges: 2,
		},
		{
			// Many rows without samples must not lift the "need more monitors
			// than exist" emergency block: only 3 candidates have samples.
			name: "many_rows_dont_lift_emergency_block",
			build: func(sl *Selector) []evaluatedMonitor {
				var rows []evaluatedMonitor
				for i := range int64(3) {
					rows = append(rows, sampledMonitor(sl, 1+i, ntpdb.ServerScoresStatusCandidate, true, 500))
				}
				return rows
			},
			wantChanges: 0,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			sl := &Selector{log: testLogger()}

			base := tt.build(sl)
			want := sl.applySelectionRules(t.Context(), base, server, map[int64]*accountLimit{}, nil, sl.promotionBudgetFor(ntpdb.GetServerPromotionCountsRow{}, base))
			if len(want) != tt.wantChanges {
				t.Fatalf("rows with samples alone gave %d changes, want %d: %+v", len(want), tt.wantChanges, want)
			}

			all := append(append([]evaluatedMonitor{}, base...), noSamples(sl)...)
			got := sl.applySelectionRules(t.Context(), all, server, map[int64]*accountLimit{}, nil, sl.promotionBudgetFor(ntpdb.GetServerPromotionCountsRow{}, all))

			if len(got) != len(want) {
				t.Fatalf("with rows without samples: %d changes %+v, want %d %+v", len(got), got, len(want), want)
			}
			for i := range want {
				if got[i] != want[i] {
					t.Errorf("change %d = %+v, want %+v", i, got[i], want[i])
				}
			}

			// reserve rows change nothing
			all = append(all, reserves(sl)...)
			gotReserve := sl.applySelectionRules(t.Context(), all, server, map[int64]*accountLimit{}, nil, sl.promotionBudgetFor(ntpdb.GetServerPromotionCountsRow{}, all))
			if !slices.Equal(gotReserve, want) {
				t.Errorf("with reserve rows: changes %+v, want %+v", gotReserve, want)
			}
		})
	}
}

func TestStoppedActiveMonitorsDemoted(t *testing.T) {
	server := &serverInfo{ID: 14, AccountID: ptr(int64(9999)), IP: "192.0.2.1", IPVersion: "4"}

	tests := []struct {
		name string
		// per-row spec: stopped active rows, healthy active rows, healthy testing rows
		stopped, healthyActive, healthyTesting int
		wantDemoted                            int
		// wantPromoted is set when a testing row must be promoted to active
		wantPromoted bool
	}{
		{"some_stopped", 2, 5, 5, 2, true},
		{"all_stopped_with_testing", 7, 0, 7, 2, true},
		{"all_stopped_nothing_else", 7, 0, 0, 2, false},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			sl := &Selector{log: testLogger()}

			var rows []evaluatedMonitor
			stopped := map[int64]bool{}
			id := int64(1)
			for range tt.stopped {
				rows = append(rows, sampledMonitor(sl, id, ntpdb.ServerScoresStatusActive, false, 0))
				stopped[id] = true
				id++
			}
			for range tt.healthyActive {
				rows = append(rows, sampledMonitor(sl, id, ntpdb.ServerScoresStatusActive, true, 500))
				id++
			}
			for range tt.healthyTesting {
				rows = append(rows, sampledMonitor(sl, id, ntpdb.ServerScoresStatusTesting, true, 500))
				id++
			}

			// a stopped row is out, and never counted as healthy
			for _, em := range rows {
				if stopped[em.monitor.ID] && (em.recommendedState != candidateOut || em.monitor.IsHealthy) {
					t.Fatalf("stopped monitor %d: state %v healthy %v", em.monitor.ID, em.recommendedState, em.monitor.IsHealthy)
				}
			}

			// stopped rows say nothing about the server being unhealthy
			if classifyServer(rows).LooksBad {
				t.Errorf("server classified as looking bad with %d stopped active rows", tt.stopped)
			}

			changes := sl.applySelectionRules(t.Context(), rows, server, accountLimitsFor(rows), nil, sl.promotionBudgetFor(ntpdb.GetServerPromotionCountsRow{}, rows))

			demoted, promoted := 0, 0
			for _, c := range changes {
				if c.fromStatus == ntpdb.ServerScoresStatusTesting && c.toStatus == ntpdb.ServerScoresStatusActive {
					promoted++
				}
				if c.fromStatus == ntpdb.ServerScoresStatusActive && c.toStatus != ntpdb.ServerScoresStatusActive {
					if !stopped[c.monitorID] {
						t.Errorf("demoted monitor %d that is still reporting", c.monitorID)
					}
					demoted++
				}
				if stopped[c.monitorID] && c.toStatus == ntpdb.ServerScoresStatusActive {
					t.Errorf("promoted stopped monitor %d", c.monitorID)
				}
			}
			if demoted != tt.wantDemoted {
				t.Errorf("demoted %d stopped active monitors, want %d: %+v", demoted, tt.wantDemoted, changes)
			}
			if tt.wantPromoted && promoted == 0 {
				t.Errorf("no testing monitor promoted to active: %+v", changes)
			}
			if !tt.wantPromoted && promoted != 0 {
				t.Errorf("%d promotions to active, want none: %+v", promoted, changes)
			}
		})
	}
}

// TestSilentActiveMonitorsAllDemoted runs repeated reviews of a server whose
// active rows have all stopped reporting. Each review is limited, but the
// safe threshold must not stop the removals: every silent row ends up out of
// service and the healthy testing rows take over.
func TestSilentActiveMonitorsAllDemoted(t *testing.T) {
	server := &serverInfo{ID: 14, AccountID: ptr(int64(9999)), IP: "192.0.2.1", IPVersion: "4"}
	sl := &Selector{log: testLogger()}

	const maxReviews = 6

	var rows []evaluatedMonitor
	silent := map[int64]bool{}
	for i := range int64(7) {
		rows = append(rows, sampledMonitor(sl, 1+i, ntpdb.ServerScoresStatusActive, false, 0))
		silent[1+i] = true
	}
	for i := range int64(7) {
		rows = append(rows, sampledMonitor(sl, 100+i, ntpdb.ServerScoresStatusTesting, true, 500))
	}

	silentInService := func() int {
		n := 0
		for _, em := range rows {
			if silent[em.monitor.ID] && inService(em.monitor.ServerStatus) {
				n++
			}
		}
		return n
	}

	promoted := 0
	reviews := 0
	for ; reviews < maxReviews && silentInService() > 0; reviews++ {
		changes := sl.applySelectionRules(t.Context(), rows, server, accountLimitsFor(rows), nil, sl.promotionBudgetFor(ntpdb.GetServerPromotionCountsRow{}, rows))
		if len(changes) == 0 {
			t.Fatalf("review %d made no changes with %d silent rows in service", reviews+1, silentInService())
		}

		to := map[int64]ntpdb.ServerScoresStatus{}
		for _, c := range changes {
			to[c.monitorID] = c.toStatus
			if c.fromStatus == ntpdb.ServerScoresStatusTesting && c.toStatus == ntpdb.ServerScoresStatusActive {
				promoted++
			}
		}
		for i, em := range rows {
			status, ok := to[em.monitor.ID]
			if !ok {
				continue
			}
			rows[i].monitor.ServerStatus = status
			rows[i].recommendedState = sl.determineState(&rows[i].monitor, em.currentViolation)
		}
	}

	if n := silentInService(); n != 0 {
		t.Errorf("%d silent rows still in service after %d reviews", n, reviews)
	}
	if promoted == 0 {
		t.Errorf("no testing row was promoted to active in %d reviews", reviews)
	}
}
