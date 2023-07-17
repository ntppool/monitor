package every

import (
	"context"
	"database/sql"
	"fmt"

	"go.ntppool.org/monitor/ntpdb"
	"go.ntppool.org/monitor/scorer/score"
)

type EveryScore struct {
	scorerID uint32
}

func New() *EveryScore {
	return &EveryScore{}
}

func (s *EveryScore) Setup(id uint32) {
	s.scorerID = id
}

func (s *EveryScore) Score(ctx context.Context, db *ntpdb.Queries, serverScore ntpdb.ServerScore, ls ntpdb.LogScore) (score.Score, error) {

	if s.scorerID == 0 {
		return score.Score{}, fmt.Errorf("EveryScore not Setup()")
	}

	// ctx, span := srv.tracer.Start(ctx, "Score")
	// defer span.End()

	scoreRaw := ls.Step + (serverScore.ScoreRaw * 0.95)

	maxscore, hasMaxScore := ls.MaxScore()

	if hasMaxScore {
		scoreRaw = maxscore
	}

	return score.Score{
		LogScore: ntpdb.LogScore{
			ServerID:   ls.ServerID,
			MonitorID:  sql.NullInt32{Valid: true, Int32: int32(s.scorerID)},
			Ts:         ls.Ts,
			Step:       ls.Step,
			Offset:     ls.Offset,
			Rtt:        ls.Rtt,
			Score:      scoreRaw,
			Attributes: ls.Attributes,
		},
		HasMaxScore: hasMaxScore,
		MaxScore:    maxscore,
	}, nil

}
