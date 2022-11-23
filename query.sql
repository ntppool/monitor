-- name: GetMonitorTLSName :one
SELECT * FROM monitors
WHERE tls_name = ? LIMIT 1;

-- name: ListMonitors :many
SELECT * FROM monitors
ORDER BY name;

-- name: GetServer :one
SELECT * FROM servers WHERE id=?;

-- name: GetServerIP :one
SELECT * FROM servers WHERE ip=?;

-- name: GetServerScore :one
SELECT * FROM server_scores
  WHERE
    server_id=? AND
    monitor_id=?;

-- name: UpdateServerScore :exec
UPDATE server_scores
  SET score_ts  = ?,
      score_raw = ?
  WHERE id = ?;

-- name: InsertServerScore :exec
insert into server_scores
  (monitor_id, server_id, score_raw, created_on)
  values (?, ?, ?, ?);

-- name: UpdateServerScoreStratum :exec
UPDATE server_scores
  SET stratum  = ?
  WHERE id = ?;

-- name: UpdateServer :exec
UPDATE servers
  SET score_ts  = sqlc.arg('score_ts'),
      score_raw = sqlc.arg('score_raw')
  WHERE
    id = ?
    AND score_ts < sqlc.arg('score_ts');

-- name: UpdateServerStratum :exec
UPDATE servers
  SET stratum = ?
  WHERE id = ?;

-- name: GetScorers :many
SELECT m.id as ID, s.id as status_id,
  m.status, s.log_score_id, m.name
FROM monitors m, scorer_status s
WHERE m.type = 'score' and (m.id=s.scorer_id);

-- name: GetScorerStatus :many
select s.*,m.name from scorer_status s, monitors m
WHERE m.type = 'score' and (m.id=s.scorer_id);

-- name: UpdateScorerStatus :exec
update scorer_status
  set log_score_id = ?
  where scorer_id = ?;

-- name: InsertScorerStatus :exec
insert into scorer_status
   (scorer_id, log_score_id, modified_on)
   values (?,?,NOW());

-- name: InsertScorer :execresult
insert into monitors
   (type, user_id, account_id,
    name, location, ip, ip_version,
    tls_name, api_key, status, config, client_version, created_on)
    VALUES ('score', NULL, NULL,
            ?, '', NULL, NULL,
            ?, NULL, 'active',
            '', '', NOW());

-- name: GetMinLogScoreID :one
-- https://github.com/kyleconroy/sqlc/issues/1965
select id from log_scores order by id limit 1;

-- name: GetScorerLogScores :many
select ls.* from log_scores ls, monitors m
WHERE
  ls.id > sqlc.arg('log_score_id') AND
  m.type = 'monitor' AND
  monitor_id = m.id
ORDER by id
LIMIT ?;

-- name: GetScorerRecentScores :many
 select ls.*
   from log_scores ls
   inner join
   (select monitor_id, max(ls2.ts) as sts
      from log_scores ls2, monitors m
      where ls2.server_id = sqlc.arg('server_id')
         and ls2.monitor_id=m.id and m.type = 'monitor'
         and ls2.ts <= sqlc.arg('ts')
         and ls2.ts >= date_sub(sqlc.arg('ts'), interval sqlc.arg('time_lookback') second)
      group by monitor_id
   ) as g
   where
     ls.server_id = sqlc.arg('server_id') AND
     g.sts = ls.ts AND
     g.monitor_id = ls.monitor_id
  order by ls.ts;

-- name: UpdateMonitorSeen :exec
UPDATE monitors
  SET last_seen = ?
  WHERE id = ?;

-- name: UpdateMonitorSubmit :exec
UPDATE monitors
  SET last_submit = ?, last_seen = ?
  WHERE id = ?;

-- name: UpdateMonitorVersion :exec
UPDATE monitors
  SET client_version = ?
  WHERE id = ?;

-- name: InsertLogScore :execresult
INSERT INTO log_scores
  (server_id, monitor_id, ts, score, step, offset, rtt, attributes)
  values (?, ?, ?, ?, ?, ?, ?, ?);

-- name: GetServers :many
SELECT s.*
    FROM servers s
    LEFT JOIN server_scores ss
        ON (s.id=ss.server_id)
WHERE (monitor_id = sqlc.arg('monitor_id')
    AND s.ip_version = sqlc.arg('ip_version')
    AND (ss.score_ts IS NULL OR
          (ss.score_raw > -90 AND ss.score_ts <
            DATE_SUB( NOW(), INTERVAL sqlc.arg('interval_minutes') minute)
            OR (ss.score_ts < DATE_SUB( NOW(), INTERVAL 65 minute)) ) )
    AND (s.score_ts IS NULL OR
        (s.score_ts < DATE_SUB( NOW(), INTERVAL sqlc.arg('interval_minutes_all') minute) ))
    AND (deletion_on IS NULL or deletion_on > NOW()))
ORDER BY score_ts
LIMIT  ?
OFFSET ?;
