package statusscore

import (
	"bytes"
	"context"
	"encoding/json"
	"log/slog"
	"math"
	"testing"
	"time"

	"github.com/jackc/pgx/v5/pgtype"
	"go.ntppool.org/common/logger"
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

func TestRttStorage(t *testing.T) {
	// A real NTP round trip is never zero or negative, nor does it need
	// more than an int32 of microseconds; a server that reports one is
	// calculating its processing time wrong. That is a bad host: the
	// sample is an error (step -4, no offset) and the rtt is stored as
	// NULL, and logged. A timeout has no round trip at all and stores NULL
	// quietly.
	respondingStatus := func(rtt *durationpb.Duration) *apiv2.ServerStatus {
		return &apiv2.ServerStatus{
			Ts:      timestamppb.New(time.Now()),
			Offset:  durationpb.New(1 * time.Millisecond),
			Rtt:     rtt,
			Stratum: 2,
		}
	}

	tests := []struct {
		name       string
		status     *apiv2.ServerStatus
		wantRtt    pgtype.Int4
		wantStep   float64
		wantOffset bool   // whether an offset is stored
		wantError  string // the error recorded in the attributes
		wantLog    bool
	}{
		{
			name:       "typical rtt",
			status:     respondingStatus(durationpb.New(10 * time.Millisecond)),
			wantRtt:    pgtype.Int4{Int32: 10000, Valid: true},
			wantStep:   1,
			wantOffset: true,
		},
		{
			// the NTP library clamps a negative rtt to zero and a real
			// round trip is never zero, so this is a bad measurement
			name:      "zero rtt on a response",
			status:    respondingStatus(durationpb.New(0)),
			wantRtt:   pgtype.Int4{},
			wantStep:  -4,
			wantError: "implausible rtt",
			wantLog:   true,
		},
		{
			name:      "rtt under a microsecond would store as zero",
			status:    respondingStatus(durationpb.New(400 * time.Nanosecond)),
			wantRtt:   pgtype.Int4{},
			wantStep:  -4,
			wantError: "implausible rtt",
			wantLog:   true,
		},
		{
			name:       "largest rtt that fits in int32 microseconds",
			status:     respondingStatus(durationpb.New(math.MaxInt32 * time.Microsecond)),
			wantRtt:    pgtype.Int4{Int32: math.MaxInt32, Valid: true},
			wantStep:   1,
			wantOffset: true,
		},
		{
			name:      "negative rtt",
			status:    respondingStatus(durationpb.New(-19462 * time.Microsecond)),
			wantRtt:   pgtype.Int4{},
			wantStep:  -4,
			wantError: "implausible rtt",
			wantLog:   true,
		},
		{
			name:      "rtt too large for int32 microseconds",
			status:    respondingStatus(durationpb.New((math.MaxInt32 + 1) * time.Microsecond)),
			wantRtt:   pgtype.Int4{},
			wantStep:  -4,
			wantError: "implausible rtt",
			wantLog:   true,
		},
		{
			name: "timeout has no rtt",
			status: &apiv2.ServerStatus{
				Ts:         timestamppb.New(time.Now()),
				NoResponse: true,
			},
			wantRtt:  pgtype.Int4{},
			wantStep: -5,
		},
		{
			// older clients may send a zero rtt with a timeout
			name: "timeout with an explicit zero rtt",
			status: &apiv2.ServerStatus{
				Ts:         timestamppb.New(time.Now()),
				NoResponse: true,
				Rtt:        durationpb.New(0),
			},
			wantRtt:  pgtype.Int4{},
			wantStep: -5,
		},
		{
			name:       "response without rtt",
			status:     respondingStatus(nil),
			wantRtt:    pgtype.Int4{},
			wantStep:   1,
			wantOffset: true,
			wantLog:    true,
		},
		{
			// a kiss code has its own step; the rtt is dropped and logged
			name: "kiss-o'-death with a zero rtt keeps its kiss code",
			status: &apiv2.ServerStatus{
				Ts:      timestamppb.New(time.Now()),
				Rtt:     durationpb.New(0),
				Error:   "RATE",
				Stratum: 0,
			},
			wantRtt:   pgtype.Int4{},
			wantStep:  -3.5,
			wantError: "RATE",
			wantLog:   true,
		},
		{
			name: "existing error is kept",
			status: &apiv2.ServerStatus{
				Ts:      timestamppb.New(time.Now()),
				Rtt:     durationpb.New(0),
				Error:   "bad stratum 11",
				Stratum: 11,
			},
			wantRtt:   pgtype.Int4{},
			wantStep:  -4,
			wantError: "bad stratum 11",
			wantLog:   true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			var logBuf bytes.Buffer
			ctx := logger.NewContext(context.Background(), slog.New(
				slog.NewJSONHandler(&logBuf, &slog.HandlerOptions{Level: slog.LevelWarn}),
			))
			server := &ntpdb.Server{ID: 42}

			got, err := NewScorer().Score(ctx, server, tt.status)
			if err != nil {
				t.Fatalf("Score() error = %v", err)
			}

			if got.Rtt != tt.wantRtt {
				t.Errorf("Rtt = %+v, want %+v", got.Rtt, tt.wantRtt)
			}
			if got.Step != tt.wantStep {
				t.Errorf("Step = %v, want %v", got.Step, tt.wantStep)
			}
			if got.Offset.Valid != tt.wantOffset {
				t.Errorf("Offset.Valid = %v, want %v", got.Offset.Valid, tt.wantOffset)
			}

			gotError := ""
			if got.Attributes != nil {
				var attrs ntpdb.LogScoreAttributes
				if err := json.Unmarshal(*got.Attributes, &attrs); err != nil {
					t.Fatalf("attributes %q: %v", *got.Attributes, err)
				}
				gotError = attrs.Error
			}
			if gotError != tt.wantError {
				t.Errorf("attributes error = %q, want %q", gotError, tt.wantError)
			}

			if !tt.wantLog {
				if logBuf.Len() != 0 {
					t.Errorf("unexpected log output: %s", logBuf.String())
				}
				return
			}

			// exactly one record, or Unmarshal fails on the trailing data
			var rec map[string]any
			if err := json.Unmarshal(logBuf.Bytes(), &rec); err != nil {
				t.Fatalf("want one log record, got %q: %v", logBuf.String(), err)
			}
			if rec["level"] != "WARN" {
				t.Errorf("log level = %v, want WARN", rec["level"])
			}
			if rec["server_id"] != float64(server.ID) {
				t.Errorf("log server_id = %v, want %d", rec["server_id"], server.ID)
			}
			if tt.status.Rtt != nil {
				// slog's JSON handler writes a time.Duration as nanoseconds
				if want := float64(tt.status.Rtt.AsDuration()); rec["rtt"] != want {
					t.Errorf("log rtt = %v, want %v", rec["rtt"], want)
				}
			}
		})
	}
}

