package cmd

import (
	"context"
	"fmt"

	"go.ntppool.org/common/logger"
	"go.ntppool.org/common/version"
)

func init() {
	logger.ConfigPrefix = "MONITOR"
}

type ApiCmd struct {
	Db      dbCmd      `cmd:"" help:"database commands"`
	Version versionCmd `cmd:"" help:"show version and build info"`
	Server  serverCmd  `cmd:"" help:"run the monitoring api server"`

	DeploymentMode string `help:"Deployment mode" env:"DEPLOYMENT_MODE" enum:"devel,test,prod" default:"devel"`
}

type versionCmd struct{}

func (c *versionCmd) Run(_ context.Context) error {
	ver := version.Version()
	fmt.Printf("%s %s\n", "ntpmon", ver)
	return nil
}
