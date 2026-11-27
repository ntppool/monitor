# Selector Package

The selector package implements the monitor selection algorithm for the NTP Pool monitoring system. It manages which monitors are assigned to which servers based on constraints, performance metrics, and availability.

## Prometheus Metrics

The selector exports metrics on its metrics port (default 9000), along
with the process (`process_*`) and Go runtime (`go_*`) metrics.

Metrics are labeled per monitor where useful and never per server. A
`server_id` label used to make the selector export well over 100k
series; per-server detail is in the `server_scores` table and the
"applied status change" log lines.

### Status Changes

#### `selector_status_changes_total`
**Type**: Counter
**Labels**: `monitor_id_token`, `monitor_tls_name`, `from_status`, `to_status`, `reason`

Monitor status changes applied by the selector.

**Status values**: `candidate`, `testing`, `active`, `paused`

**Reasons** (fixed strings; see `selector/process.go`):
- `candidate to testing`, `promotion to active`
- `replacement promotion`, `replaced by better candidate`
- `active-testing swap (promote)`, `active-testing swap (demote)`
- `gradual removal (health or constraints)`, `blocked by constraints or global status`
- `excess active demotion`, `excess testing monitors`
- `bootstrap: promoting healthy candidate` and the `emergency promotion` variants

#### `selector_promotions_limited_total`
**Type**: Counter
**Labels**: `to_status`, `kind`

Server reviews where the daily promotion budget refused a promotion, at
most once per review for each `to_status` and `kind` (`fill`, `health`,
`performance`). Rate it to see replacements skipped by the limit.

### Constraint Violations

#### `selector_constraint_violations_total`
**Type**: Counter
**Labels**: `monitor_id_token`, `monitor_tls_name`, `constraint_type`

Incremented once per server review for each evaluated monitor that is in
constraint violation.

**Constraint types**:
- `network_same_subnet` - Monitor and server in same /24 (IPv4) or /48 (IPv6) subnet
- `account` - Monitor and server belong to the same account
- `limit` - Account has exceeded per-server monitor limit
- `network_diversity` - Multiple monitors in same /20 (IPv4) or /44 (IPv6) network

### Per-Monitor Assignments

#### `selector_monitor_servers`
**Type**: Gauge
**Labels**: `monitor_id_token`, `monitor_tls_name`, `status`

Number of servers each monitor is assigned to, by `server_scores`
status. Refreshed from the database every minute, so removed monitors
drop out.

### Per-Server Pool Sizes

#### `selector_server_monitors`
**Type**: Histogram
**Labels**: `status`

Number of monitors in each status (`active`, `testing`, `candidate`,
`paused`, `reserve`) on a server, observed each time the server is
reviewed.

### Processing

#### `selector_process_duration_seconds`
**Type**: Histogram

Time spent processing one server.

#### `selector_monitors_evaluated_total`, `selector_changes_applied_total`, `selector_changes_failed_total`
**Type**: Counter

Monitors evaluated, and status changes applied or failed.

## Monitor Identification

- **`monitor_id_token`**: Unique per monitor; distinguishes the IPv4 and IPv6 monitors of a host
- **`monitor_tls_name`**: Human-readable name, shared by a host's IPv4 and IPv6 monitors

## Example Queries

```promql
# Servers per monitor and status
sum by (monitor_tls_name, status) (selector_monitor_servers)

# All monitor/server assignments by status
sum by (status) (selector_monitor_servers)

# Share of reviewed servers with no active monitor
sum(rate(selector_server_monitors_bucket{status="active", le="0"}[1h]))
  / sum(rate(selector_server_monitors_count{status="active"}[1h]))

# Average active monitors per reviewed server
sum(rate(selector_server_monitors_sum{status="active"}[1h]))
  / sum(rate(selector_server_monitors_count{status="active"}[1h]))

# Status changes per monitor
sum by (monitor_tls_name, from_status, to_status) (increase(selector_status_changes_total[1h]))

# Constraint violations by type
sum by (constraint_type) (rate(selector_constraint_violations_total[5m]))

# Processing time
histogram_quantile(0.95, rate(selector_process_duration_seconds_bucket[5m]))
```

