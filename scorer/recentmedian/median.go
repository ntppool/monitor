package recentmedian

import (
	"context"
	"database/sql"
	"fmt"

	"go.ntppool.org/monitor/ntpdb"
	"go.ntppool.org/monitor/scorer/score"
	"golang.org/x/exp/slices"
)

type RecentMedian struct {
	scorerID int32
}

func New() *RecentMedian {
	return &RecentMedian{}
}
func (s *RecentMedian) Setup(id int32) {
	s.scorerID = id
}

func (s *RecentMedian) Score(ctx context.Context, db *ntpdb.Queries, serverScore ntpdb.ServerScore, latest ntpdb.LogScore) (score.Score, error) {

	if s.scorerID == 0 {
		return score.Score{}, fmt.Errorf("RecentMedian not Setup()")
	}

	arg := ntpdb.GetScorerRecentScoresParams{}
	arg.TimeLookback = 900
	arg.ServerID = serverScore.ServerID
	arg.Ts = latest.Ts

	// log.Printf("getting recent scores: %+v", arg)

	recent, err := db.GetScorerRecentScores(ctx, arg)
	if err != nil {
		return score.Score{}, err
	}

	var ls ntpdb.LogScore

	if len(recent) < 3 {
		ls = recent[0]
	} else {
		// log.Printf("%d recent scores", len(recent))

		// js, _ := json.MarshalIndent(recent, "", "    ")
		// log.Printf("recent 1: %s", js)

		slices.SortStableFunc(recent, func(a, b ntpdb.LogScore) bool {
			return a.Score > b.Score || a.Rtt.Int32 < b.Rtt.Int32
		})

		// js, _ = json.MarshalIndent(recent, "", "    ")
		// log.Printf("recent 2: %s", js)

		i := len(recent) / 2
		if len(recent)%2 == 0 {
			i--
		}
		ls = recent[i]

	}

	return score.Score{
		LogScore: ntpdb.LogScore{
			ServerID:   ls.ServerID,
			MonitorID:  sql.NullInt32{Valid: true, Int32: int32(s.scorerID)},
			Ts:         ls.Ts,
			Step:       ls.Step,
			Offset:     ls.Offset,
			Rtt:        ls.Rtt,
			Score:      ls.Score,
			Attributes: ls.Attributes,
		},
	}, nil

}

/*

My suggestion would be:
- for each monitor and each server there is a 1-score updated as soon
 as new data is received from the monitor
- the overall-score of a server is updated whenever a 1-score of the
 server is updated
- the overall-score is set to the median score from all 1-scores that
 were updated at least once in some interval (e.g. last hour)

Even with as few as 3 monitors I think this would be more robust that
the current system using only 1 monitor. With more monitors the
percentile could be moved up or down if people felt that too many or
too few servers were blocked in the pool, but I think the median would
be a very good start.

As the next improvement there could be some weighting of the 1-scores
based on the network delay between the monitor and server, so local
monitors in a zone have a higher weight in the overall score. This
should wait until there is a larger number of monitors.

*/
