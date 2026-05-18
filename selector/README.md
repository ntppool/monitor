# Selector Package

The selector package implements the monitor selection algorithm for the NTP Pool monitoring system. It manages which monitors are assigned to which servers based on constraints, performance metrics, and availability.

## Prometheus Metrics

The selector exposes comprehensive prometheus metrics to provide operational visibility into monitor selection decisions and system performance.

### Status Change Tracking

#### `selector_status_changes_total`
**Type**: Counter
**Labels**: `monitor_id_token`, `monitor_tls_name`, `from_status`, `to_status`, `server_id`, `reason`

Tracks all monitor status transitions in the selection process.

**Status values**:
- `new` - Monitor not assigned to server
- `candidate` - Monitor selected for potential assignment
- `testing` - Monitor actively monitoring and being evaluated
- `active` - Monitor confirmed for long-term monitoring

**Reason examples**:
- `"new candidate"` - Monitor added to candidate pool
- `"candidate to testing"` - Candidate promoted to testing
- `"promotion to active"` - Testing monitor promoted to active
- `"blocked by constraints or global status"` - Monitor removed due to violations
- `"gradual removal (health or constraints)"` - Monitor gradually phased out

### Constraint Violation Metrics

#### `selector_constraint_violations_total`
**Type**: Counter
**Labels**: `monitor_id_token`, `monitor_tls_name`, `constraint_type`, `server_id`, `is_grandfathered`

Tracks all constraint violations detected during selection.

**Constraint types**:
- `network_same_subnet` - Monitor and server in same /24 (IPv4) or /48 (IPv6) subnet
- `account` - Monitor and server belong to the same account
- `limit` - Account has exceeded per-server monitor limit
- `network_diversity` - Multiple monitors in same /20 (IPv4) or /44 (IPv6) network

#### `selector_grandfathered_violations`
**Type**: Gauge
**Labels**: `monitor_id_token`, `monitor_tls_name`, `constraint_type`, `server_id`

Current count of grandfathered constraint violations. These are existing assignments that violate current constraints but are allowed to continue to prevent service disruption.

### Performance Metrics

#### `selector_process_duration_seconds`
**Type**: Histogram
**Labels**: `server_id`

Time spent processing each server in the selection algorithm.

#### `selector_monitors_evaluated_total`
**Type**: Counter
**Labels**: `server_id`

Total number of monitors evaluated per server during selection.

#### `selector_changes_applied_total`
**Type**: Counter
**Labels**: `server_id`

Number of status changes successfully applied per server.

#### `selector_changes_failed_total`
**Type**: Counter
**Labels**: `server_id`

Number of status changes that failed to apply per server.

### Monitor Pool Health

#### `selector_monitor_pool_size`
**Type**: Gauge
**Labels**: `status`, `server_id`

Current number of monitors in each status per server.

**Status values**: `active`, `testing`, `candidate`, `available`

#### `selector_globally_active_monitors`
**Type**: Gauge
**Labels**: `server_id`

Number of globally active monitors currently in the testing pool for each server. The selector maintains a minimum of 4 globally active monitors in testing.

#### `selector_constraint_blocked_monitors`
**Type**: Gauge
**Labels**: `constraint_type`, `server_id`

Number of monitors blocked by each constraint type per server.

## Monitor Identification

All metrics use dual monitor identification for rich operational insights:

- **`monitor_id_token`**: Unique identifier that distinguishes IPv4/IPv6 monitors
- **`monitor_tls_name`**: Human-readable monitor name (shared between IPv4/IPv6)

This approach enables:
- Tracking individual IPv4/IPv6 monitor behavior
- Aggregating metrics by TLS name across protocol versions
- Stable identification for dashboards and alerts

## Example Queries

### Monitor Status Distribution
```promql
# Active monitors per server
selector_monitor_pool_size{status="active"}

# Testing pool health
selector_globally_active_monitors
```

