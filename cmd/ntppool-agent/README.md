# NTP Pool monitor

After getting invited to participate in the test program, obtain API
keys on the beta and production site as the new system is rolled out
there.

 - beta site: https://manage-beta.grundclock.com/manage/monitors
 - production site: https://manage.ntppool.org/manage/monitors

## Installation

The monitor is supported on Linux and FreeBSD. Yum and deb package
repositories are available for i386, arm64, amd64 and riscv64. For repository
instructions see https://builds.ntppool.dev/repo/

## Setup

Run `ntppool-agent --env test setup` (or `prod` for production environment)
to setup an API key. You can optionally specify a hostname with the `--hostname`
flag. Then run `ntppool-agent --env test monitor` to run the monitor.
The agent supports hot reloading of configuration changes without restart.
See the README file at the root of the git repository for further instructions.

If you installed from the .deb or .rpm package, you can start
the test monitor with

   sudo systemctl enable --now ntppool-agent@test
