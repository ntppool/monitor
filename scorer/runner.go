package scorer

import (
	"context"
	"database/sql"
	"fmt"
	"log"
	"time"

	"go.ntppool.org/monitor/ntpdb"
	"go.ntppool.org/monitor/scorer/every"
	"go.ntppool.org/monitor/scorer/types"
)

const batchSize = 5000

type runner struct {
	ctx    context.Context
	dbconn *sql.DB
}

type ScorerMap struct {
	Scorer   types.Scorer
	ScorerID int32
	LastID   int64
}

func New(ctx context.Context, dbconn *sql.DB) (*runner, error) {
	return &runner{
		ctx:    ctx,
		dbconn: dbconn,
	}, nil
}

func (r *runner) Run() (int, error) {

	registry := map[string]ScorerMap{}

	db := ntpdb.New(r.dbconn)

	scorers, err := db.GetScorers(r.ctx)
	if err != nil {
		return 0, err
	}

	for _, sc := range scorers {
		log.Printf("setting up scorer: %s (last ls id: %d)", sc.Name, sc.LogScoreID.Int64)
		var s types.Scorer
		switch sc.Name {
		case "every":
			s = every.New(sc.ID)
		default:
			log.Printf("scorer %q not implemented", sc.Name)
		}

		registry[sc.Name] = ScorerMap{
			Scorer:   s,
			ScorerID: sc.ID,
			LastID:   sc.LogScoreID.Int64,
		}
	}

	count := 0

	for name, sm := range registry {
		log.Printf("processing %q from %d", name, sm.LastID)
		scount, err := r.process(name, sm)
		count += scount
		if err != nil {
			return count, err
		}
	}

	return count, nil
}

func (r *runner) process(name string, sm ScorerMap) (int, error) {

	tx, err := r.dbconn.BeginTx(r.ctx, nil)
	if err != nil {
		return 0, err
	}
	defer tx.Rollback()

	count := 0

	db := ntpdb.New(r.dbconn).WithTx(tx)

	logscores, err := db.GetScorerLogScores(r.ctx,
		ntpdb.GetScorerLogScoresParams{
			LogScoreID: sm.LastID,
			Limit:      batchSize,
		})
	if err != nil {
		return 0, err
	}
	log.Printf("got %d scores", len(logscores))

	count = len(logscores)

	if count == 0 {
		return 0, nil
	}

	for _, ls := range logscores {
		ss, err := r.getServerScore(db, ls.ServerID, sm.ScorerID)
		if err != nil {
			return 0, err
		}
		ns, err := sm.Scorer.Score(r.ctx, db, ss, ls)
		if err != nil {
			return 0, fmt.Errorf("scorer %q: %s", name, err)
		}

		p := ntpdb.InsertLogScoreParams{
			ServerID:   ns.ServerID,
			MonitorID:  ns.MonitorID,
			Ts:         ns.Ts,
			Step:       ns.Step,
			Offset:     ns.Offset,
			Rtt:        ns.Rtt,
			Score:      ns.Score,
			Attributes: ns.Attributes,
		}
		err = db.InsertLogScore(r.ctx, p)
		if err != nil {
			return 0, err
		}

		// log.Printf("updating server score id %d to score %.3f", ss.ID, ns.Score)

		err = db.UpdateServerScore(r.ctx, ntpdb.UpdateServerScoreParams{
			ID:       ss.ID,
			ScoreRaw: ns.Score,
			ScoreTs:  sql.NullTime{Time: ns.Ts, Valid: true},
		})
		if err != nil {
			return 0, err
		}
	}

	// b, err := json.MarshalIndent(newScores, "", "  ")
	// if err != nil {
	// 	log.Printf("could not json encode: %s", err)
	// }
	// fmt.Printf("%s\n", b)

	db.UpdateScorerStatus(r.ctx, ntpdb.UpdateScorerStatusParams{
		LogScoreID: sql.NullInt64{Int64: logscores[len(logscores)-1].ID, Valid: true},
		ScorerID:   sm.ScorerID,
	})

	err = tx.Commit()
	if err != nil {
		return 0, err
	}

	return count, nil
}

func (r *runner) getServerScore(db *ntpdb.Queries, serverID, monitorID int32) (ntpdb.ServerScore, error) {

	ctx := r.ctx

	p := ntpdb.GetServerScoreParams{
		ServerID:  serverID,
		MonitorID: monitorID,
	}

	// log.Printf("get server score for server id: %d", serverID)

	serverScore, err := db.GetServerScore(ctx, p)
	if err == nil {
		return serverScore, nil
	}

	// only if there's an error
	if err != sql.ErrNoRows {
		return serverScore, err
	}

	// ErrNoRows
	err = db.InsertServerScore(ctx, ntpdb.InsertServerScoreParams{
		ServerID:  p.ServerID,
		MonitorID: p.MonitorID,
		ScoreRaw:  -5,
		CreatedOn: time.Now(),
	})
	if err != nil {
		return serverScore, err
	}

	return db.GetServerScore(ctx, p)

}