### Constraint Violations
```promql
# Rate of new constraint violations
rate(selector_constraint_violations_total[5m])

# Grandfathered violations by type
sum by (constraint_type) (selector_grandfathered_violations)
```

### Performance Monitoring
```promql
# Selection processing time percentiles
histogram_quantile(0.95, selector_process_duration_seconds)

# Selection efficiency
rate(selector_changes_applied_total[5m]) / rate(selector_monitors_evaluated_total[5m])
```

### Status Change Analysis
```promql
# Promotion rate (candidate -> testing -> active)
rate(selector_status_changes_total{to_status="active"}[5m])

# Removal reasons
sum by (reason) (rate(selector_status_changes_total{to_status="new"}[5m]))
```

## Alerting Examples

### Critical Alerts
```yaml
# Insufficient globally active monitors
- alert: SelectorInsufficientGloballyActiveMonitors
  expr: selector_globally_active_monitors < 4
  for: 5m
  annotations:
    summary: "Server {{ $labels.server_id }} has insufficient globally active monitors"

# High constraint violation rate
- alert: SelectorHighConstraintViolationRate
  expr: rate(selector_constraint_violations_total[5m]) > 10
  for: 2m
  annotations:
    summary: "High rate of constraint violations in selector"
```

### Warning Alerts
```yaml
# Excessive grandfathering
- alert: SelectorExcessiveGrandfathering
  expr: sum(selector_grandfathered_violations) > 50
  for: 10m
  annotations:
    summary: "High number of grandfathered constraint violations"

# Selection processing delays
- alert: SelectorSlowProcessing
  expr: histogram_quantile(0.95, selector_process_duration_seconds) > 30
  for: 5m
  annotations:
    summary: "Selector processing taking longer than expected"
```

## Promotion Sample-Count Thresholds

Rule 3 (testing → active) and Rule 5 (candidate → testing) each require a
minimum number of `log_scores` rows per (monitor, server) within the
trailing 24-hour window before a promotion is allowed. The window is set
in the `GetMonitorPriority` SQL query (`interval 24 hour`); the
thresholds live in `monitorsettings`.

The thresholds are derived from the system-wide `monitors` setting so they
scale with the configured polling cadence:

```
required = min(historicalFloor, ceil(0.80 × maxSamplesInWindow))
maxSamplesInWindow = 24h / pollInterval
```

with an absolute floor of 8. `historicalFloor` is 32 for testing → active
and 9 for candidate → testing.

`pollInterval` differs by transition:

- **testing → active**: `interval_testing` from `system_settings("monitors")`.
- **candidate → testing**: hardcoded 120-minute fallback branch in
  `GetServers` (candidate-status `server_scores` rows match neither the
  active nor testing branch of the polling-eligibility `OR`).

Resulting thresholds at various `interval_testing` values:

| `interval_testing` | max samples / 24h | testing → active threshold |
|---|---|---|
| 15m | 96 | 32 (capped at historical floor) |
| 30m (default) | 48 | 32 (capped) |
| 45m | 32 | 26 |
| 50m | 28 | 23 |
| 60m | 24 | 20 |
| 4h | 6 | 8 (clamped to absolute floor) |

If `interval_testing` is configured such that the historical 32-sample
floor is unreachable, the monitor-api logs a warning at startup and on
each settings change. Settings are refreshed from `system_settings` every
hour (jittered) by `monitorsettings.Watcher`, with concurrent forced
refreshes coalesced via `singleflight`.

When a testing or candidate monitor is skipped for insufficient samples,
the selector emits a `Debug`-level log line with the monitor id, the
observed count, and the required threshold.

## Implementation Notes

- Metrics are automatically initialized when creating a new `Selector` instance
- Metrics tracking is conditional (`if sl.metrics != nil`) to support testing environments
- All string labels use safe fallback values ("unknown") if monitor identification is missing
- Constraint violation tracking includes both immediate violations and grandfathered cases
- Performance metrics capture the complete selection algorithm execution time
