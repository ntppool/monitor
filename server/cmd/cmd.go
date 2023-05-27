package cmd

import (
	"os"

	"github.com/spf13/cobra"
	"go.ntppool.org/monitor/version"
	"golang.org/x/exp/slog"
)

func (cli *CLI) RootCmd() *cobra.Command {

	slog.SetDefault(slog.New(slog.NewTextHandler(os.Stdout, nil)))

	cmd := &cobra.Command{
		Use:   "monitor-api",
		Short: "API server for the NTP Pool monitor",
		// DisableFlagParsing: true,
	}
	// cmd.PersistentFlags().AddGoFlagSet(cli.Config.Flags())

	cmd.AddCommand(cli.serverCmd())
	cmd.AddCommand(version.VersionCmd())

	cmd.AddCommand(cli.dbCmd())

	return cmd
}
