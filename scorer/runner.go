package scorer

import (
	"context"
	"database/sql"
	"fmt"
	"log"
	"time"

	"go.ntppool.org/monitor/ntpdb"
	"go.ntppool.org/monitor/scorer/every"
	"go.ntppool.org/monitor/scorer/recentmedian"
)

const batchSize = 2000
const mainScorer = "recentmedian"

type runner struct {
	ctx      context.Context
	dbconn   *sql.DB
	registry map[string]*ScorerMap
}

type lastUpdate struct {
	ts    time.Time
	score float64
}

func New(ctx context.Context, dbconn *sql.DB) (*runner, error) {

	m := map[string]*ScorerMap{
		"every":        {Scorer: every.New()},
		"recentmedian": {Scorer: recentmedian.New()},
	}

	for _, sm := range m {
		sm.lastScore = map[int]*lastUpdate{}
	}

	if _, ok := m[mainScorer]; !ok {
		log.Printf("invalid main scorer %s", mainScorer)
	}

	return &runner{
		ctx:      ctx,
		dbconn:   dbconn,
		registry: m,
	}, nil
}

func (r *runner) Scorers() map[string]*ScorerMap {
	return r.registry
}

func (r *runner) Run() (int, error) {

	registry := r.Scorers()

	db := ntpdb.New(r.dbconn)

	scorers, err := db.GetScorers(r.ctx)
	if err != nil {
		return 0, err
	}

	for _, sc := range scorers {
		log.Printf("setting up scorer: %s (last ls id: %d)", sc.Name, sc.LogScoreID.Int64)
		if s, ok := registry[sc.Name]; ok {
			s.Scorer.Setup(sc.ID)
			s.ScorerID = sc.ID
			s.LastID = sc.LogScoreID.Int64
		} else {
			log.Printf("scorer %q not implemented", sc.Name)
		}
	}

	count := 0

	for name, sm := range registry {
		if sm.ScorerID == 0 {
			continue
		}
		log.Printf("processing %q from %d", name, sm.LastID)
		scount, err := r.process(name, sm)
		count += scount
		if err != nil {
			log.Printf("process error: %s", err)
			return count, err
		}
	}

	return count, nil
}

func (r *runner) process(name string, sm *ScorerMap) (int, error) {

	tx, err := r.dbconn.BeginTx(r.ctx, nil)
	if err != nil {
		return 0, err
	}
	defer tx.Rollback()

	count := 0

	db := ntpdb.New(r.dbconn).WithTx(tx)

	// log.Printf("getting log scores from %d (limit %d)", sm.LastID, batchSize)

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

		if sm.IsNew(&ls) {
			// only store the new calculated score in log_scores if it's
			// changed (or we haven't for 10 minutes)

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
			_, err = db.InsertLogScore(r.ctx, p)
			if err != nil {
				return 0, err
			}
		}

		err = db.UpdateServerScore(r.ctx, ntpdb.UpdateServerScoreParams{
			ID:       ss.ID,
			ScoreRaw: ns.Score,
			ScoreTs:  sql.NullTime{Time: ns.Ts, Valid: true},
		})
		if err != nil {
			return 0, err
		}

		if name == mainScorer {
			err := db.UpdateServer(r.ctx, ntpdb.UpdateServerParams{
				ID:       ns.ServerID,
				ScoreTs:  sql.NullTime{Time: ns.Ts, Valid: true},
				ScoreRaw: ns.Score,
			})
			if err != nil {
				return 0, err
			}
		}

	}

	// b, err := json.MarshalIndent(newScores, "", "  ")
	// if err != nil {
	// 	log.Printf("could not json encode: %s", err)
	// }
	// fmt.Printf("%s\n", b)

	latestID := logscores[len(logscores)-1].ID
	// log.Printf("updating scorer status %d, new latest id: %d", sm.ScorerID, latestID)
	err = db.UpdateScorerStatus(r.ctx, ntpdb.UpdateScorerStatusParams{
		LogScoreID: sql.NullInt64{Int64: latestID, Valid: true},
		ScorerID:   sm.ScorerID,
	})
	if err != nil {
		return 0, err
	}

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
