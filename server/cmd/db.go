package cmd

import (
	"context"
	"database/sql"
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
			Use:   "mon",
			Short: "monitor config debug",
			RunE:  cli.Run(cli.dbMonitorConfig),
		})

	return dbCmd
}

func (cli *CLI) dbMonitorConfig(cmd *cobra.Command, args []string) error {

	if len(args) < 1 {
		return fmt.Errorf("db mon [monitername]")
	}

	name := args[0]

	ctx := context.Background()

	dbconn, err := ntpdb.OpenDB(cli.Config.Database)
	if err != nil {
		return err
	}
	db := ntpdb.New(dbconn)

	mon, err := db.GetMonitorTLSName(ctx, sql.NullString{String: name, Valid: true})
	if err != nil {
		return err
	}

	smon, err := ntpdb.GetSystemMonitor(ctx, db, "settings", mon.IpVersion)
	if err == nil {
		mconf, err := mon.GetConfigWithDefaults([]byte(smon.Config))
		if err != nil {
			return err
		}
		fmt.Printf("mconf: %+v", mconf)
	}

	return nil
}
