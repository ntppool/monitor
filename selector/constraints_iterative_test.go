package selector

import (
	"encoding/json"
	"testing"

	"github.com/jackc/pgx/v5/pgtype"
	"go.ntppool.org/monitor/ntpdb"
)

func TestCheckAccountConstraintsIterative(t *testing.T) {
	sl := &Selector{}
	server := &serverInfo{ID: 1982, AccountID: nil}

	// Helper to create account flags JSON
	createAccountFlags := func(limit int) *json.RawMessage {
		flags := accountFlags{MonitorsPerServerLimit: limit, MonitorEnabled: true}
		data, _ := json.Marshal(flags)
		rawMessage := json.RawMessage(data)
		return &rawMessage
	}

	// Helper to create monitor row
	createMonitor := func(id int64, accountID int64, status ntpdb.ServerScoresStatus, priority int32, limit int) ntpdb.GetMonitorPriorityRow {
		return ntpdb.GetMonitorPriorityRow{
			ID:              id,
			AccountID:       pgtype.Int8{Int64: accountID, Valid: true},
			Status:          ntpdb.NullServerScoresStatus{ServerScoresStatus: status, Valid: true},
			MonitorPriority: priority,
			AccountFlags:    createAccountFlags(limit),
		}
	}

	tests := []struct {
		name                string
		monitors            []ntpdb.GetMonitorPriorityRow
		expectedViolations  map[int64]bool // monitor ID -> should have violation
		expectedViolationID []int64        // monitors that should have violations
	}{
		{
			name: "no_violations_under_limit",
			monitors: []ntpdb.GetMonitorPriorityRow{
				createMonitor(1, 100, ntpdb.ServerScoresStatusActive, 10, 2),  // account 100, active, priority 10
				createMonitor(2, 100, ntpdb.ServerScoresStatusActive, 20, 2),  // account 100, active, priority 20
				createMonitor(3, 100, ntpdb.ServerScoresStatusTesting, 30, 2), // account 100, testing, priority 30
			},
			expectedViolations: map[int64]bool{
				1: false, 2: false, 3: false,
			},
		},
		{
			name: "active_monitors_exceed_limit_worst_performer_flagged",
			monitors: []ntpdb.GetMonitorPriorityRow{
				createMonitor(1, 100, ntpdb.ServerScoresStatusActive, 10, 2), // account 100, active, priority 10 (best)
				createMonitor(2, 100, ntpdb.ServerScoresStatusActive, 20, 2), // account 100, active, priority 20 (middle)
				createMonitor(3, 100, ntpdb.ServerScoresStatusActive, 30, 2), // account 100, active, priority 30 (worst) - should be flagged
			},
			expectedViolations: map[int64]bool{
				1: false, 2: false, 3: true,
			},
		},
		{
			name: "testing_monitors_exceed_limit_worst_performer_flagged",
			monitors: []ntpdb.GetMonitorPriorityRow{
				createMonitor(1, 100, ntpdb.ServerScoresStatusTesting, 10, 2), // account 100, testing, priority 10 (best)
				createMonitor(2, 100, ntpdb.ServerScoresStatusTesting, 20, 2), // account 100, testing, priority 20 (middle)
				createMonitor(3, 100, ntpdb.ServerScoresStatusTesting, 30, 2), // account 100, testing, priority 30 (middle)
				createMonitor(4, 100, ntpdb.ServerScoresStatusTesting, 40, 2), // account 100, testing, priority 40 (worst) - should be flagged
			},
			expectedViolations: map[int64]bool{
				1: false, 2: false, 3: false, 4: true,
			},
		},
		{
			name: "separate_limits_per_category",
			monitors: []ntpdb.GetMonitorPriorityRow{
				// Account 100 - 3 active (limit 2) + 3 testing (limit 3)
				createMonitor(1, 100, ntpdb.ServerScoresStatusActive, 10, 2),  // active, best - keep
				createMonitor(2, 100, ntpdb.ServerScoresStatusActive, 20, 2),  // active, middle - keep
				createMonitor(3, 100, ntpdb.ServerScoresStatusActive, 30, 2),  // active, worst - flag
				createMonitor(4, 100, ntpdb.ServerScoresStatusTesting, 15, 2), // testing, best - keep
				createMonitor(5, 100, ntpdb.ServerScoresStatusTesting, 25, 2), // testing, middle - keep
				createMonitor(6, 100, ntpdb.ServerScoresStatusTesting, 35, 2), // testing, worst - keep (under testing limit of 3)
			},
			expectedViolations: map[int64]bool{
				1: false, 2: false, 3: true, // only worst active flagged
				4: false, 5: false, 6: false, // all testing monitors OK
			},
		},
		{
			name: "candidates_are_exempt",
			monitors: []ntpdb.GetMonitorPriorityRow{
				createMonitor(1, 100, ntpdb.ServerScoresStatusActive, 10, 2),    // active
				createMonitor(2, 100, ntpdb.ServerScoresStatusActive, 20, 2),    // active
				createMonitor(3, 100, ntpdb.ServerScoresStatusActive, 30, 2),    // active - should be flagged
				createMonitor(4, 100, ntpdb.ServerScoresStatusCandidate, 40, 2), // candidate - exempt
				createMonitor(5, 100, ntpdb.ServerScoresStatusCandidate, 50, 2), // candidate - exempt
			},
			expectedViolations: map[int64]bool{
				1: false, 2: false, 3: true, // worst active flagged
				4: false, 5: false, // candidates exempt
			},
		},
		{
			name: "multiple_accounts_separate_limits",
			monitors: []ntpdb.GetMonitorPriorityRow{
				// Account 100 - 3 active (limit 2)
				createMonitor(1, 100, ntpdb.ServerScoresStatusActive, 10, 2), // account 100, keep
				createMonitor(2, 100, ntpdb.ServerScoresStatusActive, 20, 2), // account 100, keep
				createMonitor(3, 100, ntpdb.ServerScoresStatusActive, 30, 2), // account 100, flag
				// Account 200 - 2 active (limit 2)
				createMonitor(4, 200, ntpdb.ServerScoresStatusActive, 15, 2), // account 200, keep
				createMonitor(5, 200, ntpdb.ServerScoresStatusActive, 25, 2), // account 200, keep
			},
			expectedViolations: map[int64]bool{
				1: false, 2: false, 3: true, // only account 100's worst flagged
				4: false, 5: false, // account 200 is under limit
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			violations := sl.checkAccountConstraintsIterative(tt.monitors, server)

			// Check that expected violations match
			for monitorID, shouldHaveViolation := range tt.expectedViolations {
				_, hasViolation := violations[monitorID]
				if shouldHaveViolation && !hasViolation {
					t.Errorf("monitor %d should have violation but doesn't", monitorID)
				}
				if !shouldHaveViolation && hasViolation {
					t.Errorf("monitor %d should not have violation but does: %v", monitorID, violations[monitorID])
				}
			}

			// Verify violation details
			for monitorID, violation := range violations {
				if violation.Type != violationLimit {
					t.Errorf("monitor %d has wrong violation type: got %v, want %v", monitorID, violation.Type, violationLimit)
				}
				if violation.Details == "" {
					t.Errorf("monitor %d violation missing details", monitorID)
				}
			}
		})
	}
}

