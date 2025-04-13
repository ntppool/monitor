package main

import (
	"go.ntppool.org/monitor/client/cmd"
	basecmd "go.ntppool.org/monitor/cmd"
)

func main() {
	basecmd.Run(&cmd.ClientCmd{}, "ntpmon", "Monitoring daemon for the NTP Pool system")
}
