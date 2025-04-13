package main

import (
	"os"

	"go.ntppool.org/monitor/client/cmd"
)

func main() {
	cli := cmd.NewCLI()
	if err := cli.RootCmd().Execute(); err != nil {
		os.Exit(1)
	}
}
