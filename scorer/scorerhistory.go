package scorer

import (
	"math"
	"time"

	"go.ntppool.org/monitor/ntpdb"
	"go.ntppool.org/monitor/scorer/types"
)

type ScorerMap struct {
	Scorer       types.Scorer
	ScorerID     uint32
	LastID       uint64
	lastScore    map[int]*lastUpdate // governs IsNew / log_scores cadence
	lastComputed map[int]*lastUpdate // governs skipIteration / compute cadence; durable (committed)
}

var minScoreInterval = 15 * time.Minute

const (
	percentageSimilarityThreshold = 0.05 // 5%
	fastIntervalRatio             = 3    // 1/3 of minScoreInterval
)

func (sm *ScorerMap) IsNew(ls *ntpdb.LogScore) bool {
	if last, ok := sm.lastScore[int(ls.ServerID)]; ok {
		// Skip out-of-order scores
		if ls.Ts.Before(last.ts) {
			return false
		}

		// 20.000 != 20.000 so test it with allowance for that ...
		if almostEqual(ls.Score, last.score) {
			if last.ts.Add(minScoreInterval).After(ls.Ts) {
				// we recorded the same score recently enough
				return false
			}
		}

		// also ignore if within 5% and within a third of the min interval
		percentageClose := sm.isPercentageClose(ls.Score, last.score)
		if percentageClose && last.ts.Add(minScoreInterval/fastIntervalRatio).After(ls.Ts) {
			return false
		}
	}

	sm.lastScore[int(ls.ServerID)] = &lastUpdate{
		ts:    ls.Ts,
		score: ls.Score,
	}

	return true
}

const (
	nearRealtimeLag     = 5 * time.Minute  // below this, never skip iteration
	catchUpLagThreshold = time.Hour        // above this, allow wider freshness window
	iterationWindowNear = 5 * time.Minute  // freshness window when 5min–1hr behind
	iterationWindowFar  = 18 * time.Minute // freshness window when >1hr behind
)

// skipIteration reports whether the full per-iteration compute
// (GetServerScore, Scorer.Score, UpdateServerScore) can be skipped for ls.
//
// Returns false when lag < nearRealtimeLag so steady-state behavior is
// unchanged. Returns true only when (a) the last compute for this server
// was within the freshness window for the current lag tier, and (b) the
// score is within 5% of the last computed score, so significant
// degradations still trigger compute. Does not mutate state.
//
// batchLocal holds ran-iteration entries written in the current batch but
// not yet committed. Consulted first so within-batch dedup works without
// mutating the durable map before db.Commit succeeds.
//
// This is orthogonal to IsNew: when an iteration runs, IsNew governs the
// log_scores INSERT exactly as today.
func (sm *ScorerMap) skipIteration(ls *ntpdb.LogScore, batchLocal map[int]lastUpdate) bool {
	lag := time.Since(ls.Ts)
	if lag < nearRealtimeLag {
		return false
	}
	var last lastUpdate
	var found bool
	if local, ok := batchLocal[int(ls.ServerID)]; ok {
		last, found = local, true
	} else if durable, ok := sm.lastComputed[int(ls.ServerID)]; ok {
		last, found = *durable, true
	}
	if !found {
		return false
	}
	window := iterationWindowNear
	if lag > catchUpLagThreshold {
		window = iterationWindowFar
	}
	return ls.Ts.Sub(last.ts) < window &&
		sm.isPercentageClose(ls.Score, last.score)
}

// commitComputed merges a batch-local set of ran-iteration entries into
// the durable lastComputed map. Called by the runner only after
// db.Commit succeeds, so a rolled-back batch leaves lastComputed
// untouched and the next attempt re-runs the same work.
func (sm *ScorerMap) commitComputed(batchLocal map[int]lastUpdate) {
	for sid, u := range batchLocal {
		sm.lastComputed[sid] = &lastUpdate{ts: u.ts, score: u.score}
	}
}

const float64EqualityThreshold = 1e-12

func almostEqual(a, b float64) bool {
	return math.Abs(a-b) <= float64EqualityThreshold
}

func (sm *ScorerMap) isPercentageClose(score1, score2 float64) bool {
	if score1 == 0 && score2 == 0 {
		return true
	}
	if score1 == 0 || score2 == 0 {
		return almostEqual(score1, score2)
	}
	return math.Abs(score1-score2) <= math.Abs(score2)*percentageSimilarityThreshold
}
