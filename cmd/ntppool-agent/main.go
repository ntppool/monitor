package main

import (
	"go.ntppool.org/monitor/client/cmd"
	basecmd "go.ntppool.org/monitor/cmd"
)

func main() {
	basecmd.Run(&cmd.ClientCmd{}, "ntppool-agent", "Monitoring daemon for the NTP Pool system")
}
