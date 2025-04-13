package cmd

import (
	"context"
	"net/netip"
	"time"

	"go.ntppool.org/common/logger"
	"go.ntppool.org/common/tracing"
	"go.ntppool.org/monitor/client/config/checkconfig"
	"go.ntppool.org/monitor/client/monitor"
)

type checkCmd struct {
	IP []string `arg:"" help:"IP addresses to check"`
}

func (cmd *checkCmd) Run(ctx context.Context) error {
	log := logger.Setup()

	timeout := time.Second * 60

	ctx, cancel := context.WithTimeout(ctx, timeout)
	defer cancel()

	ctx, span := tracing.Start(ctx, "api-test")
	defer span.End()

	cfg := &checkconfig.Config{}

	for _, h := range cmd.IP {

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
