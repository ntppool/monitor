package cmd

import (
	"context"
	"database/sql"
	"errors"
	"fmt"

	"github.com/alecthomas/kong"

	"go.ntppool.org/monitor/ntpdb"
)

type dbCmd struct {
	ScorerStatus bool `cmd:"" help:"Show scorer status"`
}

func (cmd *dbCmd) Run(ctx context.Context, kctx *kong.Context) error {
	dbconn, err := ntpdb.OpenDB()
	if err != nil {
		return err
	}
	db := ntpdb.New(dbconn)

	ss, err := db.GetScorerStatus(ctx)
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			fmt.Println("No scorers found")
			return nil
		}
		return err
	}

	for _, s := range ss {
		// todo: get scorer name, too
		fmt.Printf("%-5d %-20s %-10d %s\n", s.ScorerID, s.Name, s.LogScoreID, s.ModifiedOn)
	}

	return nil
}
