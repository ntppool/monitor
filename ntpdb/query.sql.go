// Code generated by sqlc. DO NOT EDIT.
// source: query.sql

package ntpdb

import (
	"context"
	"database/sql"
	"time"
)

const getMonitorAPIKey = `-- name: GetMonitorAPIKey :one
SELECT id, user_id, name, ip, ip_version, api_key, config, last_seen, created_on FROM monitors
WHERE api_key = ? LIMIT 1
`

func (q *Queries) GetMonitorAPIKey(ctx context.Context, apiKey string) (Monitor, error) {
	row := q.db.QueryRowContext(ctx, getMonitorAPIKey, apiKey)
	var i Monitor
	err := row.Scan(
		&i.ID,
		&i.UserID,
		&i.Name,
		&i.Ip,
		&i.IpVersion,
		&i.ApiKey,
		&i.Config,
		&i.LastSeen,
		&i.CreatedOn,
	)
	return i, err
}

const getServer = `-- name: GetServer :one
SELECT id, ip, ip_version, user_id, account_id, hostname, stratum, in_pool, in_server_list, netspeed, created_on, updated_on, score_ts, score_raw, deletion_on FROM servers WHERE id=?
`

func (q *Queries) GetServer(ctx context.Context, id int32) (Server, error) {
	row := q.db.QueryRowContext(ctx, getServer, id)
	var i Server
	err := row.Scan(
		&i.ID,
		&i.Ip,
		&i.IpVersion,
		&i.UserID,
		&i.AccountID,
		&i.Hostname,
		&i.Stratum,
		&i.InPool,
		&i.InServerList,
		&i.Netspeed,
		&i.CreatedOn,
		&i.UpdatedOn,
		&i.ScoreTs,
		&i.ScoreRaw,
		&i.DeletionOn,
	)
	return i, err
}

const getServerIP = `-- name: GetServerIP :one
SELECT id, ip, ip_version, user_id, account_id, hostname, stratum, in_pool, in_server_list, netspeed, created_on, updated_on, score_ts, score_raw, deletion_on FROM servers WHERE ip=?
`

func (q *Queries) GetServerIP(ctx context.Context, ip string) (Server, error) {
	row := q.db.QueryRowContext(ctx, getServerIP, ip)
	var i Server
	err := row.Scan(
		&i.ID,
		&i.Ip,
		&i.IpVersion,
		&i.UserID,
		&i.AccountID,
		&i.Hostname,
		&i.Stratum,
		&i.InPool,
		&i.InServerList,
		&i.Netspeed,
		&i.CreatedOn,
		&i.UpdatedOn,
		&i.ScoreTs,
		&i.ScoreRaw,
		&i.DeletionOn,
	)
	return i, err
}

const getServerScore = `-- name: GetServerScore :one
SELECT id, monitor_id, server_id, score_ts, score_raw, stratum, created_on, modified_on FROM server_scores
  WHERE
    server_id=? AND
    monitor_id=?
`

type GetServerScoreParams struct {
	ServerID  int32 `json:"server_id"`
	MonitorID int32 `json:"monitor_id"`
}

func (q *Queries) GetServerScore(ctx context.Context, arg GetServerScoreParams) (ServerScore, error) {
	row := q.db.QueryRowContext(ctx, getServerScore, arg.ServerID, arg.MonitorID)
	var i ServerScore
	err := row.Scan(
		&i.ID,
		&i.MonitorID,
		&i.ServerID,
		&i.ScoreTs,
		&i.ScoreRaw,
		&i.Stratum,
		&i.CreatedOn,
		&i.ModifiedOn,
	)
	return i, err
}

const getServers = `-- name: GetServers :many
SELECT s.id, s.ip, s.ip_version, s.user_id, s.account_id, s.hostname, s.stratum, s.in_pool, s.in_server_list, s.netspeed, s.created_on, s.updated_on, s.score_ts, s.score_raw, s.deletion_on
    FROM servers s
    LEFT JOIN server_scores ss
        ON (s.id=ss.server_id)
WHERE (monitor_id = ?
    AND s.ip_version = ?
    AND (ss.score_ts IS NULL OR
          (ss.score_raw > -90 AND ss.score_ts <
            DATE_SUB( NOW(), INTERVAL ? minute)
            OR (ss.score_ts < DATE_SUB( NOW(), INTERVAL 65 minute)) ) )
    AND (s.score_ts IS NULL OR
        (s.score_ts < DATE_SUB( NOW(), INTERVAL ? minute) ))
    AND (deletion_on IS NULL or deletion_on > NOW()))
ORDER BY score_ts
LIMIT  ?
OFFSET ?
`

type GetServersParams struct {
	MonitorID          int32            `json:"monitor_id"`
	IpVersion          ServersIpVersion `json:"ip_version"`
	IntervalMinutes    interface{}      `json:"interval_minutes"`
	IntervalMinutesAll interface{}      `json:"interval_minutes_all"`
	Limit              int32            `json:"limit"`
	Offset             int32            `json:"offset"`
}

