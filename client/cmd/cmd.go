package cmd

import (
	"bufio"
	"context"
	"fmt"
	"log"
	"os"
	"path/filepath"
	"strings"

	"github.com/alecthomas/kong"
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

	Debug    bool   `name:"debug" help:"Enable debug logging"`
	StateDir string `name:"state-dir" env:"MONITOR_STATE_DIR" help:"Directory for storing state"`

	DeployEnv depenv.DeploymentEnvironment `name:"env" short:"e" aliases:"deploy-env" required:"" default:"test" env:"DEPLOYMENT_MODE" help:"Deployment environment (prod, test, devel)"`

	API     apiCmd     `cmd:"" help:"check API connection"`
	Monitor monitorCmd `cmd:"" help:"run monitor"`
	Check   checkCmd   `cmd:"" help:"run a single check"`
	Setup   setupCmd   `cmd:"" help:"initial authentication and configuration"`

	IPv4 bool `name:"ipv4" help:"IPv4 monitor (default)" default:"true" negatable:""`
	IPv6 bool `name:"ipv6" help:"IPv6 monitor (default)" default:"true" negatable:""`

	Version version.KongVersionCmd `cmd:"" help:"show version"`
}

func (c *ClientCmd) BeforeReset() error {
	c.Version = version.KongVersionCmd{
		Name: "ntppool-agent",
	}
	defaultsFile := "/etc/default/ntppool-agent"
	if _, err := os.Stat(defaultsFile); err == nil {
		logger.Setup().Debug("Loading defaults", "file", defaultsFile)
		file, err := os.Open(defaultsFile)
		if err != nil {
			return fmt.Errorf("could not open defaults file: %s", err)
		}
		defer file.Close()

		scanner := bufio.NewScanner(file)
		for scanner.Scan() {
			line := scanner.Text()
			parts := strings.SplitN(line, "=", 2)
			if len(parts) != 2 {
				continue // Skip invalid lines
			}
			key, value := strings.TrimSpace(parts[0]), strings.TrimSpace(parts[1])
			if key == "" {
				continue
			}

			// Only set if environment variable is not already set
			if _, exists := os.LookupEnv(key); !exists {
				log.Printf("Setting environment variable %s=%s", key, value)
				os.Setenv(key, value)
			}
		}

		if err := scanner.Err(); err != nil {
			return fmt.Errorf("error reading defaults file: %s", err)
		}
	}
	return nil
}

func (c *ClientCmd) BeforeApply() error {
	if c.StateDir == "" {
		configDir, err := os.UserConfigDir()
		if err != nil {
			return fmt.Errorf("could not find config dir: %s", err)
		}
		if len(configDir) > 0 {
			c.StateDir = filepath.Join(configDir, "ntppool-agent")
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

	if c.StateDir == "" {
		return fmt.Errorf("state directory not set")
	} else {
		c.StateDir = kong.ExpandPath(c.StateDir)
	}

	var err error
	c.Config, err = config.NewAppConfig(ctx, c.DeployEnv, c.StateDir, false)
	if err != nil {
		return err
	}

	return nil
}
