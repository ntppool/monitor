package scorer

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"log/slog"
	"strings"
	"time"

	"github.com/cenkalti/backoff/v5"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgconn"
	"github.com/jackc/pgx/v5/pgtype"
	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/prometheus/client_golang/prometheus"

	"go.ntppool.org/monitor/ntpdb"
	"go.ntppool.org/monitor/scorer/every"
	"go.ntppool.org/monitor/scorer/recentmedian"
	"go.opentelemetry.io/otel"
	"go.opentelemetry.io/otel/attribute"
)

const (
	defaultBatchSize      = 50
	mainScorer            = "recentmedian"
	deadlockRetryDuration = 10 * time.Minute // Continue retrying for 10 minutes total
	initialDeadlockDelay  = 5 * time.Second  // Start with 5-second delay
	maxDeadlockDelay      = 60 * time.Second // Cap at 60 seconds between attempts
)

type ScorerSettings struct {
	BatchSize int32 `json:"batch_size"`
}

type metrics struct {
	processed          *prometheus.CounterVec
	skipped            *prometheus.CounterVec
	scoreErrorsSkipped *prometheus.CounterVec
	errcount           prometheus.Counter
	runs               prometheus.Counter
	batchTime          *prometheus.HistogramVec
	batchSize          *prometheus.HistogramVec
	deadlocks          prometheus.Counter
	retries            *prometheus.CounterVec
	sqlUpdates         *prometheus.CounterVec
	lastScoreTs        *prometheus.GaugeVec
	lastBatchTs        *prometheus.GaugeVec
}

type runner struct {
	dbconn   *pgxpool.Pool
	log      *slog.Logger
	registry map[string]*ScorerMap
	m        *metrics
}

type lastUpdate struct {
	ts    time.Time
	score float64
}

