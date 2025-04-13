# NTP Pool monitor

After getting invitated to participate in the test program, obtain API
keys on the beta and production site as the new system is rolled out
there.

 - beta site: https://manage-beta.grundclock.com/manage/monitors
 - production site: https://manage.ntppool.org/manage/monitors

## Installation

The monitor is supported on Linux and FreeBSD. Yum and deb package
repositories are available for i386, arm64 and amd64. For repository
instructions see https://builds.ntppool.dev/repo/

## Setup

Run `ntpmon --env test setup` (or `prod` for production environment)
to setup an API key, and then `ntpmon --env test monitor` to
run the monitor. See the README file at the root of the git
repository for further instructions.

If you installed from the .deb or .rpm package, you can start
the test monitor with

   sudo systemctl systemctl enable --now ntpmon@test
