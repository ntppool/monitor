package monitor

import (
	"errors"
	"testing"
	"time"

	"google.golang.org/protobuf/types/known/durationpb"

	apiv2 "go.ntppool.org/monitor/gen/monitor/v2"
)

// Helper function to create a response for testing
func createTestResponse(hasError bool, errorMsg string, noResponse bool, rttMs int) *response {
	r := &response{
		Status: &apiv2.ServerStatus{
			NoResponse: noResponse,
		},
	}

	if rttMs > 0 {
		r.Status.Rtt = durationpb.New(time.Duration(rttMs) * time.Millisecond)
	}

	if hasError {
		r.Error = errors.New(errorMsg)
	}

	return r
}

func TestResponseSelection(t *testing.T) {
	tests := []struct {
		name      string
		responses []*response
		expected  struct {
			hasError   bool
			noResponse bool
			rttMs      int
		}
	}{
		{
			name: "single valid response",
			responses: []*response{
				createTestResponse(false, "", false, 50),
			},
			expected: struct {
				hasError   bool
				noResponse bool
				rttMs      int
			}{false, false, 50},
		},
		{
			name: "single timeout response",
			responses: []*response{
				createTestResponse(true, "network: i/o timeout", true, 0),
			},
			expected: struct {
				hasError   bool
				noResponse bool
				rttMs      int
			}{true, true, 0},
		},
		{
			name: "valid response wins over timeout (your bug scenario)",
			responses: []*response{
				createTestResponse(true, "network: i/o timeout", true, 0), // Query 1: timeout
				createTestResponse(false, "", false, 45),                  // Query 2: valid
				createTestResponse(false, "", false, 55),                  // Query 3: valid
				createTestResponse(true, "network: i/o timeout", true, 0), // Query 4: timeout
			},
			expected: struct {
				hasError   bool
				noResponse bool
				rttMs      int
			}{false, false, 45}, // Should pick the faster valid response (45ms)
		},
		{
			name: "partial response wins over complete timeout",
			responses: []*response{
				createTestResponse(true, "network: i/o timeout", true, 0), // Complete timeout
				createTestResponse(true, "bad stratum", false, 30),        // Partial response with error
			},
			expected: struct {
				hasError   bool
				noResponse bool
				rttMs      int
			}{true, false, 30}, // Should pick the partial response
		},
		{
			name: "fastest valid response among multiple valid",
			responses: []*response{
				createTestResponse(false, "", false, 100),
				createTestResponse(false, "", false, 25), // Fastest
				createTestResponse(false, "", false, 75),
			},
			expected: struct {
				hasError   bool
				noResponse bool
				rttMs      int
			}{false, false, 25},
		},
		{
			name: "fastest partial response among multiple partial",
			responses: []*response{
				createTestResponse(true, "bad stratum", false, 80),
				createTestResponse(true, "bad stratum", false, 40), // Fastest partial
				createTestResponse(true, "bad stratum", false, 60),
			},
			expected: struct {
				hasError   bool
				noResponse bool
				rttMs      int
			}{true, false, 40},
		},
		{
			name: "mixed scenario - valid should always win",
			responses: []*response{
				createTestResponse(true, "network: i/o timeout", true, 0), // Complete timeout
				createTestResponse(true, "bad stratum", false, 20),        // Fastest partial
				createTestResponse(false, "", false, 200),                 // Slow but valid
				createTestResponse(true, "network: i/o timeout", true, 0), // Another timeout
			},
			expected: struct {
				hasError   bool
				noResponse bool
				rttMs      int
			}{false, false, 200}, // Valid response should win despite being slowest
		},
		{
			name: "all timeouts - pick any (first one)",
			responses: []*response{
				createTestResponse(true, "network: i/o timeout", true, 0),
				createTestResponse(true, "network: i/o timeout", true, 0),
				createTestResponse(true, "network: i/o timeout", true, 0),
			},
			expected: struct {
				hasError   bool
				noResponse bool
				rttMs      int
			}{true, true, 0},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Simulate the selection logic from CheckHost
			var best *response

			for _, r := range tt.responses {
				if best == nil {
					best = r
					continue
				}

				// Priority 1: Always prefer responses without errors
				if r.Error == nil && best.Error != nil {
					best = r
					continue
				}

				// Priority 2: Among responses with errors, prefer partial responses over complete timeouts
				if r.Error != nil && best.Error != nil {
					if !r.Status.NoResponse && best.Status.NoResponse {
						best = r
						continue
					}
				}

				// Priority 3: Among equivalent response types, compare RTT (only if both have valid RTT)
				if r.Error == nil && best.Error == nil {
					// Both are valid responses - compare RTT
					if r.Status.Rtt != nil && best.Status.Rtt != nil &&
						r.Status.Rtt.AsDuration() < best.Status.Rtt.AsDuration() {
						best = r
					}
				} else if r.Error != nil && best.Error != nil &&
					r.Status.NoResponse == best.Status.NoResponse {
					// Both have same error type - compare RTT if available
					if r.Status.Rtt != nil && best.Status.Rtt != nil &&
						r.Status.Rtt.AsDuration() < best.Status.Rtt.AsDuration() {
						best = r
					}
				}
			}

			// Verify the selected response matches expectations
			if (best.Error != nil) != tt.expected.hasError {
				t.Errorf("Expected hasError=%v, got hasError=%v", tt.expected.hasError, best.Error != nil)
			}

			if best.Status.NoResponse != tt.expected.noResponse {
				t.Errorf("Expected NoResponse=%v, got NoResponse=%v", tt.expected.noResponse, best.Status.NoResponse)
			}

			expectedRTT := time.Duration(tt.expected.rttMs) * time.Millisecond
			var actualRTT time.Duration
			if best.Status.Rtt != nil {
				actualRTT = best.Status.Rtt.AsDuration()
			}

			if actualRTT != expectedRTT {
				t.Errorf("Expected RTT=%v, got RTT=%v", expectedRTT, actualRTT)
			}
		})
	}
}

func TestResponseSelectionEdgeCases(t *testing.T) {
	t.Run("nil RTT handling", func(t *testing.T) {
		responses := []*response{
			{
				Status: &apiv2.ServerStatus{
					NoResponse: false,
					Rtt:        nil, // nil RTT
				},
				Error: nil,
			},
			{
				Status: &apiv2.ServerStatus{
					NoResponse: false,
					Rtt:        durationpb.New(50 * time.Millisecond),
				},
				Error: nil,
			},
		}

		var best *response
		for _, r := range responses {
			if best == nil {
				best = r
				continue
			}

			if r.Error == nil && best.Error == nil {
				if r.Status.Rtt != nil && best.Status.Rtt != nil &&
					r.Status.Rtt.AsDuration() < best.Status.Rtt.AsDuration() {
					best = r
				}
			}
		}

		// Should not panic - the logic should prefer non-nil RTT only if doing actual comparison
		// Since the first response has nil RTT, it becomes best and stays best
		// because the nil check prevents the comparison from happening
		if best != responses[0] {
			t.Error("Expected to select first response (nil RTT check prevents replacement)")
		}
	})

	t.Run("empty responses slice", func(t *testing.T) {
		var responses []*response
		var best *response

		for _, r := range responses {
			if best == nil {
				best = r
				continue
			}
		}

		if best != nil {
			t.Error("Expected best to remain nil for empty responses")
		}
	})
}
