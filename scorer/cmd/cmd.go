package cmd

import (
	"github.com/spf13/cobra"
	"go.ntppool.org/monitor/version"
)

func (cli *CLI) RootCmd() *cobra.Command {

	cmd := &cobra.Command{
		Use:   "monitor-scorer",
		Short: "Run scoring on monitoring data",
		// DisableFlagParsing: true,
	}
	// cmd.PersistentFlags().AddGoFlagSet(cli.Config.Flags())

	cmd.AddCommand(version.VersionCmd())
	cmd.AddCommand(cli.scorerCmd())
	cmd.AddCommand(cli.dbCmd())

	return cmd
}
