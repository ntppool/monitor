// Package monitorsettings provides the live MonitorSettings shared between
// the monitor-api server and the selector, including derived sample-count
// thresholds used by the selector's promotion rules.
package monitorsettings

import (
	"math"
	"time"

	"go.ntppool.org/common/timeutil"
)

// MonitorSettings is the JSON payload stored under system_settings(key="monitors").
type MonitorSettings struct {
	IntervalActive  timeutil.Duration `json:"interval_active"`
	IntervalTesting timeutil.Duration `json:"interval_testing"`
	IntervalAll     timeutil.Duration `json:"interval_all"`
	BatchSize       int32             `json:"batch_size"`
}

// Default values applied to MonitorSettings when fields are unset or below
// the minimum sanity threshold. Kept in sync with the historical defaults
// that previously lived in server/functions.go.
const (
	DefaultIntervalActive  = 9 * time.Minute
	DefaultIntervalTesting = 30 * time.Minute
	DefaultIntervalAll     = 60 * time.Second
	DefaultBatchSize       = int32(10)

	minIntervalActive  = 20 * time.Second
	minIntervalTesting = 60 * time.Second
	minIntervalAll     = 10 * time.Second
)

// WithDefaults returns a copy of s with defaults applied where the
// configured value is missing or below the minimum sanity threshold.
func (s MonitorSettings) WithDefaults() MonitorSettings {
	if s.IntervalActive.Duration < minIntervalActive {
		s.IntervalActive = timeutil.Duration{Duration: DefaultIntervalActive}
	}
	if s.IntervalTesting.Duration < minIntervalTesting {
		s.IntervalTesting = timeutil.Duration{Duration: DefaultIntervalTesting}
	}
	if s.IntervalAll.Duration < minIntervalAll {
		s.IntervalAll = timeutil.Duration{Duration: DefaultIntervalAll}
	}
	if s.BatchSize <= 0 {
		s.BatchSize = DefaultBatchSize
	}
	return s
}

// Threshold constants for selector promotion rules.
//
// The historical floors (32, 9) are the statistical minimums originally
// chosen for promotion confidence. ThresholdPercent expresses the desired
// "monitor has been running for this fraction of the 24h sample window"
// liveness signal. The effective threshold is min(historicalFloor,
// ceil(ThresholdPercent * maxSamplesIn24h)) with an absolute floor so a
// pathological interval cannot drop the bar to zero.
const (
	HistoricalMinForActive  int64 = 32
	HistoricalMinForTesting int64 = 9

	ThresholdPercent = 0.80
	AbsoluteFloor    = int64(8)

	// candidatePollInterval matches the hardcoded "OR queue_ts < NOW() -
	// INTERVAL 120 minute" branch in the GetServers query, which is the
	// effective cadence cap for candidate-status server_scores rows.
	candidatePollInterval = 120 * time.Minute

	// SampleWindow is the trailing window over which log_scores are
	// counted in the GetMonitorPriority query.
	SampleWindow = 24 * time.Hour
)

// MaxSamplesInWindow returns the largest number of samples a single
// monitor can produce for a single server in SampleWindow, given that the
// re-query interval is at least `interval`.
func MaxSamplesInWindow(interval time.Duration) int64 {
	if interval <= 0 {
		return 0
	}
	return int64(SampleWindow / interval)
}

// RequiredCountForActive returns the minimum log_scores count required in
// the 24h window for a testing monitor to be eligible for promotion to
// active, given the configured interval_testing.
func RequiredCountForActive(intervalTesting time.Duration) int64 {
	return computeThreshold(intervalTesting, HistoricalMinForActive)
}

// RequiredCountForTesting returns the minimum log_scores count required
// for a candidate monitor to be eligible for promotion to testing.
//
// Candidates are not gated by interval_testing — they fall through to the
// hardcoded 120-minute branch in GetServers — so this value is effectively
// static unless that hardcoded cap changes.
func RequiredCountForTesting() int64 {
	return computeThreshold(candidatePollInterval, HistoricalMinForTesting)
}

func computeThreshold(interval time.Duration, historicalFloor int64) int64 {
	if interval <= 0 {
		return historicalFloor
	}
	max := MaxSamplesInWindow(interval)
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
