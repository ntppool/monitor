package version

import (
	"fmt"
	"runtime"
	"runtime/debug"
	"strings"

	"github.com/spf13/cobra"
	"go.ntppool.org/monitor/logger"
	"golang.org/x/mod/semver"
)

// VERSION has the current software version (set in the build process)
var VERSION string
var buildTime string
var gitVersion string
var gitModified bool

func init() {
	if len(VERSION) == 0 {
		VERSION = "dev-snapshot"
	} else {
		if !semver.IsValid(VERSION) {
			logger.Setup().Warn("invalid version number", "version", VERSION)
		}
		if bi, ok := debug.ReadBuildInfo(); ok {
			for _, h := range bi.Settings {
				switch h.Key {
				case "vcs.time":
					buildTime = h.Value
				case "vcs.revision":
					// https://blog.carlmjohnson.net/post/2023/golang-git-hash-how-to/
					// todo: use BuildInfo.Main.Version if revision is empty
					gitVersion = h.Value
				case "vcs.modified":
					if h.Value == "true" {
						gitModified = true
					}
				}
			}
		}
	}

	Version()
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

	v := VERSION
	if len(gitVersion) > 0 {
		g := gitVersion
		if len(g) > 7 {
			g = g[0:7]
		}
		v += "/" + g
		if gitModified {
			v += "-M"
		}

	}

	v = fmt.Sprintf("%s (%s)", v, strings.Join(extra, ", "))
	return v
}
