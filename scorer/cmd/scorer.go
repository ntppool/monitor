package cmd

import (
	"context"
	"log"
	"time"

	"github.com/spf13/cobra"
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

	return scorerCmd
}

func (cli *CLI) scorerServer(cmd *cobra.Command, args []string) error {

	for {
		count, err := cli.scorerRun(cmd, args)
		if err != nil {
			return err
		}

		if count == 0 {
			time.Sleep(20 * time.Second)
		}

	}

}

func (cli *CLI) scorer(cmd *cobra.Command, args []string) error {
	_, err := cli.scorerRun(cmd, args)
	return err
}

func (cli *CLI) scorerRun(cmd *cobra.Command, args []string) (int, error) {

	ctx := context.Background()

	dbconn, err := ntpdb.OpenDB(cli.Config.Database)
	if err != nil {
		return 0, err
	}

	sc, err := scorer.New(ctx, dbconn)
	if err != nil {
		return 0, nil
	}
	count, err := sc.Run()
	if err != nil {
		return count, err
	}
	log.Printf("Processed %d log scores", count)
	return count, nil
}