// Helper function for absolute value
func abs(x float64) float64 {
	if x < 0 {
		return -x
	}
	return x
}

func TestAttributesWithoutNUL(t *testing.T) {
	// The client reports a stratum 0 response with a zero reference ID
	// with the ID as four NUL bytes. PostgreSQL rejects \u0000 in jsonb,
	// which would fail the insert and the whole batch with it.
	status := &apiv2.ServerStatus{
		Ts:      timestamppb.New(time.Now()),
		Rtt:     durationpb.New(10 * time.Millisecond),
		Error:   "bad stratum 0 (referenceID: 0x0, \x00\x00\x00\x00)",
		Stratum: 0,
	}

	got, err := NewScorer().Score(context.Background(), &ntpdb.Server{ID: 1}, status)
	if err != nil {
		t.Fatalf("Score() error = %v", err)
	}
	if got.Attributes == nil {
		t.Fatal("Attributes = nil, want the error")
	}
	if bytes.Contains(*got.Attributes, []byte(`\u0000`)) {
		t.Errorf("attributes contain NUL: %s", *got.Attributes)
	}

	var attrs ntpdb.LogScoreAttributes
	if err := json.Unmarshal(*got.Attributes, &attrs); err != nil {
		t.Fatalf("attributes %q: %v", *got.Attributes, err)
	}
	if want := "bad stratum 0 (referenceID: 0x0, )"; attrs.Error != want {
		t.Errorf("attributes error = %q, want %q", attrs.Error, want)
	}
}
