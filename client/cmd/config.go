package cmd

import (
	"context"
	"flag"
	"fmt"
	"os"
	"path/filepath"

	"github.com/spf13/cobra"

	"go.ntppool.org/common/config/depenv"
	"go.ntppool.org/common/logger"
	"go.ntppool.org/common/version"
	"go.ntppool.org/monitor/client/config"
)

type CLI struct {
	Config config.AppConfig

	flags     *flag.FlagSet
	envName   string
	DeployEnv depenv.DeploymentEnvironment
	StateDir  string
	Debug     bool
}

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

	flags.StringVar(&cli.envName, "env", "test", "Environment name")
	flags.StringVar(&cli.StateDir, "state-dir", cli.StateDir, "Directory for storing state")
	flags.BoolVar(&cli.Debug, "debug", cli.Debug, "Enable debug logging")

	return cli
}

func (cli *CLI) Flags() *flag.FlagSet {
	return cli.flags
}

func (cli *CLI) LoadConfig(ctx context.Context, args []string) error {
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

	cli.DeployEnv = depenv.DeploymentEnvironmentFromString(cli.envName)
	if cli.DeployEnv == depenv.DeployUndefined {
		return fmt.Errorf("deployment environment %q invalid or undefined", cli.envName)
	}

	cli.Config, err = config.NewAppConfig(ctx, cli.DeployEnv, cli.StateDir, false)
	if err != nil {
		return err
	}

	return nil
}

func (cli *CLI) Run(fn func(cmd *cobra.Command, args []string) error) func(*cobra.Command, []string) error {
	return func(cmd *cobra.Command, args []string) error {
		logger.Setup().Info("ntpmon", "version", version.Version())

		err := cli.LoadConfig(cmd.Context(), args)
		if err != nil {
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
