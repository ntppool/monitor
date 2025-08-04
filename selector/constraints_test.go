package selector

import (
	"database/sql"
	"log/slog"
	"testing"

	"go.ntppool.org/monitor/ntpdb"
)

func TestCheckNetworkConstraint(t *testing.T) {
	sl := &Selector{
		log: slog.Default(),
	}

	tests := []struct {
		name      string
		monitorIP string
		serverIP  string
		wantErr   bool
	}{
		{
			name:      "same_ipv4_24_subnet",
			monitorIP: "192.168.1.10",
			serverIP:  "192.168.1.20",
			wantErr:   true,
		},
		{
			name:      "different_ipv4_24_subnet",
			monitorIP: "192.168.1.10",
			serverIP:  "192.168.2.20",
			wantErr:   false,
		},
		{
			name:      "same_ipv6_48_subnet",
			monitorIP: "2001:db8:1234::1",
			serverIP:  "2001:db8:1234::2",
			wantErr:   true,
		},
		{
			name:      "different_ipv6_48_subnet",
			monitorIP: "2001:db8:1234::1",
			serverIP:  "2001:db8:5678::2",
			wantErr:   false,
		},
		{
			name:      "ipv4_and_ipv6_different_families",
			monitorIP: "192.168.1.10",
			serverIP:  "2001:db8:1234::1",
			wantErr:   false,
		},
		{
			name:      "empty_monitor_ip",
			monitorIP: "",
			serverIP:  "192.168.1.20",
			wantErr:   false,
		},
		{
			name:      "empty_server_ip",
			monitorIP: "192.168.1.10",
			serverIP:  "",
			wantErr:   false,
		},
		{
			name:      "invalid_monitor_ip",
			monitorIP: "not-an-ip",
			serverIP:  "192.168.1.20",
			wantErr:   true,
		},
		{
			name:      "invalid_server_ip",
			monitorIP: "192.168.1.10",
			serverIP:  "not-an-ip",
			wantErr:   true,
		},
		{
			name:      "same_exact_ip",
			monitorIP: "192.168.1.10",
			serverIP:  "192.168.1.10",
			wantErr:   true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := sl.checkNetworkConstraint(tt.monitorIP, tt.serverIP)
			if (err != nil) != tt.wantErr {
				t.Errorf("checkNetworkConstraint() error = %v, wantErr %v", err, tt.wantErr)
			}
		})
	}
}

// contains is a helper function to check if a string contains a substring
func contains(s, substr string) bool {
	return len(substr) > 0 && len(s) >= len(substr) && s[:len(substr)] == substr || len(s) > len(substr) && contains(s[1:], substr)
}

