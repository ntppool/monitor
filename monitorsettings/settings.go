// Package monitorsettings provides the live MonitorSettings shared between
// the monitor-api server and the selector, including derived sample-count
// thresholds used by the selector's promotion rules. The payload, its
// defaults and minimums are in go.ntppool.org/common/monitorsettings.
package monitorsettings

import (
	"math"
	"time"

	shared "go.ntppool.org/common/monitorsettings"
)

// MonitorSettings is the JSON payload stored under system_settings(key="monitors").
type MonitorSettings = shared.MonitorSettings

// Threshold constants for selector promotion rules.
//
// The historical floors (32, 9) are the statistical minimums originally
// chosen for promotion confidence. ThresholdPercent expresses the desired
// "monitor has been running for this fraction of the sample window"
// liveness signal. The effective threshold is min(historicalFloor,
// ceil(ThresholdPercent * maxSamplesInWindow)) with an absolute floor so a
// pathological interval cannot drop the bar to zero.
const (
	HistoricalMinForActive  int64 = 32
	HistoricalMinForTesting int64 = 9

	ThresholdPercent = 0.80
	AbsoluteFloor    = int64(8)
)

// MaxSamplesInWindow returns the largest number of samples a single
// monitor can produce for a single server in window, given that the
// re-query interval is at least `interval`.
func MaxSamplesInWindow(interval, window time.Duration) int64 {
	if interval <= 0 {
		return 0
	}
	return int64(window / interval)
}

// RequiredCountForActive returns the minimum log_scores count required in
// the sample window for a testing monitor to be eligible for promotion to
// active, given the configured interval_testing.
func RequiredCountForActive(intervalTesting, window time.Duration) int64 {
	return computeThreshold(intervalTesting, window, HistoricalMinForActive)
}

// RequiredCountForTesting returns the minimum log_scores count required
// for a candidate monitor to be eligible for promotion to testing, given
// the configured interval_candidate.
func RequiredCountForTesting(intervalCandidate, window time.Duration) int64 {
	return computeThreshold(intervalCandidate, window, HistoricalMinForTesting)
}

func computeThreshold(interval, window time.Duration, historicalFloor int64) int64 {
	if interval <= 0 {
		return historicalFloor
	}
	max := MaxSamplesInWindow(interval, window)
	if max <= 0 {
		return AbsoluteFloor
	}
	pct := int64(math.Ceil(ThresholdPercent * float64(max)))
	threshold := min(pct, historicalFloor)
	if threshold < AbsoluteFloor {
		threshold = AbsoluteFloor
	}
	return threshold
}
