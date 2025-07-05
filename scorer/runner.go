package scorer

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"log/slog"
	"time"

	"github.com/prometheus/client_golang/prometheus"
	"go.ntppool.org/monitor/ntpdb"
	"go.ntppool.org/monitor/scorer/every"
	"go.ntppool.org/monitor/scorer/recentmedian"
)

const (
	defaultBatchSize = 500
	mainScorer       = "recentmedian"
)

type ScorerSettings struct {
	BatchSize int32 `json:"batch_size"`
}

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

func (r *runner) Settings(db *ntpdb.Queries) ScorerSettings {
	settingsStr, err := db.GetSystemSetting(r.ctx, "scorer")
	if err != nil {
		r.log.Warn("could not fetch scorer settings", "err", err)
	}
	var settings ScorerSettings
	if len(settingsStr) > 0 {
		err := json.Unmarshal([]byte(settingsStr), &settings)
		if err != nil {
			r.log.Warn("could not unmarshal scorer settings", "err", err)
		}
	}

	if settings.BatchSize == 0 {
		settings.BatchSize = defaultBatchSize
	}

	return settings
}

func (r *runner) Run(ctx context.Context) (int, error) {
	r.m.runs.Add(1)
	log := r.log

	db := ntpdb.New(r.dbconn)

	settings := r.Settings(db)

	registry := r.Scorers()

	scorers, err := db.GetScorers(r.ctx)
	if err != nil {
		r.m.errcount.Add(1)
		return 0, err
	}
	if len(scorers) == 0 {
		return 0, fmt.Errorf("no scorers configured")
	}

	for _, sc := range scorers {
		log.Debug("setting up scorer", "name", sc.Hostname, "last_id", sc.LogScoreID)
		if s, ok := registry[sc.Hostname]; ok {
			s.Scorer.Setup(sc.ID)
			s.ScorerID = sc.ID
			s.LastID = sc.LogScoreID
		} else {
			log.Warn("scorer not implemented", "name", sc.Hostname)
		}
	}

	count := 0

	for name, sm := range registry {
		log = log.With("name", name)
		if sm.ScorerID == 0 {
			continue
		}
		log.DebugContext(ctx, "processing", "from_id", sm.LastID)
		scount, err := r.process(ctx, name, sm, settings.BatchSize)
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

func (r *runner) getLogScores(ctx context.Context, db *ntpdb.Queries, log *slog.Logger, lastID uint64, batchSize int32, retry bool) ([]ntpdb.LogScore, error) {
	// log.Printf("getting log scores from %d (limit %d)", sm.LastID, batchSize)

	t1 := time.Now()
	logscores, err := db.GetScorerLogScores(r.ctx,
		ntpdb.GetScorerLogScoresParams{
			LogScoreID: lastID,
			Limit:      batchSize,
		})
	if err != nil {
		return nil, err
	}

	logfn := log.Debug
	dur := time.Since(t1)
	if dur > 5*time.Second {
		logfn = log.Warn
	}

	logfn("got scores", "count", len(logscores), "time", dur)

	if len(logscores) == 0 && !retry {
		logfn("checking for minimum id")
		minID, err := db.GetScorerNextLogScoreID(ctx, lastID)
		if err != nil {
			if errors.Is(err, sql.ErrNoRows) {
				// we're at the end of the queue
				return []ntpdb.LogScore{}, nil
			}
			return nil, err
		}
		return r.getLogScores(ctx, db, log, minID, batchSize, true)
	}

	return logscores, nil
}

func (r *runner) process(ctx context.Context, name string, sm *ScorerMap, batchSize int32) (int, error) {
	log := r.log.With("name", name)

	tx, err := r.dbconn.BeginTx(r.ctx, nil)
	if err != nil {
		return 0, err
	}
	defer func() {
		_ = tx.Rollback()
	}()

	count := 0

	db := ntpdb.New(r.dbconn).WithTx(tx)

	logscores, err := r.getLogScores(ctx, db, log, sm.LastID, batchSize, false)
	if err != nil {
		return 0, err
	}

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
			if err := db.UpdateServerScoreStatus(r.ctx, ntpdb.UpdateServerScoreStatusParams{
				ServerID:  ls.ServerID,
				MonitorID: sm.ScorerID,
				Status:    "active",
			}); err != nil {
				return 0, fmt.Errorf("updating server score status: %w", err)
			}
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