func New(log *slog.Logger, dbconn *pgxpool.Pool, prom prometheus.Registerer) (*runner, error) {
	reg := map[string]*ScorerMap{
		"every":        {Scorer: every.New()},
		"recentmedian": {Scorer: recentmedian.New()},
	}

	for _, sm := range reg {
		sm.lastScore = map[int]*lastUpdate{}
		sm.lastComputed = map[int]*lastUpdate{}
	}

	if _, ok := reg[mainScorer]; !ok {
		log.Warn("invalid main scorer", "name", mainScorer)
	}

	met := &metrics{
		processed: prometheus.NewCounterVec(prometheus.CounterOpts{
			Name: "scorer_processed_count",
			Help: "log_scores processed",
		}, []string{"scorer"}),
		skipped: prometheus.NewCounterVec(prometheus.CounterOpts{
			Name: "scorer_iterations_skipped_total",
			Help: "Iterations skipped because the score hasn't materially changed since the last compute. Fires only when lagging behind real time.",
		}, []string{"scorer"}),
		scoreErrorsSkipped: prometheus.NewCounterVec(prometheus.CounterOpts{
			Name: "scorer_score_errors_skipped_total",
			Help: "Scorer.Score errors that were logged and skipped instead of failing the batch, by reason",
		}, []string{"scorer", "reason"}),
		errcount: prometheus.NewCounter(prometheus.CounterOpts{
			Name: "scorer_errors",
			Help: "scorer errors",
		}),
		runs: prometheus.NewCounter(prometheus.CounterOpts{
			Name: "scorer_runs",
			Help: "scorer batches executed",
		}),
		batchTime: prometheus.NewHistogramVec(prometheus.HistogramOpts{
			Name:    "scorer_batch_duration_seconds",
			Help:    "Time taken to process each batch",
			Buckets: prometheus.ExponentialBuckets(0.01, 2, 10), // 10ms to ~10s
		}, []string{"scorer"}),
		batchSize: prometheus.NewHistogramVec(prometheus.HistogramOpts{
			Name:    "scorer_batch_size",
			Help:    "Number of records processed per batch",
			Buckets: prometheus.LinearBuckets(10, 10, 20), // 10 to 200 records
		}, []string{"scorer"}),
		deadlocks: prometheus.NewCounter(prometheus.CounterOpts{
			Name: "scorer_deadlocks_total",
			Help: "Total number of database deadlocks encountered",
		}),
		retries: prometheus.NewCounterVec(prometheus.CounterOpts{
			Name: "scorer_retries_total",
			Help: "Total number of retry attempts by reason",
		}, []string{"scorer", "reason"}),
		sqlUpdates: prometheus.NewCounterVec(prometheus.CounterOpts{
			Name: "scorer_sql_updates_total",
			Help: "Total number of SQL update operations by type",
		}, []string{"operation"}),
		lastScoreTs: prometheus.NewGaugeVec(prometheus.GaugeOpts{
			Name: "scorer_last_score_timestamp_seconds",
			Help: "Unix timestamp of the most recent log_score processed in the latest batch",
		}, []string{"scorer"}),
		lastBatchTs: prometheus.NewGaugeVec(prometheus.GaugeOpts{
			Name: "scorer_last_batch_timestamp_seconds",
			Help: "Unix wall-clock time of the most recent non-empty batch. Stale value indicates upstream is not producing log_scores.",
		}, []string{"scorer"}),
	}

	prom.MustRegister(met.processed)
	prom.MustRegister(met.skipped)
	prom.MustRegister(met.scoreErrorsSkipped)
	prom.MustRegister(met.errcount)
	prom.MustRegister(met.runs)
	prom.MustRegister(met.batchTime)
	prom.MustRegister(met.batchSize)
	prom.MustRegister(met.deadlocks)
	prom.MustRegister(met.retries)
	prom.MustRegister(met.sqlUpdates)
	prom.MustRegister(met.lastScoreTs)
	prom.MustRegister(met.lastBatchTs)

	return &runner{
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

// pool returns an autocommit Querier wrapped with OpenTelemetry tracing.
// Calling Begin on the result yields a re-wrapped transaction-scoped querier.
func (r *runner) pool() ntpdb.QuerierTx {
	return ntpdb.NewWrappedQuerier(ntpdb.New(r.dbconn))
}

func (r *runner) Settings(ctx context.Context, db ntpdb.Querier) ScorerSettings {
	settingsRaw, err := db.GetSystemSetting(ctx, "scorer")
	if err != nil {
		r.log.Warn("could not fetch scorer settings", "err", err)
	}
	var settings ScorerSettings
	if len(settingsRaw) > 0 {
		err := json.Unmarshal(settingsRaw, &settings)
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

	db := r.pool()

	settings := r.Settings(ctx, db)

	registry := r.Scorers()

	scorers, err := db.GetScorers(ctx)
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

		// Process with deadlock retry logic
		var scount int
		var err error
		startTime := time.Now()

		operation := func() (int, error) {
			attemptCount, attemptErr := r.process(ctx, name, sm, settings.BatchSize)
			if attemptErr != nil && isDeadlockError(attemptErr) {
				// Record deadlock metrics but continue retrying
				r.m.deadlocks.Inc()
				r.m.retries.WithLabelValues(name, "deadlock").Inc()

				elapsed := time.Since(startTime)
				log.Warn("deadlock detected, will retry",
					"err", attemptErr,
					"elapsed", elapsed,
					"max_duration", deadlockRetryDuration)
				return 0, attemptErr // Return error to trigger retry
			}
			return attemptCount, attemptErr // Return success or non-deadlock error
		}

		// Configure exponential backoff for deadlock retries
		bo := backoff.NewExponentialBackOff()
		bo.InitialInterval = initialDeadlockDelay
		bo.MaxInterval = maxDeadlockDelay
		bo.Multiplier = 1.5 // More gradual increase than default 2.0

		// Execute with retries using max elapsed time option
		scount, err = backoff.Retry(ctx, operation,
			backoff.WithBackOff(bo),
			backoff.WithMaxElapsedTime(deadlockRetryDuration))

		r.m.processed.WithLabelValues(name).Add(float64(scount))
		count += scount
		if err != nil {
			elapsed := time.Since(startTime)
			if isDeadlockError(err) {
				r.m.deadlocks.Inc()
				log.Error("deadlock error after retry period expired",
					"err", err,
					"retry_duration", deadlockRetryDuration,
					"elapsed", elapsed)
			} else {
				log.Error("process error", "err", err)
			}
			r.m.errcount.Add(1)
			return count, err
		}
	}

	return count, nil
}

func (r *runner) getLogScores(ctx context.Context, db ntpdb.Querier, log *slog.Logger, lastID int64, batchSize int32, retry bool) ([]ntpdb.LogScore, error) {
	// log.Printf("getting log scores from %d (limit %d)", sm.LastID, batchSize)

	t1 := time.Now()
	logscores, err := db.GetScorerLogScores(ctx,
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
			if errors.Is(err, pgx.ErrNoRows) {
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
	tracer := otel.Tracer("monitor/scorer")
	ctx, span := tracer.Start(ctx, "scorer.process_batch")
	defer span.End()

	span.SetAttributes(
		attribute.String("scorer.name", name),
		attribute.Int("scorer.batch_size", int(batchSize)),
		attribute.Int("scorer.last_id", int(sm.LastID)),
	)

	startTime := time.Now()
	log := r.log.With("name", name)

	// Validate connection before starting transaction
	if err := r.validateConnection(ctx); err != nil {
		span.RecordError(err)
		return 0, fmt.Errorf("connection validation failed: %w", err)
	}

	db, err := r.pool().Begin(ctx)
	if err != nil {
		return 0, err
	}
	committed := false
	defer func() {
		if !committed {
			_ = db.Rollback(ctx)
		}
	}()

	count := 0
	pending := map[int64]lastUpdate{}
	localComputed := map[int]lastUpdate{}

	logscores, err := r.getLogScores(ctx, db, log, sm.LastID, batchSize, false)
	if err != nil {
		return 0, err
	}

	count = len(logscores)

	if count == 0 {
		return 0, nil
	}

	for _, ls := range logscores {
		if name == mainScorer && sm.skipIteration(&ls, localComputed) {
			r.m.skipped.WithLabelValues(name).Inc()
			continue
		}

		ss, err := r.getServerScore(ctx, db, ls.ServerID, sm.ScorerID)
		if err != nil {
			return 0, err
		}
		if ss.Status != "active" {
			// if we are calculating a score, it's active ...
			if err := db.UpdateServerScoreStatus(ctx, ntpdb.UpdateServerScoreStatusParams{
				ServerID:  ls.ServerID,
				MonitorID: sm.ScorerID,
				Status:    "active",
			}); err != nil {
				return 0, fmt.Errorf("updating server score status: %w", err)
			}
			r.m.sqlUpdates.WithLabelValues("update_server_score_status").Inc()
		}
		ns, err := sm.Scorer.Score(ctx, db, ss, ls)
		if err != nil {
			if shouldSkipScoreError(err, ls.Ts.Time, time.Now()) {
				log.WarnContext(
					ctx, "could not calculate score, skipping entry",
					"server_id", ls.ServerID, "log_score_id", ls.ID,
					"ls_ts", ls.Ts.Time.String(), "err", err,
				)
				r.m.scoreErrorsSkipped.WithLabelValues(name, scoreErrorReason(err)).Inc()
				continue
			}
			return 0, fmt.Errorf("scorer %q: %s", name, err)
		}

		if sm.IsNew(&ls) {
			// only store the new calculated score in log_scores if it's
			// changed (or we haven't for a number of minutes)

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
			_, err = db.InsertLogScore(ctx, p)
			if err != nil {
				return 0, err
			}
			r.m.sqlUpdates.WithLabelValues("insert_log_score").Inc()
		}

		err = db.UpdateServerScore(ctx, ntpdb.UpdateServerScoreParams{
			ID:       ss.ID,
			ScoreRaw: ns.Score,
			ScoreTs:  pgtype.Timestamptz{Time: ns.Ts.Time, Valid: true},
		})
		if err != nil {
			return 0, err
		}
		r.m.sqlUpdates.WithLabelValues("update_server_score").Inc()

		if name == mainScorer {
			if cur, ok := pending[ns.ServerID]; !ok || ns.Ts.Time.After(cur.ts) {
				pending[ns.ServerID] = lastUpdate{ts: ns.Ts.Time, score: ns.Score}
			}
			localComputed[int(ls.ServerID)] = lastUpdate{ts: ls.Ts.Time, score: ls.Score}
		}
	}

	// b, err := json.MarshalIndent(newScores, "", "  ")
	// if err != nil {
	// 	log.Printf("could not json encode: %s", err)
	// }
	// fmt.Printf("%s\n", b)

	latestID := logscores[len(logscores)-1].ID
	// log.Printf("updating scorer status %d, new latest id: %d", sm.ScorerID, latestID)
	err = db.UpdateScorerStatus(ctx, ntpdb.UpdateScorerStatusParams{
		LogScoreID: latestID,
		ScorerID:   sm.ScorerID,
	})
	if err != nil {
		return 0, err
	}
	r.m.sqlUpdates.WithLabelValues("update_scorer_status").Inc()

	err = db.Commit(ctx)
	if err != nil {
		span.RecordError(err)
		// Check if this is a deadlock error
		if isDeadlockError(err) {
			r.m.deadlocks.Inc()
		}
		return 0, err
	}
	committed = true
	sm.commitComputed(localComputed)

	// Drain pending servers.score_ts updates with autocommit, one row per
	// server. UpdateServer's WHERE clause (score_ts < ? OR score_ts IS NULL)
	// is idempotent, so individual failures (including deadlocks) self-heal:
	// the next batch moves score_ts forward for that server. This keeps
	// `servers` X-locks held for ~1ms per statement instead of the full
	// batch duration, which is what was blocking concurrent InsertLogScore
	// from monitor clients via the log_scores_server FK.
	//
	// Deliberately sequential: parallelizing would reintroduce the lock
	// storm we just eliminated and inflate connection-pool pressure (the
	// scorer shares the pool with the API). A batched multi-row update
	// is awkward because of the per-row score_ts guard.
	if len(pending) > 0 {
		pool := r.pool()
		deduped := count - len(pending)
		for id, u := range pending {
			uerr := pool.UpdateServer(ctx, ntpdb.UpdateServerParams{
				ID:       id,
				ScoreTs:  pgtype.Timestamptz{Time: u.ts, Valid: true},
				ScoreRaw: u.score,
			})
			if uerr != nil {
				if isDeadlockError(uerr) {
					r.m.deadlocks.Inc()
				}
				log.WarnContext(ctx, "post-commit UpdateServer failed",
					"server_id", id, "err", uerr)
				r.m.errcount.Add(1)
				continue
			}
			r.m.sqlUpdates.WithLabelValues("update_server").Inc()
		}
		if deduped > 0 {
			r.m.sqlUpdates.WithLabelValues("update_server_deduped").Add(float64(deduped))
		}
		span.SetAttributes(attribute.Int("scorer.deduped_count", deduped))
	}

	// Record successful batch metrics
	duration := time.Since(startTime)
	r.m.batchTime.WithLabelValues(name).Observe(duration.Seconds())
	r.m.batchSize.WithLabelValues(name).Observe(float64(count))
	newestTs := logscores[len(logscores)-1].Ts
	r.m.lastScoreTs.WithLabelValues(name).Set(float64(newestTs.Time.Unix()))
	r.m.lastBatchTs.WithLabelValues(name).Set(float64(time.Now().Unix()))

	span.SetAttributes(
		attribute.Int("scorer.processed_count", count),
		attribute.Float64("scorer.duration_seconds", duration.Seconds()),
	)

	return count, nil
}

// getServerScore returns the current server score for the serverID and monitorID.
// If none currently exists, a new score with default values is inserted and returned.
func (r *runner) getServerScore(ctx context.Context, db ntpdb.Querier, serverID, monitorID int64) (ntpdb.ServerScore, error) {
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
	if !errors.Is(err, pgx.ErrNoRows) {
		return serverScore, err
	}

	// ErrNoRows
	err = db.InsertServerScore(ctx, ntpdb.InsertServerScoreParams{
		ServerID:  p.ServerID,
		MonitorID: p.MonitorID,
		ScoreRaw:  -5,
		CreatedOn: pgtype.Timestamptz{Time: time.Now(), Valid: true},
	})
	if err != nil {
		return serverScore, err
	}
	r.m.sqlUpdates.WithLabelValues("insert_server_score").Inc()

	return db.GetServerScore(ctx, p)
}

// validateConnection performs a lightweight health check on the database connection
func (r *runner) validateConnection(ctx context.Context) error {
	// Simple ping to verify connection is alive
	ctx, cancel := context.WithTimeout(ctx, 5*time.Second)
	defer cancel()

	return r.dbconn.Ping(ctx)
}

// shouldSkipScoreError reports whether a Scorer.Score error should be logged
// and skipped rather than failing the whole batch. ErrNoRecentScores is an
// expected data-availability gap (e.g. a poorly-monitored server) and is
// always skippable; other errors are only skipped once the log_score is old
// enough that they're presumed to be unprocessed backlog rather than a live
// bug worth failing loudly on.
func shouldSkipScoreError(err error, ts, now time.Time) bool {
	if errors.Is(err, recentmedian.ErrNoRecentScores) {
		return true
	}
	return ts.Before(now.Add(-3 * time.Hour))
}

// scoreErrorReason labels a skipped score error for the scorer_score_errors_skipped_total metric.
func scoreErrorReason(err error) string {
	if errors.Is(err, recentmedian.ErrNoRecentScores) {
		return "no_recent_scores"
	}
	return "stale_entry"
}

// isDeadlockError checks if the error is a PostgreSQL deadlock error (SQLSTATE 40P01)
func isDeadlockError(err error) bool {
	if err == nil {
		return false
	}
	// PostgreSQL deadlock error has SQLSTATE 40P01
	var pgErr *pgconn.PgError
	if errors.As(err, &pgErr) {
		return pgErr.Code == "40P01" // deadlock_detected
	}
	return errors.Is(err, pgx.ErrTxClosed) ||
		(err.Error() != "" && containsIgnoreCase(err.Error(), "deadlock"))
}

// containsIgnoreCase performs case-insensitive substring search
func containsIgnoreCase(s, substr string) bool {
	return len(s) >= len(substr) &&
		(len(substr) == 0 ||
			strings.Contains(strings.ToLower(s), strings.ToLower(substr)))
}
