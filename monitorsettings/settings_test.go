package monitorsettings

import (
	"encoding/json"
	"testing"
	"time"

	"go.ntppool.org/common/timeutil"
	"go.ntppool.org/monitor/testutil"
)

func TestRequiredCountForActive(t *testing.T) {
	tests := []struct {
		name     string
		interval time.Duration
		window   time.Duration
		want     int64
	}{
		{"30m configured default - keeps historical 32", 30 * time.Minute, 24 * time.Hour, 32},
		{"25m over 24h", 25 * time.Minute, 24 * time.Hour, 32},
		{"45m old default - drops below 32", 45 * time.Minute, 24 * time.Hour, 26}, // ceil(0.8 * 32) = 26
		{"50m broken old config - drops below 32", 50 * time.Minute, 24 * time.Hour, 23},
		{"60m hourly - half of historical", 60 * time.Minute, 24 * time.Hour, 20}, // ceil(0.8 * 24) = 20
		{"15m fast - capped at 32", 15 * time.Minute, 24 * time.Hour, 32},         // ceil(0.8 * 96) = 77, capped
		{"5m very fast - capped at 32", 5 * time.Minute, 24 * time.Hour, 32},      // ceil(0.8 * 288) = 231, capped
		{"4h slow - floors at 8", 4 * time.Hour, 24 * time.Hour, 8},               // ceil(0.8 * 6) = 5, floored
		{"60m over 72h - capped at 32", 60 * time.Minute, 72 * time.Hour, 32},     // ceil(0.8 * 72) = 58, capped
		{"0 falls back to historical", 0, 24 * time.Hour, 32},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := RequiredCountForActive(tt.interval, tt.window)
			testutil.AssertEqual(t, tt.want, got, "RequiredCountForActive(%v, %v)", tt.interval, tt.window)
		})
	}
}

func TestRequiredCountForTesting(t *testing.T) {
	tests := []struct {
		name     string
		interval time.Duration
		window   time.Duration
		want     int64
	}{
		{"2h over 24h", 2 * time.Hour, 24 * time.Hour, 9}, // 12 possible, ceil(9.6)=10, floor 9
		{"4h over 72h", 4 * time.Hour, 72 * time.Hour, 9}, // 18 possible, ceil(14.4)=15, floor 9
		{"4h over 24h", 4 * time.Hour, 24 * time.Hour, 8}, // 6 possible, ceil(4.8)=5, absolute floor 8 > 6: unreachable
		{"0 falls back to historical", 0, 24 * time.Hour, 9},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := RequiredCountForTesting(tt.interval, tt.window)
			testutil.AssertEqual(t, tt.want, got, "RequiredCountForTesting(%v, %v)", tt.interval, tt.window)
		})
	}
}

func TestMaxSamplesInWindow(t *testing.T) {
	tests := []struct {
		name     string
		interval time.Duration
		window   time.Duration
		want     int64
	}{
		{"30m over 24h", 30 * time.Minute, 24 * time.Hour, 48},
		{"45m over 24h", 45 * time.Minute, 24 * time.Hour, 32},
		{"50m over 24h", 50 * time.Minute, 24 * time.Hour, 28},
		{"60m over 24h", 60 * time.Minute, 24 * time.Hour, 24},
		{"2h over 24h", 2 * time.Hour, 24 * time.Hour, 12},
		{"2h over 72h", 2 * time.Hour, 72 * time.Hour, 36},
		{"0 interval", 0, 24 * time.Hour, 0},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := MaxSamplesInWindow(tt.interval, tt.window)
			testutil.AssertEqual(t, tt.want, got, "MaxSamplesInWindow(%v, %v)", tt.interval, tt.window)
		})
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

		IntervalCandidate: timeutil.Duration{Duration: 4 * time.Hour},
		SampleWindow:      timeutil.Duration{Duration: 72 * time.Hour},
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

// A hand-typed interval_candidate or sample_window below the sane minimum
// must fall back to the default, never hand out a row on every poll.
func TestWithDefaultsCandidateAndWindow(t *testing.T) {
	tests := []struct {
		name          string
		raw           string
		wantCandidate time.Duration
		wantWindow    time.Duration
	}{
		{"missing", `{}`, DefaultIntervalCandidate, DefaultSampleWindow},
		{"candidate 0s", `{"interval_candidate": "0s"}`, DefaultIntervalCandidate, DefaultSampleWindow},
		{"candidate 1m", `{"interval_candidate": "1m"}`, DefaultIntervalCandidate, DefaultSampleWindow},
		{"candidate 9m", `{"interval_candidate": "9m"}`, DefaultIntervalCandidate, DefaultSampleWindow},
		{"candidate 10m kept", `{"interval_candidate": "10m"}`, 10 * time.Minute, DefaultSampleWindow},
		{"candidate 4h kept", `{"interval_candidate": "4h"}`, 4 * time.Hour, DefaultSampleWindow},
		{"window 0s", `{"sample_window": "0s"}`, DefaultIntervalCandidate, DefaultSampleWindow},
		{"window 30m", `{"sample_window": "30m"}`, DefaultIntervalCandidate, DefaultSampleWindow},
		{"window 1h kept", `{"sample_window": "1h"}`, DefaultIntervalCandidate, time.Hour},
		{"window 72h kept", `{"sample_window": "72h"}`, DefaultIntervalCandidate, 72 * time.Hour},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			var s MonitorSettings
			testutil.AssertNoError(t, json.Unmarshal([]byte(tt.raw), &s), "Unmarshal(%s)", tt.raw)
			got := s.WithDefaults()
			testutil.AssertEqual(t, tt.wantCandidate, got.IntervalCandidate.Duration, "IntervalCandidate for %s", tt.raw)
			testutil.AssertEqual(t, tt.wantWindow, got.SampleWindow.Duration, "SampleWindow for %s", tt.raw)
		})
	}
}

func TestAllowLeapAnnouncements(t *testing.T) {
	tests := []struct {
		raw  string
		want bool
	}{
		{`{"interval_testing": "30m"}`, false},
		{`{"allow_leap_announcements": false}`, false},
		{`{"allow_leap_announcements": true}`, true},
	}
	for _, tt := range tests {
		var s MonitorSettings
		if err := json.Unmarshal([]byte(tt.raw), &s); err != nil {
			t.Fatalf("Unmarshal(%s): %v", tt.raw, err)
		}
		if got := s.WithDefaults().AllowLeapAnnouncements; got != tt.want {
			t.Errorf("%s: AllowLeapAnnouncements = %v, want %v", tt.raw, got, tt.want)
		}
	}
}
