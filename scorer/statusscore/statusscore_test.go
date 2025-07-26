package statusscore

import (
	"context"
	"testing"
	"time"

	apiv2 "go.ntppool.org/monitor/gen/monitor/v2"
	"go.ntppool.org/monitor/ntpdb"
	"google.golang.org/protobuf/types/known/durationpb"
	"google.golang.org/protobuf/types/known/timestamppb"
)

func TestScoringAlgorithm(t *testing.T) {
	scorer := NewScorer()
	ctx := context.Background()
	server := &ntpdb.Server{ID: 1}

	tests := []struct {
		name               string
		offsetMs           float64
		expectedStep       float64
		expectedEquilScore float64 // equilibrium score (step / 0.05)
	}{
		// Optimal range (≤ 25ms)
		{"Perfect sync", 0, 1.0, 20.0},
		{"Excellent 10ms", 10, 1.0, 20.0},
		{"Good 25ms", 25, 1.0, 20.0},

		// Linear range 1 (25-100ms)
		{"Transition 26ms", 26, 0.993, 19.86},
		{"Fair 50ms", 50, 0.833, 16.66},
		{"Score=10 threshold 100ms", 100, 0.5, 10.0},

		// Linear range 2 (100-750ms)
		{"Moderate 200ms", 200, 0.270, 5.40},
		{"Poor 400ms", 400, -0.192, -3.84},
		{"Bad 600ms", 600, -0.654, -13.08},
		{"Harsh 749ms", 749, -0.998, -19.96},

		// Cutoff range (>750ms)
		{"Just at 750ms", 750, -1.0, -20.0}, // Our formula at exactly 750ms
		{"Very bad 1000ms", 1000, -2.0, -40.0},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Create test status with specific offset
			status := &apiv2.ServerStatus{
				Ts:      timestamppb.New(time.Now()),
				Offset:  durationpb.New(time.Duration(tt.offsetMs * float64(time.Millisecond))),
				Rtt:     durationpb.New(10 * time.Millisecond),
				Stratum: 2, // Valid stratum
			}

			score, err := scorer.Score(ctx, server, status)
			if err != nil {
				t.Fatalf("Score() error = %v", err)
			}

			// Check step value (with tolerance for floating point)
			tolerance := 0.01
			if abs(score.Step-tt.expectedStep) > tolerance {
				t.Errorf("Step = %v, want %v (±%v)", score.Step, tt.expectedStep, tolerance)
			}

			// Verify equilibrium score calculation
			expectedEquilScore := tt.expectedStep / 0.05
			if abs(expectedEquilScore-tt.expectedEquilScore) > tolerance {
				t.Errorf("Equilibrium score = %v, want %v", expectedEquilScore, tt.expectedEquilScore)
			}
		})
	}
}

func TestSanityChecks(t *testing.T) {
	scorer := NewScorer()
	ctx := context.Background()
	server := &ntpdb.Server{ID: 1}

	// Test step never exceeds +1
	status := &apiv2.ServerStatus{
		Ts:     timestamppb.New(time.Now()),
		Offset: durationpb.New(1 * time.Millisecond), // Very small offset
		Rtt:    durationpb.New(10 * time.Millisecond),
	}

	score, err := scorer.Score(ctx, server, status)
	if err != nil {
		t.Fatalf("Score() error = %v", err)
	}

	if score.Step > 1.0 {
		t.Errorf("Step = %v, should never exceed 1.0", score.Step)
	}
}

func TestEquilibriumScoring(t *testing.T) {
	// Test that simulates sustained offset and validates equilibrium score
	tests := []struct {
		name          string
		offsetMs      float64
		expectedScore float64
		tolerance     float64
	}{
		{"Perfect maintains 20", 10, 19.88, 0.2}, // Allow more tolerance
		{"100ms maintains 10", 100, 10.0, 0.1},
		{"200ms maintains ~5", 200, 5.4, 0.5},
		{"400ms negative score", 400, -3.84, 0.5},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Simulate sustained measurements to reach equilibrium
			scorer := NewScorer()
			ctx := context.Background()
			server := &ntpdb.Server{ID: 1}

			currentScore := 0.0
			status := &apiv2.ServerStatus{
				Ts:      timestamppb.New(time.Now()),
				Offset:  durationpb.New(time.Duration(tt.offsetMs * float64(time.Millisecond))),
				Rtt:     durationpb.New(10 * time.Millisecond),
				Stratum: 2, // Valid stratum
			}

			// Get step value
			scoreResult, err := scorer.Score(ctx, server, status)
			if err != nil {
				t.Fatalf("Score() error = %v", err)
			}
			step := scoreResult.Step

			// Simulate 100 measurements to approach equilibrium
			// Formula: new_score = step + (old_score * 0.95)
			for i := 0; i < 100; i++ {
				currentScore = step + (currentScore * 0.95)
			}

			if abs(currentScore-tt.expectedScore) > tt.tolerance {
				t.Errorf("Equilibrium score = %v, want %v (±%v)", currentScore, tt.expectedScore, tt.tolerance)
			}
		})
	}
}

func TestSpecialCases(t *testing.T) {
	scorer := NewScorer()
	ctx := context.Background()
	server := &ntpdb.Server{ID: 1}

	// Test no response
	status := &apiv2.ServerStatus{
		Ts:         timestamppb.New(time.Now()),
		NoResponse: true,
	}

	score, err := scorer.Score(ctx, server, status)
	if err != nil {
		t.Fatalf("Score() error = %v", err)
	}

	if score.Step != -5 {
		t.Errorf("NoResponse step = %v, want -5", score.Step)
	}

	// Test large offset > 3 seconds
	status = &apiv2.ServerStatus{
		Ts:      timestamppb.New(time.Now()),
		Offset:  durationpb.New(5 * time.Second),
		Rtt:     durationpb.New(10 * time.Millisecond),
		Stratum: 2, // Valid stratum
	}

	score, err = scorer.Score(ctx, server, status)
	if err != nil {
		t.Fatalf("Score() error = %v", err)
	}

	if score.Step != -4 {
		t.Errorf("Large offset step = %v, want -4", score.Step)
	}
}

// Helper function for absolute value
func abs(x float64) float64 {
	if x < 0 {
		return -x
	}
	return x
}
