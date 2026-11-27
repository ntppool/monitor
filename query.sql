-- name: GetMonitorsTLSName :many
SELECT * FROM monitors
WHERE tls_name = $1
  AND is_current = true
  AND deleted_on is null;

-- name: GetMonitorTLSNameIP :one
SELECT
  sqlc.embed(monitors),
  sqlc.embed(accounts)
FROM monitors
LEFT JOIN accounts ON monitors.account_id = accounts.id
WHERE monitors.tls_name = $1
  -- todo: remove this when v3 monitors are gone
  AND (monitors.ip = sqlc.arg('ip') OR '' = sqlc.arg('ip'))
  AND monitors.is_current = true
  AND monitors.deleted_on is null
LIMIT 1;

-- name: GetServer :one
SELECT * FROM servers WHERE id = $1;

-- name: GetServerIP :one
SELECT * FROM servers WHERE ip = $1;

-- name: GetServerScore :one
SELECT * FROM server_scores
  WHERE
    server_id = $1 AND
    monitor_id = $2;

-- name: UpdateServerScore :exec
UPDATE server_scores
  SET score_ts  = $1,
      score_raw = $2
  WHERE id = $3;

-- name: UpdateServerScoreQueue :exec
UPDATE server_scores
  SET queue_ts  = sqlc.arg('queue_ts')
  WHERE
    monitor_id = $1
    AND server_id = ANY(sqlc.arg('server_ids')::bigint[])
    AND (queue_ts < sqlc.arg('queue_ts')
         OR queue_ts is NULL);

-- name: InsertServerScore :exec
INSERT INTO server_scores
  (monitor_id, server_id, score_raw, created_on)
  VALUES ($1, $2, $3, $4)
ON CONFLICT (monitor_id, server_id) DO UPDATE SET
  score_raw = EXCLUDED.score_raw;

-- name: UpdateServerScoreStatus :exec
UPDATE server_scores
  SET status = $1
  WHERE monitor_id = $2 AND server_id = $3;

-- name: UpdateServerScoreStratum :exec
UPDATE server_scores
  SET stratum = $1
  WHERE id = $2;

-- name: UpdateServer :exec
UPDATE servers
  SET score_ts  = sqlc.arg('score_ts'),
      score_raw = sqlc.arg('score_raw')
  WHERE
    id = $1
    AND (score_ts < sqlc.arg('score_ts') OR score_ts is NULL);

-- name: UpdateServerStratum :exec
UPDATE servers
  SET stratum = sqlc.arg('stratum')
  WHERE
    id = sqlc.arg('id')
    and (stratum != sqlc.arg('stratum')
         or stratum is null
    );

-- name: GetScorers :many
SELECT m.id as ID, s.id as status_id,
  m.status, s.log_score_id, m.hostname
FROM monitors m, scorer_status s
WHERE
  m.type = 'score'
  and m.status = 'active'
  and (m.id=s.scorer_id);

-- name: GetScorerStatus :many
select s.*,m.hostname from scorer_status s, monitors m
WHERE m.type = 'score' and (m.id=s.scorer_id);

-- name: UpdateScorerStatus :exec
UPDATE scorer_status
  SET log_score_id = $1, modified_on = NOW()
  WHERE scorer_id = $2;

-- name: InsertScorerStatus :exec
INSERT INTO scorer_status
   (scorer_id, log_score_id, modified_on)
   VALUES ($1, $2, NOW());

-- name: InsertScorer :one
INSERT INTO monitors
   (type, user_id, account_id,
    hostname, location, ip, ip_version,
    tls_name, api_key, status, config, client_version, created_on)
    VALUES ('score', NULL, NULL,
            $1, '', NULL, NULL,
            $2, NULL, 'active',
            '{}', '', NOW())
RETURNING id;

-- name: GetMinLogScoreID :one
-- https://github.com/kyleconroy/sqlc/issues/1965
select id from log_scores order by id limit 1;

-- name: GetScorerLogScores :many
SELECT ls.* FROM
  log_scores ls,
  monitors m
