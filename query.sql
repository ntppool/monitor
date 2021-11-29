-- name: GetMonitorAPIKey :one
SELECT * FROM monitors
WHERE api_key = ? LIMIT 1;

-- name: ListMonitors :many
SELECT * FROM monitors
ORDER BY name;

-- name: GetServers :many
SELECT s.*
    FROM servers s
    LEFT JOIN server_scores ss
        ON (s.id=ss.server_id)
WHERE (monitor_id = sqlc.arg('monitor_id')
    AND s.ip_version = sqlc.arg('ip_version')
    AND (ss.score_ts IS NULL OR
          (ss.score_raw > -90 AND ss.score_ts <
            DATE_SUB( NOW(), INTERVAL sqlc.arg(interval_minutes) minute)
            OR (ss.score_ts < DATE_SUB( NOW(), INTERVAL 65 minute)) ) )
    AND (s.score_ts IS NULL OR
        (s.score_ts < DATE_SUB( NOW(), INTERVAL sqlc.arg('interval_minutes_all') minute) ))
    AND (deletion_on IS NULL or deletion_on > NOW()))
ORDER BY score_ts
LIMIT  ?
OFFSET ?;
