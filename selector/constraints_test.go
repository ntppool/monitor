package selector

import (
	"database/sql"
	"testing"

	"go.ntppool.org/monitor/ntpdb"
)

func TestCheckNetworkConstraint(t *testing.T) {
	sl := &Selector{}

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

func TestCheckAccountConstraints(t *testing.T) {
	sl := &Selector{}

	accountID1 := uint32(1)
	accountID2 := uint32(2)
	accountID3 := uint32(3)

	tests := []struct {
		name          string
		monitor       monitorCandidate
		server        serverInfo
		accountLimits map[uint32]*accountLimit
		targetState   ntpdb.ServerScoresStatus
		wantErr       bool
		errContains   string
	}{
		{
			name: "same_account_violation",
			monitor: monitorCandidate{
				AccountID: &accountID1,
			},
			server: serverInfo{
				AccountID: &accountID1,
			},
			accountLimits: map[uint32]*accountLimit{},
			targetState:   ntpdb.ServerScoresStatusActive,
			wantErr:       true,
			errContains:   "same account",
		},
		{
			name: "different_accounts_ok",
			monitor: monitorCandidate{
				AccountID: &accountID1,
			},
			server: serverInfo{
				AccountID: &accountID2,
			},
			accountLimits: map[uint32]*accountLimit{
				1: {AccountID: 1, MaxPerServer: 2, ActiveCount: 0, TestingCount: 0},
			},
			targetState: ntpdb.ServerScoresStatusActive,
			wantErr:     false,
		},
		{
			name: "active_limit_reached",
			monitor: monitorCandidate{
				AccountID:    &accountID1,
				ServerStatus: ntpdb.ServerScoresStatusNew,
			},
			server: serverInfo{
				AccountID: &accountID2,
			},
			accountLimits: map[uint32]*accountLimit{
				1: {AccountID: 1, MaxPerServer: 2, ActiveCount: 2, TestingCount: 0},
			},
			targetState: ntpdb.ServerScoresStatusActive,
			wantErr:     true,
			errContains: "active limit",
		},
		{
			name: "testing_limit_reached",
			monitor: monitorCandidate{
				AccountID:    &accountID1,
				ServerStatus: ntpdb.ServerScoresStatusNew,
			},
			server: serverInfo{
				AccountID: &accountID2,
			},
			accountLimits: map[uint32]*accountLimit{
				1: {AccountID: 1, MaxPerServer: 2, ActiveCount: 0, TestingCount: 3},
			},
			targetState: ntpdb.ServerScoresStatusTesting,
			wantErr:     true,
			errContains: "testing limit",
		},
		{
			name: "total_limit_reached",
			monitor: monitorCandidate{
				AccountID:    &accountID1,
				ServerStatus: ntpdb.ServerScoresStatusNew,
			},
			server: serverInfo{
				AccountID: &accountID2,
			},
			accountLimits: map[uint32]*accountLimit{
				1: {AccountID: 1, MaxPerServer: 2, ActiveCount: 1, TestingCount: 2},
			},
			targetState: ntpdb.ServerScoresStatusActive,
			wantErr:     true,
			errContains: "total limit",
		},
		{
			name: "no_limit_on_candidates",
			monitor: monitorCandidate{
				AccountID:    &accountID1,
				ServerStatus: ntpdb.ServerScoresStatusNew,
			},
			server: serverInfo{
				AccountID: &accountID2,
			},
			accountLimits: map[uint32]*accountLimit{
				1: {AccountID: 1, MaxPerServer: 1, ActiveCount: 5, TestingCount: 5},
			},
			targetState: ntpdb.ServerScoresStatusCandidate,
			wantErr:     false,
		},
		{
			name: "dont_count_self_when_already_active",
			monitor: monitorCandidate{
				AccountID:    &accountID1,
				ServerStatus: ntpdb.ServerScoresStatusActive,
			},
			server: serverInfo{
				AccountID: &accountID2,
			},
			accountLimits: map[uint32]*accountLimit{
				1: {AccountID: 1, MaxPerServer: 2, ActiveCount: 2, TestingCount: 0},
			},
			targetState: ntpdb.ServerScoresStatusActive,
			wantErr:     false, // Should not error because we don't count self
		},
		{
			name: "monitor_without_account",
			monitor: monitorCandidate{
				AccountID: nil,
			},
			server: serverInfo{
				AccountID: &accountID1,
			},
			accountLimits: map[uint32]*accountLimit{},
			targetState:   ntpdb.ServerScoresStatusActive,
			wantErr:       false,
		},
		{
			name: "account_limit_not_loaded",
			monitor: monitorCandidate{
				AccountID: &accountID3,
			},
			server: serverInfo{
				AccountID: &accountID1,
			},
			accountLimits: map[uint32]*accountLimit{
				1: {AccountID: 1, MaxPerServer: 2},
			},
			targetState: ntpdb.ServerScoresStatusActive,
			wantErr:     true,
			errContains: "not loaded",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := sl.checkAccountConstraints(&tt.monitor, &tt.server, tt.accountLimits, tt.targetState)
			if (err != nil) != tt.wantErr {
				t.Errorf("checkAccountConstraints() error = %v, wantErr %v", err, tt.wantErr)
			}
			if err != nil && tt.errContains != "" {
				if !contains(err.Error(), tt.errContains) {
					t.Errorf("checkAccountConstraints() error = %v, want error containing %v", err, tt.errContains)
				}
			}
		})
	}
}

