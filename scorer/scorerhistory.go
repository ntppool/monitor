package scorer

import (
	"math"
	"time"

	"go.ntppool.org/monitor/ntpdb"
	"go.ntppool.org/monitor/scorer/types"
)

type ScorerMap struct {
	Scorer    types.Scorer
	ScorerID  int32
	LastID    int64
	lastScore map[int]*lastUpdate
}

var minScoreInterval = 10 * time.Minute

func (sm *ScorerMap) IsNew(ls *ntpdb.LogScore) bool {

	if last, ok := sm.lastScore[int(ls.ServerID)]; ok {
		// 20.000 != 20.000 so test it with allowance for that ...
		if almostEqual(ls.Score, last.score) {
			if last.ts.Add(minScoreInterval).After(ls.Ts) {
				// we recorded the same score recently enough
				return false
			}
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
