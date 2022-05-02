package version

import (
	"fmt"
	"runtime"
	"strings"

	"github.com/spf13/cobra"
)

// VERSION has the current software version (set in the build process)
var VERSION string
var buildTime string
var gitVersion string

func init() {
	if len(gitVersion) > 0 {
		VERSION = VERSION + "/" + gitVersion
	}
	if len(VERSION) == 0 {
		VERSION = "dev-snapshot"
	}
}

func VersionCmd() *cobra.Command {
	versionCmd := &cobra.Command{
		Use:   "version",
		Short: "Print version and build information",
		Run: func(cmd *cobra.Command, args []string) {
			ver := Version()
			fmt.Printf("ntppool-monitor %s\n", ver)
		},
	}
	return versionCmd
}

var v string

func Version() string {
	if len(v) > 0 {
		return v
	}
	extra := []string{}
	if len(buildTime) > 0 {
		extra = append(extra, buildTime)
	}
	extra = append(extra, runtime.Version())
	v = fmt.Sprintf("%s (%s)", VERSION, strings.Join(extra, ", "))
	return v
}

func init() {
	Version()
}
