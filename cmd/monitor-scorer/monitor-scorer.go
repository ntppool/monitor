package main

import (
	basecmd "go.ntppool.org/monitor/cmd"
	"go.ntppool.org/monitor/scorer/cmd"
)

func main() {
	basecmd.Run(&cmd.RootCmd{}, "monitor-api", "Monitor API Server")
}
