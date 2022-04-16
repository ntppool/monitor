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
    api_key:    "6051a5d7-xxxx-yyyy-1234-abcdef123456"
    secret_key: "79f62ccc-xxxx-yyyy-9876-54321abcde12"
