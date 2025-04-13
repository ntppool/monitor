package cmd

import (
	"github.com/spf13/cobra"
	"go.ntppool.org/common/logger"
	"go.ntppool.org/common/version"
)

func init() {
	logger.ConfigPrefix = "MONITOR"
}

func (cli *CLI) RootCmd() *cobra.Command {
	logger.Setup()

	cmd := &cobra.Command{
		Use:   "ntpmon",
		Short: "Monitoring daemon for the NTP Pool system",
	}

	cmd.AddCommand(cli.monitorCmd())
	cmd.AddCommand(cli.apiCmd())
	cmd.AddCommand(cli.checkCmd())
	cmd.AddCommand(version.VersionCmd("ntpmon"))
	cmd.AddCommand(cli.setupCmd())
	// cmd.AddCommand(cli.DebugCmd())

	cmd.PersistentFlags().AddGoFlagSet(cli.Flags())

	return cmd
}
