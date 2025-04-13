package main

import (
	basecmd "go.ntppool.org/monitor/cmd"
	"go.ntppool.org/monitor/server/cmd"
)

func main() {
	basecmd.Run(&cmd.ApiCmd{}, "monitor-api", "Monitor API Server")
}
