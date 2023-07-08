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

	log := logger.Setup()
	log.Info("monitor-api", "version", version.Version())

	cmd := &cobra.Command{
		Use:   "monitor-api",
		Short: "API server for the NTP Pool monitor",
		// DisableFlagParsing: true,
	}
	// cmd.PersistentFlags().AddGoFlagSet(cli.Config.Flags())

	cmd.AddCommand(cli.serverCmd())
	cmd.AddCommand(version.VersionCmd("monitor-api"))
	cmd.AddCommand(cli.dbCmd())

	return cmd
}
