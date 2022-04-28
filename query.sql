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

-- name: UpdateServerScoreStratum :exec
UPDATE server_scores
  SET stratum  = ?
  WHERE id = ?;

-- name: UpdateServer :exec
UPDATE servers
  SET score_ts  = ?,
      score_raw = ?
  WHERE id = ?;

-- name: UpdateServerStratum :exec
UPDATE servers
  SET stratum = ?
  WHERE id = ?;

-- name: UpdateMonitorSeen :exec
UPDATE monitors
  SET last_seen = ?
  WHERE id = ?;

-- name: InsertLogScore :exec
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
