# NTP Pool Monitor

## Client installation

Configure the appropriate repository (test or production)
from [builds.ntppool.dev/repo/](https://builds.ntppool.dev/repo/).

Install the `ntppool-agent` package, and start the systemd units for
test and/or production (example below starts both).

```
sudo apt update
sudo apt install -y ntppool-agent

for f in test prod; do
  sudo systemctl enable --now ntppool-agent@$f;
done

sudo journalctl -u ntppool-agent@\* -f
```

When installed from the rpm or deb package, state will by default
be stored in `/var/lib/ntppool-agent`. You can change the default in
`/etc/default/ntppool-agent`, or with the `--state-dir` parameter
or by setting `$MONITOR_STATE_DIR` in the environment.

The `--env` parameter specifies which server to use (prod, test or devel).

## Configuration and Hot Reloading

The ntppool-agent supports automatic configuration reloading without restart:

- **Automatic response** to `setup` command changes via file system monitoring
- **Automatic certificate renewal** when certificates approach expiration
- **Dynamic protocol activation** when IPv4/IPv6 status changes

When you run the setup command, configuration changes are applied automatically to all running agent processes. No manual restart is required.

## Monitor Selection System

The NTP Pool Monitor uses an advanced candidate status system for selecting which monitors test each server:

### Monitor States (Per-Server Assignment)
- **candidate** - Monitor selected for potential assignment to server (default for new assignments)
- **testing** - Monitor actively monitoring and being evaluated for server
- **active** - Monitor confirmed for long-term monitoring of server

### Status Flow
Regular monitors progress through states based on performance and constraints:
```
candidate → testing → active
```

New monitors are initially assigned `candidate` status and promoted by the selector based on:
- **Health metrics** - RTT, accuracy, and reliability measurements
- **Constraint compliance** - Network diversity, account limits, and geographic distribution
- **Server needs** - Available slots and current monitor coverage

### Key Features
- **Constraint validation** - Prevents monitors from same network/account
- **Gradual state transitions** - Safe promotion/demotion to maintain service stability
- **Emergency override** - System can recover from zero monitors by bypassing constraints
- **Dynamic capacity management** - Testing pool size adjusts based on active monitor availability

## Documentation

### Development Planning
See **[plans/](plans/)** directory for comprehensive implementation planning:
- **Design documents** - Timeless architecture descriptions
- **Implementation plans** - Current and planned development work
- **Archive** - Historical context from completed implementations

### Project Documentation
- **[LLM_CODING_AGENT.md](LLM_CODING_AGENT.md)** - Comprehensive developer guidelines and architectural patterns
- **[plans/README.md](plans/README.md)** - Planning documentation overview and status summary

### Monitor Types
The system supports two distinct monitor types:
- **Regular monitors** (`type = 'monitor'`) - Client agents that test NTP servers
- **Scorer monitors** (`type = 'score'`) - Backend processes that calculate aggregate performance metrics

The system ensures diverse, reliable monitoring coverage while preventing operational disruptions from mass changes.

## Client requirements

A well connected Linux or FreeBSD system (x86_64 or arm64) with good IPv4 and/or IPv6 internet connectivity.

Each instance (for the beta system and/or the production system)
takes less than 30MB memory currently and approximately no CPU
(very little anyway). However it's best if the system isn't
excessively loaded, as that can impact the NTP measurements.
The network needs to be consistently low latency "to the internet".

A future version will either require or work best with traceroute
installed and available as well.

## Server / API configuration

### Environment variables

- `DEPLOYMENT_MODE` devel
- `DATABASE_DSN` database connection string
  - example: `"tcp(ntp-db-mysql)/betadb?charset=utf8&parseTime=true"`
- `JWT_KEY` key for signing JWTs for the mosquitto server
- `VAULT_CACERT` path for public vault signing certificate
- `VAULT_ADDR` URL for vault server
- `OTEL_EXPORTER_OTLP_ENDPOINT` URL for open telemetry endpoint

### Files

If you don't add username and password to the `DATABASE_DSN`, it has to
be provided in a file named `database.yaml` or
`/vault/secrets/database.yaml` in the format:

```
mysql:
  user: some-db-user
  pass: ...
```

You can also provide a `dsn:` field in that datastructure
and omit the DATABASE_DSN altogether.