func (q *Queries) GetServers(ctx context.Context, arg GetServersParams) ([]Server, error) {
	rows, err := q.db.QueryContext(ctx, getServers,
		arg.MonitorID,
		arg.IpVersion,
		arg.IntervalMinutes,
		arg.IntervalMinutesAll,
		arg.Limit,
		arg.Offset,
	)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var items []Server
	for rows.Next() {
		var i Server
		if err := rows.Scan(
			&i.ID,
			&i.Ip,
			&i.IpVersion,
			&i.UserID,
			&i.AccountID,
			&i.Hostname,
			&i.Stratum,
			&i.InPool,
			&i.InServerList,
			&i.Netspeed,
			&i.CreatedOn,
			&i.UpdatedOn,
			&i.ScoreTs,
			&i.ScoreRaw,
			&i.DeletionOn,
		); err != nil {
			return nil, err
		}
		items = append(items, i)
	}
	if err := rows.Close(); err != nil {
		return nil, err
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}
	return items, nil
}

const insertLogScore = `-- name: InsertLogScore :exec
INSERT INTO log_scores
  (server_id, monitor_id, ts, score, step, offset, rtt, attributes)
  values (?, ?, ?, ?, ?, ?, ?, ?)
`

type InsertLogScoreParams struct {
	ServerID   int32           `json:"server_id"`
	MonitorID  sql.NullInt32   `json:"monitor_id"`
	Ts         time.Time       `json:"ts"`
	Score      float64         `json:"score"`
	Step       float64         `json:"step"`
	Offset     sql.NullFloat64 `json:"offset"`
	Rtt        sql.NullInt32   `json:"rtt"`
	Attributes sql.NullString  `json:"attributes"`
}

func (q *Queries) InsertLogScore(ctx context.Context, arg InsertLogScoreParams) error {
	_, err := q.db.ExecContext(ctx, insertLogScore,
		arg.ServerID,
		arg.MonitorID,
		arg.Ts,
		arg.Score,
		arg.Step,
		arg.Offset,
		arg.Rtt,
		arg.Attributes,
	)
	return err
}

const listMonitors = `-- name: ListMonitors :many
SELECT id, user_id, name, ip, ip_version, api_key, config, last_seen, created_on FROM monitors
ORDER BY name
`

func (q *Queries) ListMonitors(ctx context.Context) ([]Monitor, error) {
	rows, err := q.db.QueryContext(ctx, listMonitors)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var items []Monitor
	for rows.Next() {
		var i Monitor
		if err := rows.Scan(
			&i.ID,
			&i.UserID,
			&i.Name,
			&i.Ip,
			&i.IpVersion,
			&i.ApiKey,
			&i.Config,
			&i.LastSeen,
			&i.CreatedOn,
		); err != nil {
			return nil, err
		}
		items = append(items, i)
	}
	if err := rows.Close(); err != nil {
		return nil, err
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}
	return items, nil
}

const updateServer = `-- name: UpdateServer :exec
UPDATE servers
  SET score_ts  = ?,
      score_raw = ?
  WHERE id = ?
`

type UpdateServerParams struct {
	ScoreTs  sql.NullTime `json:"score_ts"`
	ScoreRaw float64      `json:"score_raw"`
	ID       int32        `json:"id"`
}

func (q *Queries) UpdateServer(ctx context.Context, arg UpdateServerParams) error {
	_, err := q.db.ExecContext(ctx, updateServer, arg.ScoreTs, arg.ScoreRaw, arg.ID)
	return err
}

const updateServerScore = `-- name: UpdateServerScore :exec
UPDATE server_scores
  SET score_ts  = ?,
      score_raw = ?
  WHERE id = ?
`

type UpdateServerScoreParams struct {
	ScoreTs  sql.NullTime `json:"score_ts"`
	ScoreRaw float64      `json:"score_raw"`
	ID       int64        `json:"id"`
}

func (q *Queries) UpdateServerScore(ctx context.Context, arg UpdateServerScoreParams) error {
	_, err := q.db.ExecContext(ctx, updateServerScore, arg.ScoreTs, arg.ScoreRaw, arg.ID)
	return err
}

const updateServerScoreStratum = `-- name: UpdateServerScoreStratum :exec
UPDATE server_scores
  SET stratum  = ?
  WHERE id = ?
`

type UpdateServerScoreStratumParams struct {
	Stratum sql.NullInt32 `json:"stratum"`
	ID      int64         `json:"id"`
}

func (q *Queries) UpdateServerScoreStratum(ctx context.Context, arg UpdateServerScoreStratumParams) error {
	_, err := q.db.ExecContext(ctx, updateServerScoreStratum, arg.Stratum, arg.ID)
	return err
}

const updateServerStratum = `-- name: UpdateServerStratum :exec
UPDATE servers
  SET stratum = ?
  WHERE id = ?
`

type UpdateServerStratumParams struct {
	Stratum sql.NullInt32 `json:"stratum"`
	ID      int32         `json:"id"`
}

func (q *Queries) UpdateServerStratum(ctx context.Context, arg UpdateServerStratumParams) error {
	_, err := q.db.ExecContext(ctx, updateServerStratum, arg.Stratum, arg.ID)
	return err
}
