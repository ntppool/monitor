package cmd

import (
	"bufio"
	"context"
	"fmt"
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
				log := logger.Setup()
				log.Debug("Setting environment variable", "name", key, "value", value)
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
	// Create a logger for debugging
	log := logger.Setup()
	ctx := logger.NewContext(context.Background(), log)

	if c.StateDir == "" {
		// Priority: MONITOR_STATE_DIR > STATE_DIRECTORY > user config dir
		if monitorStateDir := os.Getenv("MONITOR_STATE_DIR"); monitorStateDir != "" {
			c.StateDir = monitorStateDir
			log.DebugContext(ctx, "using state directory from MONITOR_STATE_DIR", "path", c.StateDir)
		} else if stateDir := os.Getenv("STATE_DIRECTORY"); stateDir != "" {
			c.StateDir = stateDir
			log.DebugContext(ctx, "using state directory from STATE_DIRECTORY", "path", c.StateDir)
		} else {
			// Fall back to user config directory
			configDir, err := os.UserConfigDir()
			if err != nil {
				return fmt.Errorf("could not find config dir: %s", err)
			}
			if len(configDir) > 0 {
				c.StateDir = filepath.Join(configDir, "ntppool-agent")
				log.DebugContext(ctx, "using default state directory", "path", c.StateDir)
			}
		}
	} else {
		log.DebugContext(ctx, "using explicit state directory", "path", c.StateDir)
	}
	return nil
}

func (c *ClientCmd) AfterApply(kctx *kong.Context, ctx context.Context) error {
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

	log := logger.FromContext(ctx)
	log.DebugContext(ctx, "determined state directory",
		"state_dir", c.StateDir,
		"env", c.DeployEnv.String(),
		"command", kctx.Command())

	// Check for problematic runtime directory usage
	if strings.HasPrefix(c.StateDir, "/var/run/ntppool-agent") || strings.HasPrefix(c.StateDir, "/run/ntppool-agent") {
		runtimeDir := os.Getenv("RUNTIME_DIRECTORY")
		if runtimeDir != "" && (c.StateDir == runtimeDir || strings.HasPrefix(c.StateDir, runtimeDir+"/")) {
			log.WarnContext(ctx, "State directory is in runtime directory - data will be lost on reboot!",
				"state_dir", c.StateDir,
				"runtime_dir", runtimeDir,
				"suggestion", "Use systemd StateDirectory or set MONITOR_STATE_DIR to a persistent location")
		}
	}

	// Skip configuration loading for version command
	if kctx.Command() == "version" {
		return nil
	}

	var err error
	c.Config, err = config.NewAppConfig(ctx, c.DeployEnv, c.StateDir, false)
	if err != nil {
		return err
	}

	return nil
}
