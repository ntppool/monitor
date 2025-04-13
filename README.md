# NTP Pool Monitor

## Client installation

Configure the appropriate repository (test or production)
from [builds.ntppool.dev/repo/](https://builds.ntppool.dev/repo/).

Install the `ntpmon` package, and start the systemd units for
test and/or production (example below starts both).

```
sudo apt update
sudo apt install -y ntpmon

cd /etc/ntpmon;
for f in test prod; do
  sudo systemctl enable --now ntpmon@$n;
done

sudo journalctl -u ntpmon@\* -f
```

## Client requirements

A well connected Linux or FreeBSD system (x86_64 or arm64).

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
