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
  SET log_score_id = $1
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
            '', '', NOW())
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
  (server_id, monitor_id, ts, score, step, "offset", rtt, attributes)
  VALUES ($1, $2, $3, $4, $5, $6, $7, $8)
RETURNING id;

-- name: GetServers :many
SELECT s.*
    FROM servers s
    LEFT JOIN server_scores ss
        ON (s.id = ss.server_id)
WHERE (monitor_id = sqlc.arg('monitor_id')
    AND s.ip_version = sqlc.arg('ip_version')
    AND (ss.queue_ts IS NULL
          OR (ss.score_raw > -90 AND ss.status = 'active'
               AND ss.queue_ts < NOW() - make_interval(secs => sqlc.arg('interval_seconds')))
          OR (ss.score_raw > -90 AND ss.status = 'testing'
              AND ss.queue_ts < NOW() - make_interval(secs => sqlc.arg('interval_seconds_testing')))
          OR (ss.queue_ts < NOW() - INTERVAL '120 minutes'))
    AND (s.score_ts IS NULL OR
        (s.score_ts < NOW() - make_interval(secs => sqlc.arg('interval_seconds_all'))))
    AND (deletion_on IS NULL OR deletion_on > NOW()))
ORDER BY ss.queue_ts
LIMIT $1
OFFSET $2;

-- name: GetMonitorPriority :many
SELECT m.id, m.id_token, m.tls_name, m.account_id, m.ip as monitor_ip,
    avg(ls.rtt) / 1000 as avg_rtt,
    0 + round((avg(ls.rtt) / 1000) * (1 + (2 * (1 - avg(ls.step))))) as monitor_priority,
    avg(ls.step) as avg_step,
    CASE WHEN avg(ls.step) < 0 THEN false ELSE true END as healthy,
    m.status as monitor_status, ss.status as status,
    count(*) as count,
    a.flags as account_flags,
    ss.constraint_violation_type,
    ss.constraint_violation_since,
    ss.last_constraint_check,
    ss.pause_reason
  FROM log_scores ls
  INNER JOIN monitors m ON m.id = ls.monitor_id
  LEFT JOIN server_scores ss ON (ss.server_id = ls.server_id AND ss.monitor_id = ls.monitor_id)
  LEFT JOIN accounts a ON (m.account_id = a.id)
  WHERE ls.server_id = $1
    AND m.type = 'monitor'
    AND ls.ts > NOW() - INTERVAL '24 hours'
  GROUP BY m.id, m.id_token, m.tls_name, m.account_id, m.ip, m.status, ss.status, a.flags,
           ss.constraint_violation_type, ss.constraint_violation_since, ss.last_constraint_check, ss.pause_reason
  ORDER BY healthy DESC, monitor_priority, avg_step DESC, avg_rtt;

-- name: GetServersMonitorReview :many
SELECT server_id FROM servers_monitor_review
WHERE (next_review <= NOW() OR next_review IS NULL)
ORDER BY next_review
LIMIT 10;

-- name: UpdateServersMonitorReview :exec
UPDATE servers_monitor_review
  SET last_review = NOW(), next_review = $1
  WHERE server_id = $2;

-- name: UpdateServersMonitorReviewChanged :exec
UPDATE servers_monitor_review
  SET last_review = NOW(), last_change = NOW(), next_review = $1
  WHERE server_id = $2;

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
