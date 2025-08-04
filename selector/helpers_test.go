package selector

import (
	"log/slog"
	"testing"

	"go.ntppool.org/monitor/ntpdb"
)

func TestGetEmergencyReason(t *testing.T) {
	tests := []struct {
		name        string
		baseReason  string
		toStatus    ntpdb.ServerScoresStatus
		isBootstrap bool
		expected    string
	}{
		{
			name:        "emergency_promotion_to_active",
			baseReason:  "promotion to active",
			toStatus:    ntpdb.ServerScoresStatusActive,
			isBootstrap: false,
			expected:    "emergency promotion: zero active monitors",
		},
		{
			name:        "emergency_promotion_to_testing",
			baseReason:  "candidate to testing",
			toStatus:    ntpdb.ServerScoresStatusTesting,
			isBootstrap: false,
			expected:    "emergency promotion to testing: zero active monitors",
		},
		{
			name:        "bootstrap_emergency_promotion_to_active",
			baseReason:  "bootstrap promotion",
			toStatus:    ntpdb.ServerScoresStatusActive,
			isBootstrap: true,
			expected:    "bootstrap emergency promotion: zero active monitors",
		},
		{
			name:        "bootstrap_emergency_promotion_to_testing",
			baseReason:  "bootstrap promotion",
			toStatus:    ntpdb.ServerScoresStatusTesting,
			isBootstrap: true,
			expected:    "bootstrap emergency promotion to testing: zero active monitors",
		},
		{
			name:        "candidate_status_returns_base",
			baseReason:  "some reason",
			toStatus:    ntpdb.ServerScoresStatusCandidate,
			isBootstrap: false,
			expected:    "some reason",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := getEmergencyReason(tt.baseReason, tt.toStatus, tt.isBootstrap)
			if result != tt.expected {
				t.Errorf("getEmergencyReason() = %q, expected %q", result, tt.expected)
			}
		})
	}
}

func TestFilterMonitorsByGlobalStatus(t *testing.T) {
	// Create test monitors with different global statuses
	monitors := []evaluatedMonitor{
		{monitor: monitorCandidate{ID: 1, GlobalStatus: ntpdb.MonitorsStatusActive}},
		{monitor: monitorCandidate{ID: 2, GlobalStatus: ntpdb.MonitorsStatusTesting}},
		{monitor: monitorCandidate{ID: 3, GlobalStatus: ntpdb.MonitorsStatusActive}},
		{monitor: monitorCandidate{ID: 4, GlobalStatus: ntpdb.MonitorsStatusPaused}},
	}

	// Test filtering by active status
	activeMonitors := filterMonitorsByGlobalStatus(monitors, ntpdb.MonitorsStatusActive)
	if len(activeMonitors) != 2 {
		t.Errorf("Expected 2 active monitors, got %d", len(activeMonitors))
	}
	if activeMonitors[0].monitor.ID != 1 || activeMonitors[1].monitor.ID != 3 {
		t.Errorf("Expected monitors 1 and 3, got %d and %d",
			activeMonitors[0].monitor.ID, activeMonitors[1].monitor.ID)
	}

	// Test filtering by testing status
	testingMonitors := filterMonitorsByGlobalStatus(monitors, ntpdb.MonitorsStatusTesting)
	if len(testingMonitors) != 1 {
		t.Errorf("Expected 1 testing monitor, got %d", len(testingMonitors))
	}
	if testingMonitors[0].monitor.ID != 2 {
		t.Errorf("Expected monitor 2, got %d", testingMonitors[0].monitor.ID)
	}

	// Test filtering by paused status
	pausedMonitors := filterMonitorsByGlobalStatus(monitors, ntpdb.MonitorsStatusPaused)
	if len(pausedMonitors) != 1 {
		t.Errorf("Expected 1 paused monitor, got %d", len(pausedMonitors))
	}
}

func TestFilterBootstrapCandidates(t *testing.T) {
	monitors := []evaluatedMonitor{
		{monitor: monitorCandidate{ID: 1, GlobalStatus: ntpdb.MonitorsStatusActive, IsHealthy: true}},
		{monitor: monitorCandidate{ID: 2, GlobalStatus: ntpdb.MonitorsStatusActive, IsHealthy: false}},
		{monitor: monitorCandidate{ID: 3, GlobalStatus: ntpdb.MonitorsStatusTesting, IsHealthy: true}},
		{monitor: monitorCandidate{ID: 4, GlobalStatus: ntpdb.MonitorsStatusTesting, IsHealthy: false}},
		{monitor: monitorCandidate{ID: 5, GlobalStatus: ntpdb.MonitorsStatusPaused, IsHealthy: true}}, // Should be filtered out
	}

	healthy, other := filterBootstrapCandidates(monitors)

	// Should have 2 healthy monitors (IDs 1 and 3)
	if len(healthy) != 2 {
		t.Errorf("Expected 2 healthy monitors, got %d", len(healthy))
	}

	// Should have 2 other monitors (IDs 2 and 4)
	if len(other) != 2 {
		t.Errorf("Expected 2 other monitors, got %d", len(other))
	}

	// Verify healthy monitors are correct
	healthyIDs := []uint32{healthy[0].monitor.ID, healthy[1].monitor.ID}
	expectedHealthy := []uint32{1, 3}
	for i, expectedID := range expectedHealthy {
		if healthyIDs[i] != expectedID {
			t.Errorf("Expected healthy monitor %d at position %d, got %d", expectedID, i, healthyIDs[i])
		}
	}

	// Verify other monitors are correct
	otherIDs := []uint32{other[0].monitor.ID, other[1].monitor.ID}
	expectedOther := []uint32{2, 4}
	for i, expectedID := range expectedOther {
		if otherIDs[i] != expectedID {
			t.Errorf("Expected other monitor %d at position %d, got %d", expectedID, i, otherIDs[i])
		}
	}
}

