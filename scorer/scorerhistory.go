package scorer

import (
	"math"
	"time"

	"go.ntppool.org/monitor/ntpdb"
	"go.ntppool.org/monitor/scorer/types"
)

type ScorerMap struct {
	Scorer    types.Scorer
	ScorerID  uint32
	LastID    uint64
	lastScore map[int]*lastUpdate
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
