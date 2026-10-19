# monitor-api

`monitor-api` is the central API for the monitoring agents. Agents
connect with Connect RPC (`monitor.v2.MonitorService`) to get their
configuration and server lists and to submit test results. It also
tracks agent status over MQTT.

## Prometheus Metrics

`monitor-api` exports metrics on its metrics port (`--metrics-port`,
default 9000), along with the process (`process_*`) and Go runtime
(`go_*`) metrics.

### RPC Calls

#### `rpc_server_call_duration_seconds`
**Type**: Histogram
**Labels**: `rpc_method`, `rpc_response_status_code`, `rpc_system_name`, `error_type`

Duration of each Connect RPC call. Recorded by the `otelconnect`
interceptor and exported through the Prometheus registry
(`server/metrics/otel.go`). The `_count` series is the request count.

- `rpc_method`: the procedure, e.g. `monitor.v2.MonitorService/GetServers`
- `rpc_response_status_code`: `OK`, or the Connect error code in upper
  case, e.g. `INVALID_ARGUMENT`, `UNAUTHENTICATED`, `INTERNAL`
- `rpc_system_name`: `connectrpc`, or `grpc` for gRPC and gRPC-Web clients
- `error_type`: the same code as `rpc_response_status_code`; only on errors

**Buckets**: 5ms to 10s

### NTP Tests

#### `tests_requested_total`
**Type**: Counter
**Labels**: `monitor`, `ip_version`, `account`, `account_id`

Servers handed to monitors to test, counted in `GetServers`.

#### `tests_completed_total`
**Type**: Counter
**Labels**: `monitor`, `ip_version`, `result`, `version`, `account`, `account_id`

Test results received in `SubmitResults`.

- `monitor`: the monitor's TLS name
- `ip_version`: `v4` or `v6`
- `result`: `ok`, `offset`, `timeout`, `signature_validation`, `batch_out_of_order`
- `version`: the `SubmitResults` data version
- `account`: the account's ID token; `account_id`: the numeric account ID
  (`0` without an account)

### MQTT

#### `monitors_connected`
**Type**: Gauge
**Labels**: `client`, `version`, `ip_version`, `account`, `account_id`, `depenv`

Monitors online over MQTT, one series per monitor and IP version. Rebuilt
when a monitor's MQTT status changes. `account` is always empty here.

### Database Pool

The `pgxpool_*` metrics come from `pgdb.NewPoolCollector` in
`go.ntppool.org/common`:

| Metric | Type | Labels |
|---|---|---|
| `pgxpool_connections` | Gauge | `state` (`acquired`, `constructing`, `idle`) |
| `pgxpool_max_connections` | Gauge | |
| `pgxpool_acquires_total` | Counter | |
| `pgxpool_acquire_seconds_total` | Counter | |
| `pgxpool_empty_acquires_total` | Counter | |
| `pgxpool_empty_acquire_wait_seconds_total` | Counter | |
| `pgxpool_canceled_acquires_total` | Counter | |
| `pgxpool_new_connections_total` | Counter | |
| `pgxpool_destroyed_connections_total` | Counter | `reason` (`max_idle`, `max_lifetime`) |

### Build

#### `monitorapi_build_info`
**Type**: Gauge
**Labels**: `version`, `git`, `gittime`, `buildtime`

Always 1.

## Example Queries

```promql
# Requests per second by method and status
sum by (rpc_method, rpc_response_status_code) (rate(rpc_server_call_duration_seconds_count[5m]))

# Error ratio
sum(rate(rpc_server_call_duration_seconds_count{rpc_response_status_code!="OK"}[5m]))
  / sum(rate(rpc_server_call_duration_seconds_count[5m]))

# p95 latency by method
histogram_quantile(0.95, sum by (le, rpc_method) (rate(rpc_server_call_duration_seconds_bucket[5m])))
```
