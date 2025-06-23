package cmd

import (
	"context"
	"fmt"

	"go.ntppool.org/common/version"
	"go.ntppool.org/monitor/selector"
)

type RootCmd struct {
	Scorer   ScorerCmd    `cmd:"scorer" help:"Scoring commands"`
	Selector selector.Cmd `cmd:"selector" help:"monitor selection"`

	Db      dbCmd      `cmd:"db" help:"Database operations"`
	Version versionCmd `cmd:"version" help:"Show version"`
}

type ScorerCmd struct {
	Run    scorerOnceCmd   `cmd:"run" help:"Run once"`
	Server scorerServerCmd `cmd:"server" help:"Run continuously"`
	Setup  scorerSetupCmd  `cmd:"setup" help:"Setup scorers"`
}

type (
	scorerOnceCmd   struct{}
	scorerServerCmd struct{}
	scorerSetupCmd  struct{}
)

type versionCmd struct{}

func (c *versionCmd) Run(_ context.Context) error {
	ver := version.Version()
	fmt.Printf("%s %s\n", "monitor-scorer", ver)
	return nil
}
