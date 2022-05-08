package types

import (
	"context"

	"go.ntppool.org/monitor/ntpdb"
	"go.ntppool.org/monitor/scorer/score"
)

type Scorer interface {
	// Lookback(LookbackOptions)
	Score(ctx context.Context, db *ntpdb.Queries, serverScore ntpdb.ServerScore, ls ntpdb.LogScore) (score.Score, error)
}
