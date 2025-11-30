package main

import (
	"github.com/alecthomas/kong"

	basecmd "go.ntppool.org/monitor/cmd"
	"go.ntppool.org/monitor/scorer/cmd"
	"go.ntppool.org/monitor/selector"
)

func main() {
	root := &cmd.RootCmd{}
	basecmd.Run(root, "monitor-scorer", "Monitor Scorer and Selector",
		kong.BindTo(root, (*selector.ParentConfig)(nil)),
	)
}