WHERE
  ls.id > sqlc.arg('log_score_id') AND
  ls.id < (sqlc.arg('log_score_id') + 10000) AND
  m.type = 'monitor' AND
  monitor_id = m.id
ORDER BY ls.id
LIMIT $1;

-- name: GetScorerNextLogScoreID :one
--   this is very slow when there's a backlog, so
--   only run it when there are no results to make
--   sure we don't get stuck behind a bunch of scoring
--   ids.
--   https://github.com/kyleconroy/sqlc/issues/1965
SELECT ls.id FROM
  log_scores ls,
  monitors m
WHERE
  ls.id > sqlc.arg('log_score_id') AND
  m.type = 'monitor' AND
  monitor_id = m.id
ORDER BY ls.id
LIMIT 1;

-- name: GetScorerRecentScores :many
SELECT ls.*
  FROM log_scores ls
  INNER JOIN (
    SELECT ls2.monitor_id, max(ls2.ts) as sts
      FROM log_scores ls2,
           monitors m,
           server_scores ss
      WHERE ls2.server_id = sqlc.arg('server_id')
        AND ls2.monitor_id = m.id AND m.type = 'monitor'
        AND (ls2.monitor_id = ss.monitor_id AND ls2.server_id = ss.server_id)
        AND ss.status IN (sqlc.arg('monitor_status'), sqlc.narg('monitor_status_2'))
        AND ls2.ts <= sqlc.arg('ts')
        AND ls2.ts >= sqlc.arg('ts') - make_interval(secs => sqlc.arg('time_lookback'))
      GROUP BY ls2.monitor_id
  ) AS g ON g.sts = ls.ts AND g.monitor_id = ls.monitor_id
  WHERE ls.server_id = sqlc.arg('server_id')
  ORDER BY ls.ts;

-- name: UpdateMonitorSeen :exec
UPDATE monitors
  SET last_seen = $1
  WHERE id = $2;

-- name: UpdateMonitorSubmit :exec
UPDATE monitors
  SET last_submit = $1, last_seen = $2
  WHERE id = $3;

-- name: UpdateMonitorVersion :exec
UPDATE monitors
  SET client_version = $1
  WHERE id = $2;

-- name: InsertLogScore :one
INSERT INTO log_scores
  (server_id, monitor_id, ts, score, step, "offset", rtt, attributes, status)
  VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9)
RETURNING id;

-- name: GetServers :many
SELECT sqlc.embed(s), ss.status, ss.queue_ts AS prev_queue_ts,
    (ss.status IN ('active', 'testing') AND ss.score_raw <= -90)::boolean AS down
    FROM servers s
    JOIN server_scores ss ON (s.id = ss.server_id)
WHERE ss.monitor_id = sqlc.arg('monitor_id')
    AND s.ip_version = sqlc.arg('ip_version')
    AND ((ss.queue_ts IS NULL AND ss.status IN ('candidate', 'testing', 'active'))
          OR (ss.score_raw > -90 AND ss.status = 'active'
              AND ss.queue_ts < NOW() - make_interval(secs => sqlc.arg('interval_seconds')))
          OR (ss.score_raw > -90 AND ss.status = 'testing'
              AND ss.queue_ts < NOW() - make_interval(secs => sqlc.arg('interval_seconds_testing')))
          OR (ss.status = 'candidate'
              AND ss.queue_ts < NOW() - make_interval(secs => sqlc.arg('interval_seconds_candidate')))
          OR (ss.status IN ('active', 'testing') AND ss.score_raw <= -90
              AND ss.queue_ts < NOW() - INTERVAL '120 minutes'))
    AND (s.score_ts IS NULL OR
        (s.score_ts < NOW() - make_interval(secs => sqlc.arg('interval_seconds_all'))))
    AND (s.deletion_on IS NULL OR s.deletion_on > NOW())
ORDER BY ss.queue_ts
LIMIT sqlc.arg('limit') OFFSET sqlc.arg('offset');

-- name: CountLiveServers :many
SELECT ip_version, count(*) AS servers
    FROM servers
WHERE deletion_on IS NULL OR deletion_on > NOW()
GROUP BY ip_version;