func TestCheckNetworkDiversityConstraint(t *testing.T) {
	sl := &Selector{}

	tests := []struct {
		name             string
		monitorID        uint32
		monitorIP        string
		existingMonitors []ntpdb.GetMonitorPriorityRow
		targetState      ntpdb.ServerScoresStatus
		wantErr          bool
		errContains      string
	}{
		{
			name:             "no_existing_monitors",
			monitorID:        1,
			monitorIP:        "192.168.1.10",
			existingMonitors: []ntpdb.GetMonitorPriorityRow{},
			targetState:      ntpdb.ServerScoresStatusActive,
			wantErr:          false,
		},
		{
			name:      "ipv4_no_diversity_conflict",
			monitorID: 1,
			monitorIP: "192.168.1.10",
			existingMonitors: []ntpdb.GetMonitorPriorityRow{
				{
					ID:        2,
					MonitorIp: sql.NullString{String: "10.0.0.1", Valid: true},
					Status:    ntpdb.NullServerScoresStatus{ServerScoresStatus: ntpdb.ServerScoresStatusActive, Valid: true},
				},
			},
			targetState: ntpdb.ServerScoresStatusActive,
			wantErr:     false,
		},
		{
			name:      "ipv4_diversity_conflict_same_20",
			monitorID: 1,
			monitorIP: "192.168.1.10",
			existingMonitors: []ntpdb.GetMonitorPriorityRow{
				{
					ID:        2,
					MonitorIp: sql.NullString{String: "192.168.2.20", Valid: true},
					Status:    ntpdb.NullServerScoresStatus{ServerScoresStatus: ntpdb.ServerScoresStatusActive, Valid: true},
				},
				{
					ID:        3,
					MonitorIp: sql.NullString{String: "192.168.3.30", Valid: true},
					Status:    ntpdb.NullServerScoresStatus{ServerScoresStatus: ntpdb.ServerScoresStatusActive, Valid: true},
				},
			},
			targetState: ntpdb.ServerScoresStatusActive,
			wantErr:     true,
			errContains: "monitor would conflict",
		},
		{
			name:      "ipv6_no_diversity_conflict",
			monitorID: 1,
			monitorIP: "2001:db8:1234::1",
			existingMonitors: []ntpdb.GetMonitorPriorityRow{
				{
					ID:        2,
					MonitorIp: sql.NullString{String: "2001:db8:5678::1", Valid: true},
					Status:    ntpdb.NullServerScoresStatus{ServerScoresStatus: ntpdb.ServerScoresStatusActive, Valid: true},
				},
			},
			targetState: ntpdb.ServerScoresStatusActive,
			wantErr:     false,
		},
		{
			name:      "ipv6_diversity_conflict_same_44",
			monitorID: 1,
			monitorIP: "2001:db8:1234::1",
			existingMonitors: []ntpdb.GetMonitorPriorityRow{
				{
					ID:        2,
					MonitorIp: sql.NullString{String: "2001:db8:1234:1::1", Valid: true},
					Status:    ntpdb.NullServerScoresStatus{ServerScoresStatus: ntpdb.ServerScoresStatusActive, Valid: true},
				},
				{
					ID:        3,
					MonitorIp: sql.NullString{String: "2001:db8:1234:2::1", Valid: true},
					Status:    ntpdb.NullServerScoresStatus{ServerScoresStatus: ntpdb.ServerScoresStatusActive, Valid: true},
				},
			},
			targetState: ntpdb.ServerScoresStatusActive,
			wantErr:     true,
			errContains: "monitor would conflict",
		},
		{
			name:      "mixed_status_only_count_target",
			monitorID: 1,
			monitorIP: "192.168.1.10",
			existingMonitors: []ntpdb.GetMonitorPriorityRow{
				{
					ID:        2,
					MonitorIp: sql.NullString{String: "192.168.2.20", Valid: true},
					Status:    ntpdb.NullServerScoresStatus{ServerScoresStatus: ntpdb.ServerScoresStatusActive, Valid: true},
				},
				{
					ID:        3,
					MonitorIp: sql.NullString{String: "192.168.3.30", Valid: true},
					Status:    ntpdb.NullServerScoresStatus{ServerScoresStatus: ntpdb.ServerScoresStatusTesting, Valid: true},
				},
			},
			targetState: ntpdb.ServerScoresStatusActive,
			wantErr:     true, // Conflicts with active monitor when targeting active
		},
		{
			name:      "self_reference_should_not_conflict",
			monitorID: 100,
			monitorIP: "192.168.1.10",
			existingMonitors: []ntpdb.GetMonitorPriorityRow{
				{
					ID:        100,                                                 // Same ID as the monitor being checked
					MonitorIp: sql.NullString{String: "192.168.1.10", Valid: true}, // Same IP
					Status:    ntpdb.NullServerScoresStatus{ServerScoresStatus: ntpdb.ServerScoresStatusActive, Valid: true},
				},
				{
					ID:        101,
					MonitorIp: sql.NullString{String: "10.0.0.1", Valid: true}, // Different network
					Status:    ntpdb.NullServerScoresStatus{ServerScoresStatus: ntpdb.ServerScoresStatusActive, Valid: true},
				},
			},
			targetState: ntpdb.ServerScoresStatusActive,
			wantErr:     false, // Should not conflict with itself
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := sl.checkNetworkDiversityConstraint(tt.monitorID, tt.monitorIP, tt.existingMonitors, tt.targetState)
			if (err != nil) != tt.wantErr {
				t.Errorf("checkNetworkDiversityConstraint() error = %v, wantErr %v", err, tt.wantErr)
			}
			if err != nil && tt.errContains != "" {
				if !contains(err.Error(), tt.errContains) {
					t.Errorf("checkNetworkDiversityConstraint() error = %v, want error containing %v", err, tt.errContains)
				}
			}
		})
	}
}

