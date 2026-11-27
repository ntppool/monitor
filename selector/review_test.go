package selector

import (
	"testing"
	"time"

	"go.ntppool.org/monitor/monitorsettings"
)

func TestNextReviewIn(t *testing.T) {
	s := monitorsettings.MonitorSettings{}.WithDefaults()

	tests := []struct {
		name string
		in   reviewOutcome
		want time.Duration
	}{
		{"normal, unchanged", reviewOutcome{}, 2 * time.Hour},
		{"normal, changed", reviewOutcome{Changed: true}, 4 * time.Hour},
		{"bootstrap, unchanged", reviewOutcome{Mode: serverMode{Bootstrap: true}}, time.Hour},
		{"bootstrap, changed", reviewOutcome{Changed: true, Mode: serverMode{Bootstrap: true}}, time.Hour},
		{"bootstrap and looks bad, changed", reviewOutcome{Changed: true, Mode: serverMode{Bootstrap: true, LooksBad: true}}, 4 * time.Hour},
		{"bootstrap and looks bad, unchanged", reviewOutcome{Mode: serverMode{Bootstrap: true, LooksBad: true}}, 2 * time.Hour},
		{"looks bad, not bootstrap, unchanged", reviewOutcome{Mode: serverMode{LooksBad: true}}, 2 * time.Hour},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := nextReviewIn(tt.in, s); got != tt.want {
				t.Errorf("nextReviewIn(%+v) = %v, want %v", tt.in, got, tt.want)
			}
		})
	}
}
