package cmd

import (
	"context"
	"net/netip"
	"time"

	"github.com/spf13/cobra"

	"go.ntppool.org/common/logger"
	"go.ntppool.org/common/tracing"
	"go.ntppool.org/monitor/client/config/checkconfig"
	"go.ntppool.org/monitor/client/monitor"
)

func (cli *CLI) checkCmd() *cobra.Command {
	checkCmd := &cobra.Command{
		Use:   "check",
		Short: "do a single check",
		Long:  ``,
		RunE:  cli.Run(cli.checkRun),
		Args:  cobra.MatchAll(cobra.MinimumNArgs(1)),
	}
	checkCmd.PersistentFlags().AddGoFlagSet(cli.Flags())

	return checkCmd
}

func (cli *CLI) checkRun(cmd *cobra.Command, args []string) error {
	log := logger.Setup()

	timeout := time.Second * 60

	ctx, cancel := context.WithTimeout(cmd.Context(), timeout)
	defer cancel()

	ctx, span := tracing.Start(ctx, "api-test")
	defer span.End()

	cfg := &checkconfig.Config{}

	for _, h := range args {

		ip, err := netip.ParseAddr(h)
		if err != nil {
			log.ErrorContext(ctx, "could not parse IP", "ip", h, "err", err)
			continue
		}

		status, resp, err := monitor.CheckHost(ctx, &ip, cfg)
		if err != nil {
			log.InfoContext(ctx, "returned error", "err", err)
		}
		log.InfoContext(ctx, "status", "status", status)
		log.InfoContext(ctx, "response", "resp", resp)

	}

	cancel()

	log.InfoContext(ctx, "check done")

	return nil
}