func TestAttemptPromotion(t *testing.T) {
	// Create a minimal selector for testing
	sl := &Selector{
		log: slog.Default(),
	}

	// Mock data
	monitor := &monitorCandidate{
		ID:           1,
		GlobalStatus: ntpdb.MonitorsStatusActive,
		IsHealthy:    true,
		AccountID:    uint32Ptr(100),
	}

	server := &serverInfo{
		ID:        1,
		IP:        "192.168.1.1",
		AccountID: uint32Ptr(200), // Different account
		IPVersion: "4",
	}

	workingLimits := map[uint32]*accountLimit{
		100: {
			AccountID:    100,
			MaxPerServer: 2,
			ActiveCount:  0,
			TestingCount: 0,
		},
	}

	tests := []struct {
		name     string
		req      promotionRequest
		expected promotionResult
	}{
		{
			name: "successful_candidate_to_testing",
			req: promotionRequest{
				monitor:           monitor,
				server:            server,
				workingLimits:     workingLimits,
				assignedMonitors:  []ntpdb.GetMonitorPriorityRow{},
				emergencyOverride: false,
				fromStatus:        ntpdb.ServerScoresStatusCandidate,
				toStatus:          ntpdb.ServerScoresStatusTesting,
				baseReason:        "candidate to testing",
				emergencyReason:   "emergency promotion to testing: zero active monitors",
			},
			expected: promotionResult{
				success:          true,
				activeIncrement:  0,
				testingIncrement: 1,
			},
		},
		{
			name: "successful_testing_to_active",
			req: promotionRequest{
				monitor:           monitor,
				server:            server,
				workingLimits:     workingLimits,
				assignedMonitors:  []ntpdb.GetMonitorPriorityRow{},
				emergencyOverride: false,
				fromStatus:        ntpdb.ServerScoresStatusTesting,
				toStatus:          ntpdb.ServerScoresStatusActive,
				baseReason:        "promotion to active",
				emergencyReason:   "emergency promotion: zero active monitors",
			},
			expected: promotionResult{
				success:          true,
				activeIncrement:  1,
				testingIncrement: -1,
			},
		},
		{
			name: "emergency_override_uses_emergency_reason",
			req: promotionRequest{
				monitor:           monitor,
				server:            server,
				workingLimits:     workingLimits,
				assignedMonitors:  []ntpdb.GetMonitorPriorityRow{},
				emergencyOverride: true,
				fromStatus:        ntpdb.ServerScoresStatusCandidate,
				toStatus:          ntpdb.ServerScoresStatusTesting,
				baseReason:        "candidate to testing",
				emergencyReason:   "emergency promotion to testing: zero active monitors",
			},
			expected: promotionResult{
				success:          true,
				activeIncrement:  0,
				testingIncrement: 1,
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := sl.attemptPromotion(tt.req)

			if result.success != tt.expected.success {
				t.Errorf("Expected success=%v, got %v", tt.expected.success, result.success)
			}

			if result.success {
				if result.activeIncrement != tt.expected.activeIncrement {
					t.Errorf("Expected activeIncrement=%d, got %d",
						tt.expected.activeIncrement, result.activeIncrement)
				}

				if result.testingIncrement != tt.expected.testingIncrement {
					t.Errorf("Expected testingIncrement=%d, got %d",
						tt.expected.testingIncrement, result.testingIncrement)
				}

				if result.change == nil {
					t.Error("Expected change to be non-nil for successful promotion")
				} else {
					// Check reason selection
					expectedReason := tt.req.baseReason
					if tt.req.emergencyOverride {
						expectedReason = tt.req.emergencyReason
					}
					if result.change.reason != expectedReason {
						t.Errorf("Expected reason=%q, got %q", expectedReason, result.change.reason)
					}

					// Check status change
					if result.change.fromStatus != tt.req.fromStatus {
						t.Errorf("Expected fromStatus=%v, got %v",
							tt.req.fromStatus, result.change.fromStatus)
					}
					if result.change.toStatus != tt.req.toStatus {
						t.Errorf("Expected toStatus=%v, got %v",
							tt.req.toStatus, result.change.toStatus)
					}
				}
			}
		})
	}
}

// Helper function for creating uint32 pointers
func uint32Ptr(val uint32) *uint32 {
	return &val
}
