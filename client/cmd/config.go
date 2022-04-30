package cmd

import (
	"flag"
	"fmt"

	"github.com/cristalhq/aconfig"
	"github.com/cristalhq/aconfig/aconfigdotenv"
	"github.com/cristalhq/aconfig/aconfigyaml"
	"github.com/spf13/cobra"
)

type CLI struct {
	Config *APIConfig
}

var (
	envPrefix = "MONITOR"
)

type APIConfig struct {
	Name string
	API  struct {
		Key    string `default:""`
		Secret string `default:"" `
	}

	StateDir string `default:"." flag:"state-dir" usage:"Directory for storing state"`

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

	if len(cfg.StateDir) == 0 {
		return fmt.Errorf("state-dir configuration required")
	}
	if len(cfg.Name) == 0 {
		return fmt.Errorf("name configuration required")
	}

	cfg.loaded = true
	return nil
}

func (cfg *APIConfig) setLoader(args []string) {

	acfg := aconfig.Config{
		MergeFiles: true,
		EnvPrefix:  envPrefix,
		FileFlag:   "config",
		// Files:      []string{"monitor-api.yaml", "/vault/secrets/database.yaml"},
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

func (cli *CLI) Run(fn func(cmd *cobra.Command) error) func(*cobra.Command, []string) error {
	return func(cmd *cobra.Command, args []string) error {
		err := cli.Config.Load(args)
		if err != nil {
			fmt.Printf("Could not load config: %s", err)
			return err
		}
		err = fn(cmd)
		if err != nil {
			fmt.Printf("error: %s", err)
			return err
		}
		return nil
	}
}
