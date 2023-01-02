package cmd

import (
	"flag"
	"fmt"

	"github.com/cristalhq/aconfig"
	"github.com/cristalhq/aconfig/aconfigdotenv"
	"github.com/cristalhq/aconfig/aconfigyaml"
	"github.com/spf13/cobra"
	"go.ntppool.org/monitor/ntpdb"
)

type CLI struct {
	Config *APIConfig
}

type APIConfig struct {
	Database ntpdb.DBConfig

	Listen string `default:":8000" usage:"Listen address" flag:"listen"`

	TLS struct {
		Key  string `default:"/etc/tls/tls.key"`
		Cert string `default:"/etc/tls/tls.crt"`
	}

	DeploymentMode string `default:"" usage:"prod, test or devel" flag:"deployment-mode"`

	JWTKey string `default:"" usage:"JWT signing key" flag:"jwtkey"`

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
		Files:    []string{"monitor-api.yaml", "/vault/secrets/database.yaml"},
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
		err = fn(cmd, args)
		if err != nil {
			fmt.Printf("error: %s", err)
			return err
		}
		return nil
	}
}
