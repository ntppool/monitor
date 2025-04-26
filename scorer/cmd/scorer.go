package cmd

import (
	"context"
	"database/sql"
	"time"

	"github.com/cenkalti/backoff/v5"
	"github.com/prometheus/client_golang/prometheus"

	"go.ntppool.org/common/logger"
	"go.ntppool.org/common/metricsserver"
	"go.ntppool.org/monitor/ntpdb"
	"go.ntppool.org/monitor/scorer"
)

func (cmd *scorerOnceCmd) Run(ctx context.Context) error {
	return scorerRun(ctx, false)
}

func (cmd *scorerServerCmd) Run(ctx context.Context) error {
	return scorerRun(ctx, true)
}

func scorerRun(ctx context.Context, continuous bool) error {
	log := logger.FromContext(ctx)
	log.Info("starting", "continuous", continuous)

	metricssrv := metricsserver.New()
	go func() {
		err := metricssrv.ListenAndServe(ctx, 9000)
		if err != nil {
			log.Error("metricssrv", "err", err)
		}
	}()

	dbconn, err := ntpdb.OpenDB()
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

	for {
		count, err := sc.Run(ctx)
		if err != nil {
			log.Error("run error", "err", err, "count", count)
			return err
		}
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

func (cmd *scorerSetupCmd) Run(ctx context.Context) error {
	log := logger.FromContext(ctx)

	dbconn, err := ntpdb.OpenDB()
	if err != nil {
		return err
	}

	tx, err := dbconn.BeginTx(ctx, nil)
	if err != nil {
		return err
	}
	defer tx.Rollback()

	db := ntpdb.New(dbconn).WithTx(tx)

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
		existingScorers[dbS.Name] = true
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

		insert, err := db.InsertScorer(ctx, ntpdb.InsertScorerParams{
			Name:    name,
			TlsName: sql.NullString{String: name + ".scores.ntp.dev", Valid: true},
		})
		if err != nil {
			return err
		}
		scorerID, err := insert.LastInsertId()
		if err != nil {
			return err
		}
		db.InsertScorerStatus(ctx, ntpdb.InsertScorerStatusParams{
			ScorerID:   uint32(scorerID),
			LogScoreID: minLogScoreID,
		})

	}

	err = tx.Commit()
	if err != nil {
		return err
	}

	return nil
}
