package cmd

import (
	"context"

	"github.com/spf13/cobra"
	"go.ntppool.org/monitor/ntpdb"
	"go.ntppool.org/monitor/scorer"
)

func (cli *CLI) scorerCmd() *cobra.Command {

	var scorerCmd = &cobra.Command{
		Use:   "scorer",
		Short: "Run scorer",
		RunE:  cli.Run(cli.scorer),
	}

	scorerCmd.PersistentFlags().AddGoFlagSet(cli.Config.Flags())

	// scorerCmd.AddCommand(
	// 	&cobra.Command{
	// 		Use:   "run",
	// 		Short: "scorer status debug",
	// 		RunE:  cli.Run(cli.dbScorerStatus),
	// 	})

	return scorerCmd
}

func (cli *CLI) scorer(cmd *cobra.Command, args []string) error {

	ctx := context.Background()

	dbconn, err := ntpdb.OpenDB(cli.Config.Database)
	if err != nil {
		return err
	}

	sc, err := scorer.New(ctx, dbconn)
	if err != nil {
		return nil
	}
	return sc.Run()
}