func contains(s, substr string) bool {
	return len(s) >= len(substr) && (s == substr || len(s) > 0 && len(substr) == 0 || (len(substr) > 0 && findSubstring(s, substr) != -1))
}

func findSubstring(s, substr string) int {
	if len(substr) == 0 {
		return 0
	}
	for i := 0; i <= len(s)-len(substr); i++ {
		if s[i:i+len(substr)] == substr {
			return i
		}
	}
	return -1
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
			name:             "no_existing_monitors_ok",
			monitorID:        100,
			monitorIP:        "192.168.1.10",
			existingMonitors: []ntpdb.GetMonitorPriorityRow{},
			targetState:      ntpdb.ServerScoresStatusActive,
			wantErr:          false,
		},
		{
			name:      "ipv4_different_20_networks_ok",
			monitorID: 100,
			monitorIP: "192.168.1.10", // 192.168.0.0/20
			existingMonitors: []ntpdb.GetMonitorPriorityRow{
				{
					ID:        1,
					MonitorIp: sql.NullString{String: "192.169.1.10", Valid: true}, // 192.169.0.0/20 (different /20)
					Status:    ntpdb.NullServerScoresStatus{ServerScoresStatus: ntpdb.ServerScoresStatusActive, Valid: true},
				},
			},
			targetState: ntpdb.ServerScoresStatusActive,
			wantErr:     false,
		},
		{
			name:      "ipv4_same_20_network_conflict",
			monitorID: 100,
			monitorIP: "192.168.1.10", // 192.168.0.0/20
			existingMonitors: []ntpdb.GetMonitorPriorityRow{
				{
					ID:        1,
					MonitorIp: sql.NullString{String: "192.168.15.20", Valid: true}, // Same /20 network
					Status:    ntpdb.NullServerScoresStatus{ServerScoresStatus: ntpdb.ServerScoresStatusActive, Valid: true},
				},
			},
			targetState: ntpdb.ServerScoresStatusActive,
			wantErr:     true,
			errContains: "conflict",
		},
		{
			name:      "ipv6_different_44_networks_ok",
			monitorID: 100,
			monitorIP: "2001:db8:1000::1", // 2001:db8:1000::/44
			existingMonitors: []ntpdb.GetMonitorPriorityRow{
				{
					ID:        1,
					MonitorIp: sql.NullString{String: "2001:db8:2000::1", Valid: true}, // 2001:db8:2000::/44 (different /44)
					Status:    ntpdb.NullServerScoresStatus{ServerScoresStatus: ntpdb.ServerScoresStatusActive, Valid: true},
				},
			},
			targetState: ntpdb.ServerScoresStatusActive,
			wantErr:     false,
		},
		{
			name:      "ipv6_same_44_network_conflict",
			monitorID: 100,
			monitorIP: "2001:db8:1000::1", // 2001:db8:1000::/44
			existingMonitors: []ntpdb.GetMonitorPriorityRow{
				{
					ID:        1,
					MonitorIp: sql.NullString{String: "2001:db8:100f::1", Valid: true}, // Same /44 network
					Status:    ntpdb.NullServerScoresStatus{ServerScoresStatus: ntpdb.ServerScoresStatusActive, Valid: true},
				},
			},
			targetState: ntpdb.ServerScoresStatusActive,
			wantErr:     true,
			errContains: "conflict",
		},
		{
			name:      "testing_with_existing_active_conflict",
			monitorID: 100,
			monitorIP: "192.168.1.10",
			existingMonitors: []ntpdb.GetMonitorPriorityRow{
				{
					ID:        1,
					MonitorIp: sql.NullString{String: "192.168.15.20", Valid: true}, // Same /20 network
					Status:    ntpdb.NullServerScoresStatus{ServerScoresStatus: ntpdb.ServerScoresStatusActive, Valid: true},
				},
			},
			targetState: ntpdb.ServerScoresStatusTesting,
			wantErr:     true,
			errContains: "conflict",
		},
		{
			name:      "candidate_state_no_conflict_check",
			monitorID: 100,
			monitorIP: "192.168.1.10",
			existingMonitors: []ntpdb.GetMonitorPriorityRow{
				{
					ID:        1,
					MonitorIp: sql.NullString{String: "192.168.15.20", Valid: true}, // Same /20 network
					Status:    ntpdb.NullServerScoresStatus{ServerScoresStatus: ntpdb.ServerScoresStatusActive, Valid: true},
				},
			},
			targetState: ntpdb.ServerScoresStatusCandidate,
			wantErr:     false, // Candidates don't conflict with active/testing
		},
		{
			name:      "self_comparison_should_not_conflict",
			monitorID: 100,
			monitorIP: "192.168.1.10",
			existingMonitors: []ntpdb.GetMonitorPriorityRow{
				{
					ID:        100, // Same ID as the monitor being checked
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
