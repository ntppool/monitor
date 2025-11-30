package selector

import (
	"log/slog"
	"testing"

	"go.ntppool.org/monitor/ntpdb"
)

func TestCandidateStateString(t *testing.T) {
	tests := []struct {
		state    candidateState
		expected string
	}{
		{candidateUnknown, "candidateUnknown"},
		{candidateIn, "candidateIn"},
		{candidateOut, "candidateOut"},
		{candidateBlock, "candidateBlock"},
		{candidatePending, "candidatePending"},
	}

	for _, tt := range tests {
		t.Run(tt.expected, func(t *testing.T) {
			if got := tt.state.String(); got != tt.expected {
				t.Errorf("candidateState.String() = %v, want %v", got, tt.expected)
			}
		})
	}
}

func TestDetermineState(t *testing.T) {
	sl := &Selector{
		log: slog.Default(),
	}

	tests := []struct {
		name      string
		monitor   monitorCandidate
		violation constraintViolation
		want      candidateState
	}{
		{
			name: "globally_pending_monitor_should_phase_out_gradually",
			monitor: monitorCandidate{
				GlobalStatus: ntpdb.MonitorsStatusPending,
				ServerStatus: ntpdb.ServerScoresStatusCandidate,
			},
			violation: constraintViolation{Type: violationNone},
			want:      candidateOut,
		},
		{
			name: "globally_paused_monitor_should_be_blocked",
			monitor: monitorCandidate{
				GlobalStatus: ntpdb.MonitorsStatusPaused,
				ServerStatus: ntpdb.ServerScoresStatusActive,
			},
			violation: constraintViolation{Type: violationNone},
			want:      candidateBlock,
		},
		{
			name: "globally_deleted_monitor_should_be_blocked",
			monitor: monitorCandidate{
				GlobalStatus: ntpdb.MonitorsStatusDeleted,
				ServerStatus: ntpdb.ServerScoresStatusTesting,
			},
			violation: constraintViolation{Type: violationNone},
			want:      candidateBlock,
		},
		{
			name: "globally_pending_but_server_active_is_inconsistent",
			monitor: monitorCandidate{
				GlobalStatus: ntpdb.MonitorsStatusPending,
				ServerStatus: ntpdb.ServerScoresStatusActive,
			},
			violation: constraintViolation{Type: violationNone},
			want:      candidateOut, // Gradual removal for inconsistency
		},
		{
			name: "active_monitor_with_no_violations_is_candidate_in",
			monitor: monitorCandidate{
				GlobalStatus: ntpdb.MonitorsStatusActive,
				ServerStatus: ntpdb.ServerScoresStatusActive,
				HasMetrics:   true,
				IsHealthy:    true,
			},
			violation: constraintViolation{Type: violationNone},
			want:      candidateIn,
		},
		{
			name: "testing_monitor_cannot_be_promoted_unless_globally_active",
			monitor: monitorCandidate{
				GlobalStatus: ntpdb.MonitorsStatusTesting,
				ServerStatus: ntpdb.ServerScoresStatusTesting,
				HasMetrics:   true,
				IsHealthy:    true,
			},
			violation: constraintViolation{Type: violationNone},
			want:      candidatePending, // Stay in testing
		},
		{
			name: "candidate_monitor_with_constraint_violation_gets_gradual_removal",
			monitor: monitorCandidate{
				GlobalStatus: ntpdb.MonitorsStatusActive,
				ServerStatus: ntpdb.ServerScoresStatusCandidate,
			},
			violation: constraintViolation{
				Type: violationNetworkSameSubnet,
			},
			want: candidateOut,
		},
		{
			name: "unhealthy_monitor_is_candidate_out",
			monitor: monitorCandidate{
				GlobalStatus: ntpdb.MonitorsStatusActive,
				ServerStatus: ntpdb.ServerScoresStatusActive,
				HasMetrics:   true,
				IsHealthy:    false,
			},
			violation: constraintViolation{Type: violationNone},
			want:      candidateOut,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := sl.determineState(&tt.monitor, &tt.violation)
			if got != tt.want {
				t.Errorf("determineState() = %v, want %v", got, tt.want)
			}
		})
	}
}