-- name: GetMonitorPriority :many
-- Starts from server_scores so rows without recent samples are returned too
-- (count 0, not healthy, sorted after every row with samples). Samples are
-- counted over candidate_window for candidates and sample_window for other
-- rows; the larger of the two bounds the join.
SELECT m.id, m.id_token, m.tls_name, m.account_id, m.ip as monitor_ip,
    -- rtt is NULL for timeouts and implausible values, so the average is
    -- NULL when a monitor has no rtt samples at all for the server
    coalesce(avg(ls.rtt) / 1000, 0)::integer as avg_rtt,
    coalesce(round((avg(ls.rtt) / 1000) * (1 + (2 * (1 - avg(ls.step))))), 0)::integer as monitor_priority,
    coalesce(avg(ls.step), 0)::float8 as avg_step,
    (count(ls.id) > 0 AND coalesce(avg(ls.step), 0) >= 0)::boolean as healthy,
    m.status as monitor_status, ss.status as status,
    ss.status_since,
    count(ls.id) as count,
    a.flags as account_flags,
    ss.constraint_violation_type,
    ss.constraint_violation_since,
    ss.last_constraint_check,
    ss.pause_reason
  FROM server_scores ss
  INNER JOIN monitors m ON m.id = ss.monitor_id
  LEFT JOIN log_scores ls ON (
    ls.server_id = ss.server_id
    AND ls.monitor_id = ss.monitor_id
    AND ls.ts > NOW() - make_interval(secs => GREATEST(sqlc.arg('candidate_window_seconds')::float8, sqlc.arg('sample_window_seconds')::float8))
    AND ls.ts > NOW() - CASE ss.status
        WHEN 'candidate' THEN make_interval(secs => sqlc.arg('candidate_window_seconds')::float8)
        ELSE make_interval(secs => sqlc.arg('sample_window_seconds')::float8) END
  )
  LEFT JOIN accounts a ON (m.account_id = a.id)
  WHERE ss.server_id = sqlc.arg('server_id')
    AND m.type = 'monitor'
  GROUP BY m.id, m.id_token, m.tls_name, m.account_id, m.ip, m.status, ss.status, ss.status_since, a.flags,
           ss.constraint_violation_type, ss.constraint_violation_since, ss.last_constraint_check, ss.pause_reason
  ORDER BY (count(ls.id) > 0) DESC, healthy DESC, monitor_priority, avg_step DESC, avg_rtt;

-- name: GetTrialStartedSince :one
-- Whether a candidate trial started on the server within the window: a row
-- inserted as a candidate, or a reserve row made a candidate. A testing row
-- demoted to candidate is not a trial, and neither is the row a scorer
-- inserts for itself (with the default status candidate).
SELECT EXISTS (
    SELECT 1
      FROM server_scores_status_log l
      WHERE l.server_id = sqlc.arg('server_id')
        AND l.ts > NOW() - make_interval(secs => sqlc.arg('since_seconds')::float8)
        AND ((l.op = 'insert' AND l.new_status = 'candidate')
          OR (l.op = 'update' AND l.old_status = 'reserve' AND l.new_status = 'candidate'))
        AND NOT EXISTS (
            SELECT 1 FROM monitors m
              WHERE m.id = l.monitor_id AND m.type <> 'monitor')
)::boolean AS started;

-- name: GetServerPromotionCounts :one
-- Promotions across each status boundary on a server within the window. A
-- swap's demotion (active -> testing, testing -> candidate) is not counted.
-- Only regular monitors count; scorer rows are not handed out by the selector.
SELECT
    (count(*) FILTER (WHERE l.old_status = 'candidate' AND l.new_status = 'testing'))::bigint AS into_testing,
    (count(*) FILTER (WHERE l.old_status = 'testing' AND l.new_status = 'active'))::bigint AS into_active
  FROM server_scores_status_log l
  INNER JOIN monitors m ON m.id = l.monitor_id
  WHERE l.server_id = sqlc.arg('server_id')
    AND m.type = 'monitor'
    AND l.op = 'update'
    AND l.ts > NOW() - make_interval(secs => sqlc.arg('window_seconds')::float8);

