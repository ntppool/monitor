package cmd

import (
	"context"
	"fmt"

	"github.com/spf13/cobra"
	"go.ntppool.org/monitor/ntpdb"
)

func (cli *CLI) dbCmd() *cobra.Command {

	var dbCmd = &cobra.Command{
		Use:   "db",
		Short: "db utility functions",
		// DisableFlagParsing: true,
		// Args:  cobra.ExactArgs(1),
	}

	dbCmd.PersistentFlags().AddGoFlagSet(cli.Config.Flags())

	dbCmd.AddCommand(
		&cobra.Command{
			Use:   "scorer-status",
			Short: "scorer status debug",
			RunE:  cli.Run(cli.dbScorerStatus),
		})

	return dbCmd
}

func (cli *CLI) dbScorerStatus(cmd *cobra.Command, args []string) error {

	ctx := context.Background()

	dbconn, err := ntpdb.OpenDB(cli.Config.Database)
	if err != nil {
		return err
	}
	db := ntpdb.New(dbconn)

	ss, err := db.GetScorerStatus(ctx)
	if err != nil {
		return err
	}

	for _, s := range ss {
		// todo: get scorer name, too
		fmt.Printf("%-5d %-20s %-10d %s\n", s.ScorerID, s.Name, s.LogScoreID.Int64, s.ModifiedOn)
	}

	return nil
}
