package scorer

import (
	"context"
	"database/sql"
	"fmt"
	"time"

	"github.com/prometheus/client_golang/prometheus"
	"go.ntppool.org/monitor/ntpdb"
	"go.ntppool.org/monitor/scorer/every"
	"go.ntppool.org/monitor/scorer/recentmedian"
	"golang.org/x/exp/slog"
)

const batchSize = 500
const mainScorer = "recentmedian"

type metrics struct {
	processed *prometheus.CounterVec
	errcount  prometheus.Counter
	runs      prometheus.Counter
}

type runner struct {
	ctx      context.Context
	dbconn   *sql.DB
	log      *slog.Logger
	registry map[string]*ScorerMap
	m        *metrics
}

type lastUpdate struct {
	ts    time.Time
	score float64
}

func New(ctx context.Context, log *slog.Logger, dbconn *sql.DB, prom prometheus.Registerer) (*runner, error) {

	reg := map[string]*ScorerMap{
		"every":        {Scorer: every.New()},
		"recentmedian": {Scorer: recentmedian.New()},
	}

	for _, sm := range reg {
		sm.lastScore = map[int]*lastUpdate{}
	}

	if _, ok := reg[mainScorer]; !ok {
		log.Warn("invalid main scorer", "name", mainScorer)
	}

	met := &metrics{
		processed: prometheus.NewCounterVec(prometheus.CounterOpts{
			Name: "scorer_processed_count",
			Help: "log_scores processed",
		}, []string{"scorer"}),
		errcount: prometheus.NewCounter(prometheus.CounterOpts{
			Name: "scorer_errors",
			Help: "scorer errors",
		}),
		runs: prometheus.NewCounter(prometheus.CounterOpts{
			Name: "scorer_runs",
			Help: "scorer batches executed",
		}),
	}

	prom.MustRegister(met.processed)
	prom.MustRegister(met.errcount)
	prom.MustRegister(met.runs)

	return &runner{
		ctx:      ctx,
		dbconn:   dbconn,
		registry: reg,
		log:      log,
		m:        met,
	}, nil
}

// Scorers returns a map of name and a ScorerMap for each
// active scorer
func (r *runner) Scorers() map[string]*ScorerMap {
	return r.registry
}

func (r *runner) Run() (int, error) {

	r.m.runs.Add(1)

	log := r.log

	registry := r.Scorers()

	db := ntpdb.New(r.dbconn)

	scorers, err := db.GetScorers(r.ctx)
	if err != nil {
		r.m.errcount.Add(1)
		return 0, err
	}
	if len(scorers) == 0 {
		return 0, fmt.Errorf("no scorers configured")
	}

	for _, sc := range scorers {
		log.Debug("setting up scorer", "name", sc.Name, "last_id", sc.LogScoreID)
		if s, ok := registry[sc.Name]; ok {
			s.Scorer.Setup(sc.ID)
			s.ScorerID = sc.ID
			s.LastID = sc.LogScoreID
		} else {
			log.Warn("scorer not implemented", "name", sc.Name)
		}
	}

	count := 0

	for name, sm := range registry {
		log = log.With("name", name)
		if sm.ScorerID == 0 {
			continue
		}
		log.Debug("processing", "from_id", sm.LastID)
		scount, err := r.process(name, sm)
		r.m.processed.WithLabelValues(name).Add(float64(scount))
		count += scount
		if err != nil {
			log.Error("process error", "err", err)
			r.m.errcount.Add(1)
			return count, err
		}
	}

	return count, nil
}

func (r *runner) process(name string, sm *ScorerMap) (int, error) {

	log := r.log.With("name", name)

	tx, err := r.dbconn.BeginTx(r.ctx, nil)
	if err != nil {
		return 0, err
	}
	defer tx.Rollback()

	count := 0

	db := ntpdb.New(r.dbconn).WithTx(tx)

	// log.Printf("getting log scores from %d (limit %d)", sm.LastID, batchSize)

	t1 := time.Now()
	logscores, err := db.GetScorerLogScores(r.ctx,
		ntpdb.GetScorerLogScoresParams{
			LogScoreID: sm.LastID,
			Limit:      batchSize,
		})
	if err != nil {
		return 0, err
	}

	logfn := log.Debug
	dur := time.Since(t1)
	if dur > 5*time.Second {
		logfn = log.Warn
	}
	logfn("got scores", "count", len(logscores), "time", dur)

	count = len(logscores)

	if count == 0 {
		return 0, nil
	}

	for _, ls := range logscores {
		ss, err := r.getServerScore(db, ls.ServerID, sm.ScorerID)
		if err != nil {
			return 0, err
		}
		if ss.Status != "active" {
			// if we are calculating a score, it's active ...
			db.UpdateServerScoreStatus(r.ctx, ntpdb.UpdateServerScoreStatusParams{
				ServerID:  ls.ServerID,
				MonitorID: sm.ScorerID,
				Status:    "active",
			})
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
		LogScoreID: latestID,
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

// getServerScore returns the current server score for the serverID and monitorID.
// If none currently exists, a new score with default values is inserted and returned.
func (r *runner) getServerScore(db *ntpdb.Queries, serverID, monitorID uint32) (ntpdb.ServerScore, error) {

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