-- name: GetServersMonitorReview :many
SELECT r.server_id FROM servers_monitor_review r
  INNER JOIN servers s ON s.id = r.server_id
WHERE (r.next_review <= NOW() OR r.next_review IS NULL)
  AND (s.deletion_on IS NULL OR s.deletion_on > NOW())
ORDER BY r.next_review
LIMIT 10;

-- name: UpdateServersMonitorReview :exec
UPDATE servers_monitor_review
  SET last_review = NOW(), next_review = $1
  WHERE server_id = $2;

-- name: UpdateServersMonitorReviewChanged :exec
UPDATE servers_monitor_review
  SET last_review = NOW(), last_change = NOW(), next_review = $1
  WHERE server_id = $2;

-- name: GetMonitorServerStatusCounts :many
SELECT m.id_token, m.tls_name, ss.status, count(*) AS servers
  FROM server_scores ss
  INNER JOIN monitors m ON m.id = ss.monitor_id
  WHERE m.type = 'monitor'
  GROUP BY m.id, m.id_token, m.tls_name, ss.status;

-- name: GetSystemSetting :one
SELECT value FROM system_settings WHERE "key" = $1;

-- name: UpdateServerScoreConstraintViolation :exec
UPDATE server_scores
SET constraint_violation_type = $1,
    constraint_violation_since = $2
WHERE server_id = $3 AND monitor_id = $4;

-- name: ClearServerScoreConstraintViolation :exec
UPDATE server_scores
SET constraint_violation_type = NULL,
    constraint_violation_since = NULL,
    last_constraint_check = NOW(),
    pause_reason = NULL
WHERE server_id = $1 AND monitor_id = $2;

-- name: UpdateServerScorePauseReason :exec
UPDATE server_scores
SET pause_reason = $1,
    last_constraint_check = NOW()
WHERE server_id = $2 AND monitor_id = $3;

-- name: UpdateServerScoreLastConstraintCheck :exec
UPDATE server_scores
SET last_constraint_check = NOW()
WHERE server_id = $1 AND monitor_id = $2;


-- name: DeleteServerScore :exec
-- Remove a monitor assignment from a server
DELETE FROM server_scores
WHERE server_id = $1 AND monitor_id = $2;

-- Read-only queries for `monitor-api db settings`. Only regular monitors
-- (type 'monitor') and servers that aren't deleted are counted.