func TestCanPromoteToTestingEmergencyOverride(t *testing.T) {
	sl := &Selector{
		log: slog.Default(),
	}

	// Helper function to create monitor candidate
	createMonitor := func(id uint32, accountID uint32, ip string, globalStatus ntpdb.MonitorsStatus, serverStatus ntpdb.ServerScoresStatus) *monitorCandidate {
		return &monitorCandidate{
			ID:           id,
			AccountID:    &accountID,
			IP:           ip,
			GlobalStatus: globalStatus,
			ServerStatus: serverStatus,
			IsHealthy:    true,
			HasMetrics:   true,
		}
	}

	// Helper function to create server info
	createServer := func(id uint32, accountID uint32, ip string) *serverInfo {
		return &serverInfo{
			ID:        id,
			AccountID: &accountID,
			IP:        ip,
		}
	}

	tests := []struct {
		name              string
		monitor           *monitorCandidate
		server            *serverInfo
		accountLimits     map[uint32]*accountLimit
		existingMonitors  []ntpdb.GetMonitorPriorityRow
		emergencyOverride bool
		wantPromote       bool
	}{
		{
			name:    "emergency_override_bypasses_account_limit",
			monitor: createMonitor(1, 1, "10.0.0.1", ntpdb.MonitorsStatusActive, ntpdb.ServerScoresStatusCandidate),
			server:  createServer(1, 2, "192.168.1.1"),
			accountLimits: map[uint32]*accountLimit{
				1: {AccountID: 1, MaxPerServer: 2, ActiveCount: 0, TestingCount: 3}, // Testing limit exceeded
			},
			existingMonitors:  []ntpdb.GetMonitorPriorityRow{},
			emergencyOverride: true,
			wantPromote:       true,
		},
		{
			name:    "emergency_override_bypasses_network_constraint",
			monitor: createMonitor(1, 1, "192.168.1.10", ntpdb.MonitorsStatusActive, ntpdb.ServerScoresStatusCandidate),
			server:  createServer(1, 2, "10.0.0.1"),
			accountLimits: map[uint32]*accountLimit{
				1: {AccountID: 1, MaxPerServer: 5, ActiveCount: 0, TestingCount: 0},
			},
			existingMonitors: []ntpdb.GetMonitorPriorityRow{
				{
					ID:        2,
					AccountID: sql.NullInt32{Int32: 2, Valid: true},
					MonitorIp: sql.NullString{String: "192.168.1.20", Valid: true}, // Same /24 subnet
					Status:    ntpdb.NullServerScoresStatus{ServerScoresStatus: ntpdb.ServerScoresStatusTesting, Valid: true},
				},
			},
			emergencyOverride: true,
			wantPromote:       true,
		},
		{
			name:    "emergency_override_bypasses_account_conflict",
			monitor: createMonitor(1, 1, "10.0.0.1", ntpdb.MonitorsStatusActive, ntpdb.ServerScoresStatusCandidate),
			server:  createServer(1, 2, "192.168.1.1"),
			accountLimits: map[uint32]*accountLimit{
				1: {AccountID: 1, MaxPerServer: 5, ActiveCount: 0, TestingCount: 0},
			},
			existingMonitors: []ntpdb.GetMonitorPriorityRow{
				{
					ID:        2,
					AccountID: sql.NullInt32{Int32: 1, Valid: true}, // Same account
					MonitorIp: sql.NullString{String: "10.0.0.2", Valid: true},
					Status:    ntpdb.NullServerScoresStatus{ServerScoresStatus: ntpdb.ServerScoresStatusTesting, Valid: true},
				},
			},
			emergencyOverride: true,
			wantPromote:       true,
		},
		{
			name:    "emergency_override_still_requires_active_testing_global_status",
			monitor: createMonitor(1, 1, "10.0.0.1", ntpdb.MonitorsStatusPending, ntpdb.ServerScoresStatusCandidate),
			server:  createServer(1, 2, "192.168.1.1"),
			accountLimits: map[uint32]*accountLimit{
				1: {AccountID: 1, MaxPerServer: 5, ActiveCount: 0, TestingCount: 0},
			},
			existingMonitors:  []ntpdb.GetMonitorPriorityRow{},
			emergencyOverride: true,
			wantPromote:       false, // Global status is pending, not active/testing
		},
		{
			name:    "no_emergency_account_limit_blocks_promotion",
			monitor: createMonitor(1, 1, "10.0.0.1", ntpdb.MonitorsStatusActive, ntpdb.ServerScoresStatusCandidate),
			server:  createServer(1, 2, "192.168.1.1"),
			accountLimits: map[uint32]*accountLimit{
				1: {AccountID: 1, MaxPerServer: 2, ActiveCount: 0, TestingCount: 3}, // Testing limit exceeded
			},
			existingMonitors:  []ntpdb.GetMonitorPriorityRow{},
			emergencyOverride: false,
			wantPromote:       false,
		},
		{
			name:    "no_emergency_network_constraint_blocks_promotion",
			monitor: createMonitor(1, 1, "192.168.1.10", ntpdb.MonitorsStatusActive, ntpdb.ServerScoresStatusCandidate),
			server:  createServer(1, 2, "10.0.0.1"),
			accountLimits: map[uint32]*accountLimit{
				1: {AccountID: 1, MaxPerServer: 5, ActiveCount: 0, TestingCount: 0},
			},
			existingMonitors: []ntpdb.GetMonitorPriorityRow{
				{
					ID:        2,
					AccountID: sql.NullInt32{Int32: 2, Valid: true},
					MonitorIp: sql.NullString{String: "192.168.1.20", Valid: true}, // Same /24 subnet
					Status:    ntpdb.NullServerScoresStatus{ServerScoresStatus: ntpdb.ServerScoresStatusTesting, Valid: true},
				},
			},
			emergencyOverride: false,
			wantPromote:       false,
		},
		{
			name:    "normal_promotion_without_constraints",
			monitor: createMonitor(1, 1, "10.0.0.1", ntpdb.MonitorsStatusActive, ntpdb.ServerScoresStatusCandidate),
			server:  createServer(1, 2, "192.168.1.1"),
			accountLimits: map[uint32]*accountLimit{
				1: {AccountID: 1, MaxPerServer: 5, ActiveCount: 0, TestingCount: 0},
			},
			existingMonitors:  []ntpdb.GetMonitorPriorityRow{},
			emergencyOverride: false,
			wantPromote:       true,
		},
		{
			name:    "emergency_with_multiple_constraint_violations",
			monitor: createMonitor(1, 1, "192.168.1.10", ntpdb.MonitorsStatusActive, ntpdb.ServerScoresStatusCandidate),
			server:  createServer(1, 1, "10.0.0.1"), // Same account as monitor
			accountLimits: map[uint32]*accountLimit{
				1: {AccountID: 1, MaxPerServer: 2, ActiveCount: 0, TestingCount: 3}, // Testing limit exceeded
			},
			existingMonitors: []ntpdb.GetMonitorPriorityRow{
				{
					ID:        2,
					AccountID: sql.NullInt32{Int32: 2, Valid: true},
					MonitorIp: sql.NullString{String: "192.168.1.20", Valid: true}, // Same /24 subnet
					Status:    ntpdb.NullServerScoresStatus{ServerScoresStatus: ntpdb.ServerScoresStatusTesting, Valid: true},
				},
			},
			emergencyOverride: true,
			wantPromote:       true, // Emergency override bypasses all constraints
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			gotPromote := sl.canPromoteToTesting(tt.monitor, tt.server, tt.accountLimits, tt.existingMonitors, tt.emergencyOverride)
			if gotPromote != tt.wantPromote {
				t.Errorf("canPromoteToTesting() = %v, want %v", gotPromote, tt.wantPromote)
			}
		})
	}
}
