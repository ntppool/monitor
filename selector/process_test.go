package selector

import (
	"context"
	"log/slog"
	"testing"

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
		violation: &constraintViolation{
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
