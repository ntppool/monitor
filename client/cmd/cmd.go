package cmd

import (
	"github.com/spf13/cobra"
	"go.ntppool.org/monitor/logger"
	"go.ntppool.org/monitor/version"
)

func (cli *CLI) RootCmd() *cobra.Command {

	logger.Setup()

	cmd := &cobra.Command{
		Use:   "ntppool-monitor",
		Short: "Monitoring daemon for the NTP Pool system",
	}

	cmd.AddCommand(cli.monitorCmd())
	cmd.AddCommand(cli.apiCmd())
	cmd.AddCommand(version.VersionCmd())
	// cmd.AddCommand(cli.DebugCmd())

	cmd.PersistentFlags().AddGoFlagSet(cli.Config.Flags())

	return cmd
}