func TestHasStateInconsistency(t *testing.T) {
	sl := &Selector{
		log: slog.Default(),
	}

	tests := []struct {
		name    string
		monitor monitorCandidate
		want    bool
	}{
		{
			name: "globally_pending_but_server_active_is_inconsistent",
			monitor: monitorCandidate{
				GlobalStatus: ntpdb.MonitorsStatusPending,
				ServerStatus: ntpdb.ServerScoresStatusActive,
			},
			want: true,
		},
		{
			name: "globally_pending_but_server_testing_is_inconsistent",
			monitor: monitorCandidate{
				GlobalStatus: ntpdb.MonitorsStatusPending,
				ServerStatus: ntpdb.ServerScoresStatusTesting,
			},
			want: true,
		},
		{
			name: "globally_deleted_but_server_active_is_inconsistent",
			monitor: monitorCandidate{
				GlobalStatus: ntpdb.MonitorsStatusDeleted,
				ServerStatus: ntpdb.ServerScoresStatusActive,
			},
			want: true,
		},
		{
			name: "globally_active_and_server_active_is_consistent",
			monitor: monitorCandidate{
				GlobalStatus: ntpdb.MonitorsStatusActive,
				ServerStatus: ntpdb.ServerScoresStatusActive,
			},
			want: false,
		},
		{
			name: "globally_testing_and_server_testing_is_consistent",
			monitor: monitorCandidate{
				GlobalStatus: ntpdb.MonitorsStatusTesting,
				ServerStatus: ntpdb.ServerScoresStatusTesting,
			},
			want: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := sl.hasStateInconsistency(&tt.monitor); got != tt.want {
				t.Errorf("hasStateInconsistency() = %v, want %v", got, tt.want)
			}
		})
	}
}

func TestIsOutOfOrder(t *testing.T) {
	tests := []struct {
		name        string
		statusList  newStatusList
		wantBest    int64
		wantReplace int64
		wantFound   bool
	}{
		{
			name: "no_candidates_returns_not_found",
			statusList: newStatusList{
				{MonitorID: 1, CurrentStatus: ntpdb.ServerScoresStatusActive, NewState: candidateOut},
				{MonitorID: 2, CurrentStatus: ntpdb.ServerScoresStatusTesting, NewState: candidateOut},
			},
			wantBest:    0,
			wantReplace: 0,
			wantFound:   false,
		},
		{
			name: "testing_candidate_replaces_active",
			statusList: newStatusList{
				{MonitorID: 2, CurrentStatus: ntpdb.ServerScoresStatusTesting, NewState: candidateIn}, // Must come first to set best
				{MonitorID: 1, CurrentStatus: ntpdb.ServerScoresStatusActive, NewState: candidateIn},
			},
			wantBest:    2,
			wantReplace: 1,
			wantFound:   true,
		},
		{
			name: "only_testing_candidates_returns_not_found",
			statusList: newStatusList{
				{MonitorID: 1, CurrentStatus: ntpdb.ServerScoresStatusTesting, NewState: candidateIn},
				{MonitorID: 2, CurrentStatus: ntpdb.ServerScoresStatusTesting, NewState: candidateIn},
			},
			wantBest:    0, // No replacement found
			wantReplace: 0,
			wantFound:   false,
		},
		{
			name: "only_active_candidates_returns_not_found",
			statusList: newStatusList{
				{MonitorID: 1, CurrentStatus: ntpdb.ServerScoresStatusActive, NewState: candidateIn},
				{MonitorID: 2, CurrentStatus: ntpdb.ServerScoresStatusActive, NewState: candidateIn},
			},
			wantBest:    0,
			wantReplace: 0,
			wantFound:   false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			gotBest, gotReplace, gotFound := tt.statusList.IsOutOfOrder()
			if gotBest != tt.wantBest || gotReplace != tt.wantReplace || gotFound != tt.wantFound {
				t.Errorf("IsOutOfOrder() = (%v, %v, %v), want (%v, %v, %v)",
					gotBest, gotReplace, gotFound,
					tt.wantBest, tt.wantReplace, tt.wantFound)
			}
		})
	}
}
