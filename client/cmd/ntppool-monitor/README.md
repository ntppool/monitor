# NTP Pool monitor

After getting invitated to participate in the test program, obtain API
keys on the beta and production site as the new system is rolled out
there.

 - beta site: https://manage-beta.grundclock.com/manage/monitors
 - production site: https://manage.ntppool.org/manage/monitors

Configure the system in `ntppool-monitor.yaml` (json and toml files
are also supported). You can specify another configuration file with
the `--config` parameter.

    name:       "uspao1-abcd24.devel.mon.ntppool.dev"
    api:
      key:    "6051a5d7-xxxx-yyyy-1234-abcdef123456"
      secret: "79f62ccc-xxxx-yyyy-9876-54321abcde12"

## Installation

The monitor is supported on Linux and FreeBSD. Yum and deb package
repositories are available for i386, arm64 and amd64. For repository
instructions see https://builds.ntppool.dev/repo/

## Setup

After the monitor has been provisioned on the management website,
download the .json configuration file and place it in
`/var/run/ntpmon` on the monitoring system.

With the configuration in place, enable the monitor for that
configuration with the name from the configuarion:

   sudo systemctl systemctl enable --now ntppool-monitor@[client-name]