-- name: GetSettingsServerLoad :many
-- Per IP version: servers, the candidates per server, and the servers with
-- at most bootstrap_limit active rows (the selector's bootstrap mode).
WITH monitor_rows AS (
    SELECT ss.server_id, ss.status
      FROM server_scores ss
      INNER JOIN monitors m ON m.id = ss.monitor_id
      WHERE m.type = 'monitor'
), per_server AS (
    SELECT s.id, s.ip_version::text AS ip_version,
        count(r.server_id) FILTER (WHERE r.status = 'candidate') AS candidates,
        count(r.server_id) FILTER (WHERE r.status = 'active') AS active
      FROM servers s
      LEFT JOIN monitor_rows r ON r.server_id = s.id
      WHERE s.deletion_on IS NULL OR s.deletion_on > NOW()
      GROUP BY s.id, s.ip_version
)
SELECT ip_version::text AS ip_version,
    count(*)::bigint AS servers,
    avg(candidates)::float8 AS candidates_avg,
    (percentile_disc(0.5) WITHIN GROUP (ORDER BY candidates))::bigint AS candidates_p50,
    (percentile_disc(0.9) WITHIN GROUP (ORDER BY candidates))::bigint AS candidates_p90,
    max(candidates)::bigint AS candidates_max,
    (count(*) FILTER (WHERE active <= sqlc.arg('bootstrap_limit')::bigint))::bigint AS bootstrap_servers
  FROM per_server
  GROUP BY ip_version
  ORDER BY ip_version;

-- name: GetSettingsStatusCounts :many
-- Per IP version and status: the server_scores rows.
SELECT s.ip_version::text AS ip_version, ss.status, count(*)::bigint AS rows
  FROM server_scores ss
  INNER JOIN servers s ON s.id = ss.server_id
  INNER JOIN monitors m ON m.id = ss.monitor_id
  WHERE m.type = 'monitor'
    AND (s.deletion_on IS NULL OR s.deletion_on > NOW())
  GROUP BY s.ip_version, ss.status
  ORDER BY s.ip_version, ss.status;

-- name: GetSettingsMonitorLoad :many
-- Per IP version: the active and testing monitors, and the candidate rows
-- per monitor (average and max over those monitors).
WITH candidates AS (
    SELECT ss.monitor_id, count(*) AS candidates
      FROM server_scores ss
      INNER JOIN servers s ON s.id = ss.server_id
      WHERE ss.status = 'candidate'
        AND (s.deletion_on IS NULL OR s.deletion_on > NOW())
      GROUP BY ss.monitor_id
)
SELECT m.ip_version::text AS ip_version,
    (count(*) FILTER (WHERE m.status = 'active'))::bigint AS active_monitors,
    (count(*) FILTER (WHERE m.status = 'testing'))::bigint AS testing_monitors,
    coalesce(avg(coalesce(c.candidates, 0)), 0)::float8 AS candidates_avg,
    coalesce(max(c.candidates), 0)::bigint AS candidates_max
  FROM monitors m
  LEFT JOIN candidates c ON c.monitor_id = m.id
  WHERE m.type = 'monitor'
    AND m.status IN ('active', 'testing')
    AND m.ip_version IS NOT NULL
  GROUP BY m.ip_version
  ORDER BY m.ip_version;

-- name: GetSettingsTrialStarts :many
-- Per IP version: the trials started within the window, with the condition
-- of GetTrialStartedSince. A new server's first candidates count as trials
-- too.
SELECT s.ip_version::text AS ip_version, count(*)::bigint AS trials
  FROM server_scores_status_log l
  INNER JOIN servers s ON s.id = l.server_id
  WHERE l.ts > NOW() - make_interval(secs => sqlc.arg('since_seconds')::float8)
    AND ((l.op = 'insert' AND l.new_status = 'candidate')
      OR (l.op = 'update' AND l.old_status = 'reserve' AND l.new_status = 'candidate'))
    AND NOT EXISTS (
        SELECT 1 FROM monitors m
          WHERE m.id = l.monitor_id AND m.type <> 'monitor')
    AND (s.deletion_on IS NULL OR s.deletion_on > NOW())
  GROUP BY s.ip_version
  ORDER BY s.ip_version;

-- name: GetSettingsPromotions :many
-- Per IP version and boundary: the promotions within the window, counted
-- like GetServerPromotionCounts, the most on one server, and the servers
-- with at least budget promotions.
WITH per_server AS (
    SELECT l.server_id,
        count(*) FILTER (WHERE l.old_status = 'candidate' AND l.new_status = 'testing') AS into_testing,
        count(*) FILTER (WHERE l.old_status = 'testing' AND l.new_status = 'active') AS into_active
      FROM server_scores_status_log l
      INNER JOIN monitors m ON m.id = l.monitor_id
      WHERE m.type = 'monitor'
        AND l.op = 'update'
        AND l.ts > NOW() - make_interval(secs => sqlc.arg('window_seconds')::float8)
      GROUP BY l.server_id
)
SELECT s.ip_version::text AS ip_version,
    coalesce(sum(p.into_testing), 0)::bigint AS into_testing,
    coalesce(max(p.into_testing), 0)::bigint AS into_testing_max,
    (count(*) FILTER (WHERE p.into_testing >= sqlc.arg('budget')::bigint))::bigint AS into_testing_at_budget,
    coalesce(sum(p.into_active), 0)::bigint AS into_active,
    coalesce(max(p.into_active), 0)::bigint AS into_active_max,
    (count(*) FILTER (WHERE p.into_active >= sqlc.arg('budget')::bigint))::bigint AS into_active_at_budget
  FROM per_server p
  INNER JOIN servers s ON s.id = p.server_id
  WHERE s.deletion_on IS NULL OR s.deletion_on > NOW()
  GROUP BY s.ip_version
  ORDER BY s.ip_version;
