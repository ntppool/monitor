package monitorsettings

import (
	"testing"
	"time"

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
