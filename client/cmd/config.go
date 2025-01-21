package cmd

import (
	"flag"
	"fmt"
	"os"
	"path/filepath"

	"github.com/spf13/cobra"

	"go.ntppool.org/common/logger"
	"go.ntppool.org/common/version"
	"go.ntppool.org/monitor/api"
)

type CLI struct {
	Config api.AppConfig

	flags    *flag.FlagSet
	EnvName  string
	StateDir string
	Debug    bool
}

var envPrefix = "MONITOR"

func NewCLI() *CLI {
	log := logger.Setup()

	flags := flag.NewFlagSet("api", flag.ContinueOnError)

	cli := &CLI{
		flags: flags,
	}

	configDir, err := os.UserConfigDir()
	if err != nil {
		log.Error("could not find config dir", "err", err)
	}
	if len(configDir) > 0 {
		cli.StateDir = filepath.Join(configDir, "ntpmon")
	}

	flags.StringVar(&cli.EnvName, "env", "beta", "Environment name")
	flags.StringVar(&cli.StateDir, "state-dir", cli.StateDir, "Directory for storing state")
	flags.BoolVar(&cli.Debug, "debug", cli.Debug, "Enable debug logging")

	return cli
}

func (cli *CLI) Flags() *flag.FlagSet {
	return cli.flags
}

func (cli *CLI) LoadConfig(args []string) error {
	// cfg := cli.Config

	err := cli.Flags().Parse(args)
	if err != nil {
		return err
	}

	if len(cli.StateDir) == 0 {
		return fmt.Errorf("state-dir configuration required")
	}
	if cli.Debug {
		os.Setenv("MONITOR_DEBUG", "true")
	}

	cli.Config, err = api.NewAppConfig(cli.EnvName, cli.StateDir)
	if err != nil {
		return err
	}

	return nil
}

func (cli *CLI) Run(fn func(cmd *cobra.Command, args []string) error) func(*cobra.Command, []string) error {
	return func(cmd *cobra.Command, args []string) error {
		logger.Setup().Info("ntppool-monitor", "version", version.Version())

		err := cli.LoadConfig(args)
		if err != nil {
			fmt.Printf("Could not load config: %s", err)
			return err
		}

		err = fn(cmd, args)
		if err != nil {
			fmt.Printf("error: %s", err)
			return err
		}

		return nil
	}
}
