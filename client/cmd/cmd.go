package cmd

import (
	"log"
	"os"

	"github.com/spf13/cobra"
	"go.ntppool.org/monitor/version"
)

func (cli *CLI) RootCmd() *cobra.Command {

	if len(os.Getenv("INVOCATION_ID")) > 0 {
		// don't add timestamps when running under systemd
		log.Default().SetFlags(0)
	}

	cmd := &cobra.Command{
		Use:   "ntppool-monitor",
		Short: "Monitoring daemon for the NTP Pool system",
	}

	cmd.AddCommand(cli.monitorCmd())
	cmd.AddCommand(cli.apiCmd())
	cmd.AddCommand(version.VersionCmd())

	cmd.PersistentFlags().AddGoFlagSet(cli.Config.Flags())

	return cmd
}
