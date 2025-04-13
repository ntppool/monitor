package cmd

import (
	"context"
	"fmt"
	"os"
	"path/filepath"

	"go.ntppool.org/common/config/depenv"
	"go.ntppool.org/common/logger"
	"go.ntppool.org/common/version"
	"go.ntppool.org/monitor/client/config"
)

func init() {
	logger.ConfigPrefix = "MONITOR"
}

type ClientCmd struct {
	Config config.AppConfig `kong:"-"`

	Debug    bool   `kong:"debug" help:"Enable debug logging"`
	StateDir string `kong:"state-dir" help:"Directory for storing state"`

	DeployEnv depenv.DeploymentEnvironment `kong:"env,default=test" flag:"env" help:"Deployment environment"`

	API     apiCmd     `cmd:"" help:"check API connection"`
	Monitor monitorCmd `cmd:"" help:"run monitor"`
	Check   checkCmd   `cmd:"" help:"run a single check"`
	Setup   setupCmd   `cmd:"" help:"initial authentication and configuration"`

	Version version.KongVersionCmd `cmd:"" help:"show version"`
}

func (c *ClientCmd) BeforeApply() error {
	if c.StateDir == "" {
		configDir, err := os.UserConfigDir()
		if err != nil {
			return fmt.Errorf("could not find config dir: %s", err)
		}
		if len(configDir) > 0 {
			c.StateDir = filepath.Join(configDir, "ntpmon")
		}
	}
	return nil
}

func (c *ClientCmd) AfterApply(ctx context.Context) error {
	if c.Debug {
		os.Setenv("MONITOR_DEBUG", "true")
	}
	if c.DeployEnv == depenv.DeployUndefined {
		return fmt.Errorf("deployment environment invalid or undefined")
	}

	var err error
	c.Config, err = config.NewAppConfig(ctx, c.DeployEnv, c.StateDir, false)
	if err != nil {
		return err
	}

	return nil
}