## Promotion Sample-Count Thresholds

Rule 3 (testing → active) and Rule 5 (candidate → testing) each require a
minimum number of `log_scores` rows per (monitor, server) within a trailing
window before a promotion is allowed. The window is the `sample_window`
setting (default 24h) for testing and active rows and the `candidate_window`
setting (default 72h) for candidates. The selector passes both to the
`GetMonitorPriority` SQL query, with `max(sample_window, candidate_window)`
as the bound on the `log_scores` join; the thresholds live in
`monitorsettings`.

The thresholds are derived from the system-wide `monitors` setting so they
scale with the configured polling cadence:

```
required = min(historicalFloor, ceil(0.80 × maxSamplesInWindow))
maxSamplesInWindow = window / pollInterval
```

with an absolute floor of 8. `historicalFloor` is 32 for testing → active
and 9 for candidate → testing.

`pollInterval` differs by transition:

- **testing → active**: `interval_testing` from `system_settings("monitors")`.
- **candidate → testing**: `interval_candidate` from
  `system_settings("monitors")` (default 2h, 4h once the api sets it), the
  cadence at which `GetServers` hands out candidates. The window is
  `candidate_window`: at 4h over 72h that is 18 possible samples,
  `ceil(0.80 × 18)` is 15, so the threshold stays at the floor of 9.

Resulting thresholds at various `interval_testing` values, with the
default 24h `sample_window`:

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

## Reserve Tier and Candidate Trials

A server keeps `candidate_pool` candidates (default 30) plus the ones on
trial. Every other monitor has the status `reserve`: `GetServers` hands
out no work for it, and no rule counts it except Rule 7 and the trial
rule. A candidate is on trial for `candidate_window` (default 72h) from its
`status_since`.

Rule 7 unpauses a row whose constraint has resolved to `reserve`. The trial
rule runs last in a review, after Rule 6, so a candidate promoted earlier is
neither moved nor counted in the pool. It uses only `status_since`, the
status log and the sample counts, never health:

1. A candidate that is blocked (global status not active or testing, or a
   violation the selector can resolve) goes to `reserve`, reason
   `reserve: blocked`. Violations are the ones a promotion to testing would
   hit (network diversity, account limit), checked as `canPromoteToTesting`
   does, for candidate and reserve rows alike. A candidate with an unchangeable violation (same
   account, same subnet) is left to the pause logic.
2. A candidate past its trial with fewer samples than the
   `candidate → testing` threshold goes to `reserve`, reason `trial_end`.
3. Candidates past their trial beyond `candidate_pool` go to `reserve`, last
   ranked first, reason `trial_end`.
4. When no trial started within `trial_every` (default 12h), one random
   `reserve` row without a violation, of a globally active or testing
   monitor, becomes a candidate, reason `trial_start`. A trial start is a
   status log row `insert` as candidate or `update` from `reserve` to
   candidate; a testing row demoted to candidate isn't one, and neither is
   the row a scorer inserts for itself.

Rules 1-3 move at most 20 candidates per review (`maxReserveMovesPerReview`).
A server with many candidates past their trial is brought down to the pool
over several reviews. Rows with a status this version doesn't know are
skipped.

## Review Cadence

A server is reviewed 2h after a review without changes (`review_every`),
4h after one that changed a status (`review_after_change`) and 1h while it
is in bootstrap mode (`review_bootstrap`). A bootstrap server that looks
bad (more than half of its active monitors with samples are unhealthy) uses
the normal times, since more reviews won't help. The values are in
`system_settings("monitors")`; a value below 1m falls back to the default.
Deleted servers aren't reviewed.

## Implementation Notes

- Metrics are automatically initialized when creating a new `Selector` instance
- Metrics tracking is conditional (`if sl.metrics != nil`) to support testing environments
- All string labels use safe fallback values ("unknown") if monitor identification is missing
- Constraint violation tracking includes both immediate violations and grandfathered cases
- Performance metrics capture the complete selection algorithm execution time
