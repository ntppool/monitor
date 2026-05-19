package monitorsettings

import (
	"testing"
	"time"

	"go.ntppool.org/common/timeutil"
)

func TestRequiredCountForActive(t *testing.T) {
	tests := []struct {
		name     string
		interval time.Duration
		want     int64
	}{
		{"30m configured default - keeps historical 32", 30 * time.Minute, 32},
		{"45m old default - drops below 32", 45 * time.Minute, 26}, // ceil(0.8 * 32) = 26
		{"50m broken old config - drops below 32", 50 * time.Minute, 23},
		{"60m hourly - half of historical", 60 * time.Minute, 20}, // ceil(0.8 * 24) = 20
		{"15m fast - capped at 32", 15 * time.Minute, 32},         // ceil(0.8 * 96) = 77, capped
		{"5m very fast - capped at 32", 5 * time.Minute, 32},      // ceil(0.8 * 288) = 231, capped
		{"4h slow - floors at 8", 4 * time.Hour, 8},               // ceil(0.8 * 6) = 5, floored
		{"0 falls back to historical", 0, 32},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := RequiredCountForActive(tt.interval)
			if got != tt.want {
				t.Errorf("RequiredCountForActive(%v) = %d, want %d", tt.interval, got, tt.want)
			}
		})
	}
}

func TestRequiredCountForTesting(t *testing.T) {
	// Candidates are gated by the hardcoded 120-minute fallback in
	// GetServers, not by interval_testing. The threshold is therefore
	// effectively static and should match the historical 9.
	got := RequiredCountForTesting()
	if got != HistoricalMinForTesting {
		t.Errorf("RequiredCountForTesting() = %d, want %d", got, HistoricalMinForTesting)
	}
}

func TestMaxSamplesInWindow(t *testing.T) {
	tests := []struct {
		interval time.Duration
		want     int64
	}{
		{30 * time.Minute, 48},
		{45 * time.Minute, 32},
		{50 * time.Minute, 28},
		{60 * time.Minute, 24},
		{2 * time.Hour, 12},
		{0, 0},
	}
	for _, tt := range tests {
		got := MaxSamplesInWindow(tt.interval)
		if got != tt.want {
			t.Errorf("MaxSamplesInWindow(%v) = %d, want %d", tt.interval, got, tt.want)
		}
	}
}

func TestWithDefaults(t *testing.T) {
	zero := MonitorSettings{}.WithDefaults()
	if zero.IntervalActive.Duration != DefaultIntervalActive {
		t.Errorf("zero IntervalActive: got %v, want %v", zero.IntervalActive.Duration, DefaultIntervalActive)
	}
	if zero.IntervalTesting.Duration != DefaultIntervalTesting {
		t.Errorf("zero IntervalTesting: got %v, want %v", zero.IntervalTesting.Duration, DefaultIntervalTesting)
	}
	if zero.IntervalAll.Duration != DefaultIntervalAll {
		t.Errorf("zero IntervalAll: got %v, want %v", zero.IntervalAll.Duration, DefaultIntervalAll)
	}
	if zero.BatchSize != DefaultBatchSize {
		t.Errorf("zero BatchSize: got %d, want %d", zero.BatchSize, DefaultBatchSize)
	}

	// Values above the sanity minimums are preserved.
	in := MonitorSettings{
		IntervalActive:  timeutil.Duration{Duration: 7 * time.Minute},
		IntervalTesting: timeutil.Duration{Duration: 25 * time.Minute},
		IntervalAll:     timeutil.Duration{Duration: 40 * time.Second},
		BatchSize:       60,
	}
	out := in.WithDefaults()
	if out != in {
		t.Errorf("values above minimums should be preserved: got %+v, want %+v", out, in)
	}

	// Below-minimum values get defaulted.
	tooSmall := MonitorSettings{
		IntervalActive:  timeutil.Duration{Duration: 10 * time.Second},
		IntervalTesting: timeutil.Duration{Duration: 30 * time.Second},
		IntervalAll:     timeutil.Duration{Duration: 5 * time.Second},
		BatchSize:       -1,
	}
	fixed := tooSmall.WithDefaults()
	if fixed.IntervalActive.Duration != DefaultIntervalActive {
		t.Errorf("tooSmall IntervalActive not defaulted")
	}
	if fixed.IntervalTesting.Duration != DefaultIntervalTesting {
		t.Errorf("tooSmall IntervalTesting not defaulted")
	}
	if fixed.IntervalAll.Duration != DefaultIntervalAll {
		t.Errorf("tooSmall IntervalAll not defaulted")
	}
	if fixed.BatchSize != DefaultBatchSize {
		t.Errorf("tooSmall BatchSize not defaulted")
	}
}
