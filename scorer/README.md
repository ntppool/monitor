# Scorer Package

The scorer processes `log_scores` entries produced by monitoring clients and computes per-server performance scores. It runs as a background loop in `monitor-scorer`, draining new `log_scores` rows in batches and updating `server_scores` and `servers.score_ts` / `score_raw`.

Two scorers are registered:

- `every` — records every log_score (used for full-history scoring).
- `recentmedian` — the main scorer; produces the score visible to clients via the public API.

## Prometheus Metrics

All scorer metrics are registered in `runner.go` (`New()`) and live under the `scorer_` prefix.

### Throughput

#### `scorer_runs`
**Type**: Counter
**Labels**: none

Number of times `Run()` has been invoked (one increment per scheduler tick, not per scorer).

#### `scorer_processed_count`
**Type**: Counter
**Labels**: `scorer`

`log_scores` rows processed, partitioned by scorer name (`every`, `recentmedian`).

#### `scorer_batch_size`
**Type**: Histogram
**Labels**: `scorer`
**Buckets**: 10 → 200, linear (step 10)

Number of records processed per successful batch.

#### `scorer_batch_duration_seconds`
**Type**: Histogram
**Labels**: `scorer`
**Buckets**: 10ms → ~10s, exponential

Wall-clock time to process a single batch (the `process()` function), including the transaction commit and post-commit `UpdateServer` drain.

### Freshness

#### `scorer_last_score_timestamp_seconds`
**Type**: Gauge
**Labels**: `scorer`

Unix timestamp of the newest `log_score.ts` processed in the most recent non-empty batch. Reflects the timestamp of the data, not when it was processed — a stale value here means the scorer is behind real time (queue depth, slow processing, or no fresh data upstream).

Grafana lag expression:
```promql
time() - scorer_last_score_timestamp_seconds
```

#### `scorer_last_batch_timestamp_seconds`
**Type**: Gauge
**Labels**: `scorer`

Unix wall-clock time when the scorer last completed a batch with `count > 0`. If this gauge stops advancing while `time()` keeps moving, the scorer is running but finding nothing to do — usually a signal that upstream monitors are not submitting results.

Alert expression:
```promql
time() - scorer_last_batch_timestamp_seconds > 300
```

Note: only updated on non-empty batches. The metric is unset until the scorer processes at least one batch with work.

### Errors and Retries

#### `scorer_errors`
**Type**: Counter
**Labels**: none

Cumulative count of errors in `Run()` and post-commit `UpdateServer` failures. Includes deadlock exhaustion after the retry budget is spent.

#### `scorer_deadlocks_total`
**Type**: Counter
**Labels**: none

MySQL deadlock errors (Error 1213) encountered during batch processing or post-commit `UpdateServer`. Each retry attempt increments this; a healthy scorer will show occasional deadlocks under load. Sustained growth indicates lock contention.

#### `scorer_retries_total`
**Type**: Counter
**Labels**: `scorer`, `reason`

Retry attempts. Currently emitted with `reason="deadlock"` when a batch is retried via exponential backoff (5s → 60s, up to 10 min total).

### SQL Operations

#### `scorer_sql_updates_total`
**Type**: Counter
**Labels**: `operation`

Per-operation count of individual SQL writes. Operation values:

- `insert_log_score` — new score row written (only when score changed or refresh interval elapsed)
- `update_server_score` — `server_scores.score_raw`/`score_ts` updated
- `update_server_score_status` — `server_scores.status` forced to `active` (scorer monitors are always active when producing scores)
- `insert_server_score` — first-time `server_scores` row for a (server, scorer) pair
- `update_server` — post-commit `servers.score_ts`/`score_raw` update, one per server per batch (recentmedian only)
- `update_server_deduped` — count of redundant per-server updates skipped within a batch (recentmedian only)
- `update_scorer_status` — bookmark advance for the scorer's `last_id`

## Example Queries

**Processing rate per scorer:**
```promql
rate(scorer_processed_count[5m])
```

**Lag of processed data behind real time:**
```promql
time() - scorer_last_score_timestamp_seconds
```

**Minutes since last useful work (detects upstream stalls):**
```promql
(time() - scorer_last_batch_timestamp_seconds) / 60
```

**Deadlock rate:**
```promql
rate(scorer_deadlocks_total[5m])
```

**p95 batch duration:**
```promql
histogram_quantile(0.95, rate(scorer_batch_duration_seconds_bucket[5m]))
```

## Notes

- The scorer also emits OpenTelemetry spans (`scorer.process_batch`) with the same attributes as the metrics — useful for tracing individual slow batches.
- Existing critical alert in `monitoring/grafana/alerts/critical-alerts.yaml`: `rate(scorer_processed_count[10m]) == 0`. The `scorer_last_batch_timestamp_seconds` gauge is a more direct, single-expression alternative for the same signal.
