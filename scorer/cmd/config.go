package cmd

import (
	"flag"
	"fmt"
	"os"

	"github.com/cristalhq/aconfig"
	"github.com/cristalhq/aconfig/aconfigdotenv"
	"github.com/cristalhq/aconfig/aconfigyaml"
	"github.com/spf13/cobra"
	"golang.org/x/exp/slog"
)

type CLI struct {
	Config *APIConfig
}

type APIConfig struct {
	Database struct {
		DSN  string `default:"" flag:"dsn" usage:"Database DSN"`
		User string `default:"" flag:"user"`
		Pass string `default:"" flag:"pass"`
	}

	DeploymentMode string `default:"" usage:"prod, test or devel" flag:"deployment-mode"`

	loaded bool
	loader *aconfig.Loader
	args   []string
}

func NewCLI() *CLI {
	cli := &CLI{}
	cli.Config = &APIConfig{}
	cli.Config.setLoader([]string{})
	return cli
}

func (cfg *APIConfig) Flags() *flag.FlagSet {
	return cfg.loader.Flags()
}

func (cfg *APIConfig) Load(args []string) error {
	if cfg.loaded {
		return nil
	}

	cfg.setLoader(args)

	err := cfg.loader.Load()
	if err != nil {
		return err
	}
	cfg.loaded = true
	return nil
}

func (cfg *APIConfig) setLoader(args []string) {

	acfg := aconfig.Config{
		// MergeFiles: true,
		FileFlag: "config",
		Files:    []string{"monitor-scorer.yaml", "/vault/secrets/database.yaml"},
		FileDecoders: map[string]aconfig.FileDecoder{
			".yaml": aconfigyaml.New(),
			".env":  aconfigdotenv.New(),
		},
	}

	if len(args) > 0 {
		cfg.args = args
	}

	if len(cfg.args) > 0 {
		acfg.Args = cfg.args
	}

	cfg.loader = aconfig.LoaderFor(cfg, acfg)

}

func (cli *CLI) Run(fn func(cmd *cobra.Command, args []string) error) func(*cobra.Command, []string) error {
	return func(cmd *cobra.Command, args []string) error {
		err := cli.Config.Load(args)
		if err != nil {
			fmt.Printf("Could not load config: %s", err)
			return err
		}

		var programLevel = new(slog.LevelVar) // Info by default

		// temp -- should be an option, and maybe have a runtime signal to adjust?
		// programLevel.Set(slog.LevelDebug)

		logOptions := slog.HandlerOptions{Level: programLevel}
		logHandler := logOptions.NewTextHandler(os.Stdout)
		slog.SetDefault(slog.New(logHandler))

		err = fn(cmd, args)
		if err != nil {
			fmt.Printf("error: %s", err)
			return err
		}
		return nil
	}
}
