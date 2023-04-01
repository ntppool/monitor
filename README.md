# NTP Pool Monitor

## Client installation

Configure the appropriate repository (test or production)
from [builds.ntppool.dev/repo/](https://builds.ntppool.dev/repo/).

Install the `ntppool-monitor` package, copy the configuration files to
`/etc/ntpmon` and then start the systemd units (or file and unit, if you
are only running either a v4 or v6 monitor instead of both).

```
sudo apt update
sudo apt install -y ntppool-monitor

cd /etc/ntpmon;
for f in *ntppool.dev.json; do
  n=`basename $f .json`;
  sudo systemctl enable --now ntppool-monitor@$n;
done

sudo journalctl -u ntppool-monitor@\* -f
```

## Client requirements

A well connected Linux or FreeBSD system (x86_64 or arm64).

Each instance (you need one for each of IPv4 and IPv6, and maybe for
the beta system and the production system) takes less than 20MB memory
currently and approximately no CPU.

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
be provided in a file named `/vault/secrets/database.yaml` in the format:

```
database:
  user: some-db-user
  pass: ...
```
