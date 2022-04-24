package cmd

import (
	"github.com/spf13/cobra"
	"go.ntppool.org/monitor/version"
)

func (cli *CLI) RootCmd() *cobra.Command {

	cmd := &cobra.Command{
		Use:   "monitor-api",
		Short: "API server for the NTP Pool monitor",
		// DisableFlagParsing: true,
	}
	// cmd.PersistentFlags().AddGoFlagSet(cli.Config.Flags())

	cmd.AddCommand(cli.serverCmd())
	cmd.AddCommand(version.VersionCmd())

	return cmd
}