package cmd

import (
	"context"
	"strings"
	"time"

	"github.com/cenkalti/backoff/v5"
	"github.com/jackc/pgx/v5/pgtype"
	"github.com/prometheus/client_golang/prometheus"

	"go.ntppool.org/common/database"
	"go.ntppool.org/common/logger"
	"go.ntppool.org/common/metricsserver"
	"go.ntppool.org/common/version"
	"go.ntppool.org/monitor/ntpdb"
	"go.ntppool.org/monitor/scorer"
)

func (cmd *scorerOnceCmd) Run(ctx context.Context) error {
	return scorerRun(ctx, cmd.ConfigFile, false, cmd.MetricsPort)
}

func (cmd *scorerServerCmd) Run(ctx context.Context) error {
	return scorerRun(ctx, cmd.ConfigFile, true, cmd.MetricsPort)
}

func scorerRun(ctx context.Context, configFile string, continuous bool, metricsPort int) error {
	log := logger.FromContext(ctx)
	log.InfoContext(ctx, "starting monitor-scorer", "version", version.Version(), "continuous", continuous)

	metricssrv := metricsserver.New()
	version.RegisterMetric("scorer", metricssrv.Registry())
	go func() {
		err := metricssrv.ListenAndServe(ctx, metricsPort)
		if err != nil {
			log.Error("metrics server error", "err", err)
		}
	}()

	dbconn, err := ntpdb.OpenDB(ctx, configFile)
	if err != nil {
		return err
	}

	sc, err := scorer.New(ctx, log, dbconn, metricssrv.Registry())
	if err != nil {
		return nil
	}

	expback := backoff.NewExponentialBackOff()
	expback.InitialInterval = time.Second * 3
	expback.MaxInterval = time.Second * 60

	dbErrorBackoff := backoff.NewExponentialBackOff()
	dbErrorBackoff.InitialInterval = time.Second * 5
	dbErrorBackoff.MaxInterval = time.Second * 120

	for {
		count, err := sc.Run(ctx)
		if err != nil {
			log.Error("run error", "err", err, "count", count)

			// Check if this is a database connection error
			if isConnectionError(err) {
				if !continuous {
					// In once mode, still fail on connection errors
					return err
				}

				// In continuous mode, retry with exponential backoff
				wait := dbErrorBackoff.NextBackOff()
				log.Warn("database connection error, retrying", "wait", wait)
				time.Sleep(wait)
				continue
			}

			// For other errors, fail immediately
			return err
		}

		// Reset database error backoff on successful run
		dbErrorBackoff.Reset()

		if count > 0 || !continuous {
			// todo: add prom metric counter
			log.Debug("Processed log scores", "count", count)
		}

		if !continuous {
			break
		}

		if count == 0 {
			sl := expback.NextBackOff()
			time.Sleep(sl)
		} else {
			expback.Reset()
		}

	}

	return nil
}

// isConnectionError checks if an error is a database connection error
// that should trigger a retry rather than a fatal exit
func isConnectionError(err error) bool {
	if err == nil {
		return false
	}

	errStr := err.Error()

	// Check for common connection error patterns
	connectionErrors := []string{
		"driver: bad connection",
		"invalid connection",
		"connection refused",
		"connection reset by peer",
		"broken pipe",
		"EOF",
		"i/o timeout",
		"network is unreachable",
		"no such host",
		"connection timed out",
		"Too many connections",
		"connection refused",
	}

	for _, pattern := range connectionErrors {
		if strings.Contains(errStr, pattern) {
			return true
		}
	}

	return false
}

func (cmd *scorerSetupCmd) Run(ctx context.Context) error {
	log := logger.FromContext(ctx)

	dbconn, err := ntpdb.OpenDB(ctx, cmd.ConfigFile)
	if err != nil {
		return err
	}

	db := ntpdb.New(dbconn)
	err = database.WithTransaction(ctx, db, func(ctx context.Context, db ntpdb.QuerierTx) error {
		scr, err := scorer.New(ctx, log, dbconn, prometheus.DefaultRegisterer)
		if err != nil {
			return err
		}

		dbScorers, err := db.GetScorers(ctx)
		if err != nil {
			return err
		}
		existingScorers := map[string]bool{}

		for _, dbS := range dbScorers {
			existingScorers[dbS.Hostname] = true
		}

		log.Debug("dbScorers", "scorers", dbScorers)

		codeScorers := scr.Scorers()

		minLogScoreID, err := db.GetMinLogScoreID(ctx)
		if err != nil {
			return err
		}

		for name := range codeScorers {
			if _, ok := existingScorers[name]; ok {
				log.Info("scorer already configured", "name", name)
				continue
			}
			log.Info("setting up scorer, scorerSetup", "name", name)

			scorerID, err := db.InsertScorer(ctx, ntpdb.InsertScorerParams{
				Hostname: name,
				TlsName:  pgtype.Text{String: name + ".scores.ntp.dev", Valid: true},
			})
			if err != nil {
				return err
			}
			if err := db.InsertScorerStatus(ctx, ntpdb.InsertScorerStatusParams{
				ScorerID:   scorerID,
				LogScoreID: minLogScoreID,
			}); err != nil {
				log.WarnContext(ctx, "Failed to insert scorer status", "err", err)
			}

		}

		return nil
	})
	if err != nil {
		return err
	}

	return nil
}
