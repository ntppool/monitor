package cmd

import (
	"context"
	"errors"
	"fmt"

	"github.com/jackc/pgx/v5"

	"go.ntppool.org/monitor/ntpdb"
)

type dbCmd struct {
	ConfigFile   string `name:"config" short:"c" default:"database.yaml" help:"Database config file"`
	ScorerStatus bool   `cmd:"" help:"Show scorer status"`
}

func (cmd *dbCmd) Run(ctx context.Context) error {
	dbconn, err := ntpdb.OpenDB(ctx, cmd.ConfigFile)
	if err != nil {
		return err
	}
	db := ntpdb.New(dbconn)

	ss, err := db.GetScorerStatus(ctx)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			fmt.Println("No scorers found")
			return nil
		}
		return err
	}

	for _, s := range ss {
		// todo: get scorer name, too
		fmt.Printf("%-5d %-20s %-10d %s\n", s.ScorerID, s.Hostname, s.LogScoreID, s.ModifiedOn.Time)
	}

	return nil
}