func TestCheckAccountConstraintsIterative_EdgeCases(t *testing.T) {
	sl := &Selector{}
	server := &serverInfo{ID: 1982, AccountID: nil}

	// Helper to create account flags JSON
	createAccountFlags := func(limit int) *json.RawMessage {
		flags := accountFlags{MonitorsPerServerLimit: limit, MonitorEnabled: true}
		data, _ := json.Marshal(flags)
		rawMessage := json.RawMessage(data)
		return &rawMessage
	}

	t.Run("empty_monitors_list", func(t *testing.T) {
		violations := sl.checkAccountConstraintsIterative([]ntpdb.GetMonitorPriorityRow{}, server)
		if len(violations) != 0 {
			t.Errorf("expected no violations for empty list, got %d", len(violations))
		}
	})

	t.Run("monitors_without_accounts", func(t *testing.T) {
		monitors := []ntpdb.GetMonitorPriorityRow{
			{
				ID:              1,
				AccountID:       pgtype.Int8{Valid: false}, // no account
				Status:          ntpdb.NullServerScoresStatus{ServerScoresStatus: ntpdb.ServerScoresStatusActive, Valid: true},
				MonitorPriority: 10,
				AccountFlags:    createAccountFlags(2),
			},
		}
		violations := sl.checkAccountConstraintsIterative(monitors, server)
		if len(violations) != 0 {
			t.Errorf("expected no violations for monitors without accounts, got %d", len(violations))
		}
	})

	t.Run("monitors_without_status", func(t *testing.T) {
		monitors := []ntpdb.GetMonitorPriorityRow{
			{
				ID:              1,
				AccountID:       pgtype.Int8{Int64: 100, Valid: true},
				Status:          ntpdb.NullServerScoresStatus{Valid: false}, // no status
				MonitorPriority: 10,
				AccountFlags:    createAccountFlags(2),
			},
		}
		violations := sl.checkAccountConstraintsIterative(monitors, server)
		if len(violations) != 0 {
			t.Errorf("expected no violations for monitors without status, got %d", len(violations))
		}
	})

	t.Run("default_limit_used_when_no_flags", func(t *testing.T) {
		monitors := []ntpdb.GetMonitorPriorityRow{
			{
				ID:              1,
				AccountID:       pgtype.Int8{Int64: 100, Valid: true},
				Status:          ntpdb.NullServerScoresStatus{ServerScoresStatus: ntpdb.ServerScoresStatusActive, Valid: true},
				MonitorPriority: 10,
				AccountFlags:    nil, // no flags
			},
			{
				ID:              2,
				AccountID:       pgtype.Int8{Int64: 100, Valid: true},
				Status:          ntpdb.NullServerScoresStatus{ServerScoresStatus: ntpdb.ServerScoresStatusActive, Valid: true},
				MonitorPriority: 20,
				AccountFlags:    nil, // no flags
			},
			{
				ID:              3,
				AccountID:       pgtype.Int8{Int64: 100, Valid: true},
				Status:          ntpdb.NullServerScoresStatus{ServerScoresStatus: ntpdb.ServerScoresStatusActive, Valid: true},
				MonitorPriority: 30,
				AccountFlags:    nil, // no flags
			},
		}
		violations := sl.checkAccountConstraintsIterative(monitors, server)

		// Should use default limit of 2, so monitor 3 (worst) should be flagged
		if len(violations) != 1 {
			t.Errorf("expected 1 violation with default limit, got %d", len(violations))
		}
		if _, hasViolation := violations[3]; !hasViolation {
			t.Errorf("monitor 3 should be flagged when using default limit")
		}
	})
}
