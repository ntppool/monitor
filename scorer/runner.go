package scorer

import (
	"context"
	"database/sql"
	"encoding/json"
	"fmt"
	"log"
	"time"

	"go.ntppool.org/monitor/ntpdb"
	"go.ntppool.org/monitor/scorer/every"
	"go.ntppool.org/monitor/scorer/score"
	"go.ntppool.org/monitor/scorer/types"
)

const batchSize = 500

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

func (r *runner) Run() error {

	registry := map[string]ScorerMap{}

	db := ntpdb.New(r.dbconn)

	scorers, err := db.GetScorers(r.ctx)
	if err != nil {
		return err
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

	for name, sm := range registry {
		log.Printf("processing %q from %d", name, sm.LastID)
		err := r.process(name, sm)
		if err != nil {
			return err
		}
	}

	return nil
}

func (r *runner) process(name string, sm ScorerMap) error {

	tx, err := r.dbconn.BeginTx(r.ctx, nil)
	if err != nil {
		return err
	}
	defer tx.Rollback()

	db := ntpdb.New(r.dbconn).WithTx(tx)

	logscores, err := db.GetScorerLogScores(r.ctx,
		ntpdb.GetScorerLogScoresParams{
			LogScoreID: sm.LastID,
			Limit:      batchSize,
		})
	if err != nil {
		return err
	}
	log.Printf("got %d scores", len(logscores))

	newScores := []score.Score{}

	for _, ls := range logscores {
		ss, err := r.getServerScore(db, ls.ServerID, ls.MonitorID.Int32)
		if err != nil {
			return err
		}
		ns, err := sm.Scorer.Score(r.ctx, db, ss, ls)
		if err != nil {
			return fmt.Errorf("scorer %q: %s", name, err)
		}
		newScores = append(newScores, ns)
	}

	for _, s := range newScores {

		p := ntpdb.InsertLogScoreParams{
			ServerID:   s.ServerID,
			MonitorID:  s.MonitorID,
			Ts:         s.Ts,
			Step:       s.Step,
			Offset:     s.Offset,
			Rtt:        s.Rtt,
			Score:      s.Score,
			Attributes: s.Attributes,
		}
		err := db.InsertLogScore(r.ctx, p)
		if err != nil {
			return err
		}
		err = db.UpdateServerScore(r.ctx, ntpdb.UpdateServerScoreParams{})
		if err != nil {
			return err
		}

	}

	b, err := json.MarshalIndent(newScores, "", "  ")
	if err != nil {
		log.Printf("could not json encode: %s", err)
	}
	fmt.Printf("%s\n", b)

	db.UpdateScorerStatus(r.ctx, ntpdb.UpdateScorerStatusParams{
		LogScoreID: sql.NullInt64{Int64: logscores[len(logscores)-1].ID, Valid: true},
		ScorerID:   sm.ScorerID,
	})

	err = tx.Commit()
	if err != nil {
		return err
	}

	return nil
}

func (r *runner) getServerScore(db *ntpdb.Queries, serverID, monitorID int32) (ntpdb.ServerScore, error) {

	ctx := r.ctx

	p := ntpdb.GetServerScoreParams{
		ServerID:  serverID,
		MonitorID: monitorID,
	}

	log.Printf("get server score for server id: %d", serverID)

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
