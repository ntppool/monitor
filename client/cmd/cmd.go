package cmd

import (
	"log"
	"os"

	"github.com/spf13/cobra"
	"go.ntppool.org/monitor/version"
	"golang.org/x/exp/slog"
)

func (cli *CLI) RootCmd() *cobra.Command {

	var programLevel = new(slog.LevelVar) // Info by default

	// temp -- should be an option, and maybe have a runtime signal to adjust?
	// programLevel.Set(slog.LevelDebug)

	logOptions := slog.HandlerOptions{Level: programLevel}

	if len(os.Getenv("INVOCATION_ID")) > 0 {
		// don't add timestamps when running under systemd
		log.Default().SetFlags(0)

		logReplace := func(groups []string, a slog.Attr) slog.Attr {
			// Remove time
			if a.Key == slog.TimeKey && len(groups) == 0 {
				a.Key = ""
			}
			return a
		}

		logOptions.ReplaceAttr = logReplace

	}

	logHandler := logOptions.NewTextHandler(os.Stderr)
	slog.SetDefault(slog.New(logHandler))

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
