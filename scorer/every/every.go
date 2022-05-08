package every

import (
	"context"
	"database/sql"

	"go.ntppool.org/monitor/ntpdb"
	"go.ntppool.org/monitor/scorer/score"
)

type EveryScore struct {
	scorerID int32
}

func New(id int32) *EveryScore {
	return &EveryScore{scorerID: id}
}

func (s *EveryScore) Score(ctx context.Context, db *ntpdb.Queries, serverScore ntpdb.ServerScore, ls ntpdb.LogScore) (score.Score, error) {

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
