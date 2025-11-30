package recentmedian

import (
	"context"
	"encoding/json"
	"fmt"

	"github.com/jackc/pgx/v5/pgtype"
	"golang.org/x/exp/slices"

	"go.ntppool.org/common/logger"
	"go.ntppool.org/monitor/ntpdb"
	"go.ntppool.org/monitor/scorer/score"
)

type RecentMedian struct {
	scorerID int64
}

func New() *RecentMedian {
	return &RecentMedian{}
}

func (s *RecentMedian) Setup(id int64) {
	s.scorerID = id
}

func (s *RecentMedian) Score(ctx context.Context, db *ntpdb.Queries, serverScore ntpdb.ServerScore, latest ntpdb.LogScore) (score.Score, error) {
	log := logger.Setup()

	if s.scorerID == 0 {
		return score.Score{}, fmt.Errorf("RecentMedian not Setup()")
	}

	// log.Printf("median, processing ls: %d", latest.ID)

	arg := ntpdb.GetScorerRecentScoresParams{
		TimeLookback:  1200,
		ServerID:      serverScore.ServerID,
		MonitorStatus: ntpdb.ServerScoresStatusActive,
		Ts:            latest.Ts,
	}

	log.Debug("getting recent scores", "serverID", serverScore.ServerID, "Ts", latest.Ts)

	recent, err := db.GetScorerRecentScores(ctx, arg)
	if err != nil {
		return score.Score{}, err
	}

	if len(recent) == 0 {
		arg.TimeLookback = 2700
		arg.MonitorStatus2 = ntpdb.NullServerScoresStatus{ServerScoresStatus: ntpdb.ServerScoresStatusTesting, Valid: true}
		recent, err = db.GetScorerRecentScores(ctx, arg)
		if err != nil {
			return score.Score{}, err
		}
	}

	// If still no scores, also include candidate monitors (they should still be monitoring)
	if len(recent) == 0 {
		arg.TimeLookback = 7200 // Extend lookback to 2 hours
		arg.MonitorStatus = ntpdb.ServerScoresStatusCandidate
		arg.MonitorStatus2 = ntpdb.NullServerScoresStatus{ServerScoresStatus: ntpdb.ServerScoresStatusTesting, Valid: true}
		recent, err = db.GetScorerRecentScores(ctx, arg)
		if err != nil {
			return score.Score{}, err
		}
	}

	if len(recent) == 0 {
		return score.Score{}, fmt.Errorf("no recent scores found for %d", serverScore.ServerID)
	}

	var ls ntpdb.LogScore

	if len(recent) < 3 {
		ls = recent[0]
	} else {

		slices.SortStableFunc(recent, func(a, b ntpdb.LogScore) int {
			if a.Score != b.Score {
				if a.Score > b.Score {
					return 1
				}
				if a.Score < b.Score {
					return -1
				}
			}
			if a.Rtt.Int32 > b.Rtt.Int32 {
				return 1
			}
			if a.Rtt.Int32 < b.Rtt.Int32 {
				return -1
			}
			return 0
		})

		i := len(recent) / 2
		if len(recent)%2 == 0 {
			i--
		}
		ls = recent[i]

	}

	attributes := ntpdb.LogScoreAttributes{}

	if ls.Attributes.Valid {
		err := json.Unmarshal([]byte(ls.Attributes.String), &attributes)
		if err != nil {
			return score.Score{
				LogScore: ntpdb.LogScore{
					ServerID:  ls.ServerID,
					MonitorID: pgtype.Int8{Valid: true, Int64: s.scorerID},
					Ts:        latest.Ts,
				},
			}, err
		}
	}

	attributes.FromLSID = int(latest.ID)
	attributes.FromSSID = int(serverScore.ID)
	b, err := json.Marshal(attributes)
	if err != nil {
		log.Error("could not marshal attributes", "attributes", attributes, "err", err)
	}
	attributeStr := pgtype.Text{
		String: string(b),
		Valid:  true,
	}

	// log.Printf("inserting median from LS %d", ls.ID)

	ns := score.Score{
		LogScore: ntpdb.LogScore{
			ServerID:   ls.ServerID,
			MonitorID:  pgtype.Int8{Valid: true, Int64: s.scorerID},
			Ts:         latest.Ts,
			Step:       ls.Step,
			Score:      ls.Score,
			Attributes: attributeStr,
			// Offset:     ls.Offset,
			// Rtt:        ls.Rtt,
		},
	}

	return ns, nil
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
