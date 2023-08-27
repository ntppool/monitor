package cmd

import (
	"context"
	"database/sql"
	"time"

	"github.com/cenkalti/backoff/v4"
	"github.com/prometheus/client_golang/prometheus"
	"github.com/spf13/cobra"

	"go.ntppool.org/common/logger"
	"go.ntppool.org/common/metricsserver"
	"go.ntppool.org/monitor/ntpdb"
	"go.ntppool.org/monitor/scorer"
)

func (cli *CLI) scorerCmd() *cobra.Command {

	var scorerCmd = &cobra.Command{
		Use:   "scorer",
		Short: "scorer execution",
	}

	scorerCmd.PersistentFlags().AddGoFlagSet(cli.Config.Flags())

	scorerCmd.AddCommand(
		&cobra.Command{
			Use:   "run",
			Short: "scorer run once",
			RunE:  cli.Run(cli.scorer),
		})

	scorerCmd.AddCommand(
		&cobra.Command{
			Use:   "server",
			Short: "run continously",
			RunE:  cli.Run(cli.scorerServer),
		})

	scorerCmd.AddCommand(
		&cobra.Command{
			Use:   "setup",
			Short: "setup scorers",
			RunE:  cli.Run(cli.scorerSetup),
		})

	return scorerCmd
}

func (cli *CLI) scorerServer(cmd *cobra.Command, args []string) error {
	return cli.scorerRun(cmd, args, true)
}

func (cli *CLI) scorer(cmd *cobra.Command, args []string) error {
	return cli.scorerRun(cmd, args, false)
}

func (cli *CLI) scorerRun(cmd *cobra.Command, args []string, continuous bool) error {

	log := logger.Setup()
	log.Info("starting", "continuous", continuous)

	ctx := context.Background()

	metricssrv := metricsserver.New()
	go func() {
		err := metricssrv.ListenAndServe(ctx, 9000)
		if err != nil {
			log.Error("metricssrv", "err", err)
		}
	}()

	dbconn, err := ntpdb.OpenDB(cli.Config.Database)
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
	expback.MaxElapsedTime = 0

	for {
		count, err := sc.Run()
		if err != nil {
			log.Error("run error", "err", err, "count", count)
			return err
		}
		if count > 0 || !continuous {
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

func (cli *CLI) scorerSetup(cmd *cobra.Command, args []string) error {

	ctx := context.Background()
	log := logger.Setup()

	dbconn, err := ntpdb.OpenDB(cli.Config.Database)
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
		log.Info("setting up scorer", "name", name)

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
