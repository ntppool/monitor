// Code generated by sqlc. DO NOT EDIT.
// versions:
//   sqlc v1.29.0
// source: query.sql

package ntpdb

import (
	"context"
	"database/sql"
	"encoding/json"
	"strings"
	"time"
)

const clearServerScoreConstraintViolation = `-- name: ClearServerScoreConstraintViolation :exec
UPDATE server_scores
SET constraint_violation_type = NULL,
    constraint_violation_since = NULL
WHERE server_id = ? AND monitor_id = ?
`

type ClearServerScoreConstraintViolationParams struct {
	ServerID  uint32 `json:"server_id"`
	MonitorID uint32 `json:"monitor_id"`
}

func (q *Queries) ClearServerScoreConstraintViolation(ctx context.Context, arg ClearServerScoreConstraintViolationParams) error {
	_, err := q.db.ExecContext(ctx, clearServerScoreConstraintViolation, arg.ServerID, arg.MonitorID)
	return err
}

const deleteServerScore = `-- name: DeleteServerScore :exec
DELETE FROM server_scores
WHERE server_id = ? AND monitor_id = ?
`

type DeleteServerScoreParams struct {
	ServerID  uint32 `json:"server_id"`
	MonitorID uint32 `json:"monitor_id"`
}

// Remove a monitor assignment from a server
func (q *Queries) DeleteServerScore(ctx context.Context, arg DeleteServerScoreParams) error {
	_, err := q.db.ExecContext(ctx, deleteServerScore, arg.ServerID, arg.MonitorID)
	return err
}

const getMinLogScoreID = `-- name: GetMinLogScoreID :one
select id from log_scores order by id limit 1
`

// https://github.com/kyleconroy/sqlc/issues/1965
func (q *Queries) GetMinLogScoreID(ctx context.Context) (uint64, error) {
	row := q.db.QueryRowContext(ctx, getMinLogScoreID)
	var id uint64
	err := row.Scan(&id)
	return id, err
}

const getMonitorPriority = `-- name: GetMonitorPriority :many
select m.id, m.id_token, m.tls_name, m.account_id, m.ip as monitor_ip,
    avg(ls.rtt) / 1000 as avg_rtt,
    round((avg(ls.rtt)/1000) * (1+(2 * (1-avg(ls.step))))) as monitor_priority,
    avg(ls.step) as avg_step,
    if(avg(ls.step) < 0, false, true) as healthy,
    m.status as monitor_status, ss.status as status,
    count(*) as count,
    a.flags as account_flags,
    ss.constraint_violation_type,
    ss.constraint_violation_since
  from log_scores ls
  inner join monitors m
  left join server_scores ss on (ss.server_id = ls.server_id and ss.monitor_id = ls.monitor_id)
  left join accounts a on (m.account_id = a.id)
  where
    m.id = ls.monitor_id
  and ls.server_id = ?
  and m.type = 'monitor'
  and ls.ts > date_sub(now(), interval 12 hour)
  group by m.id, m.id_token, m.tls_name, m.account_id, m.ip, m.status, ss.status, a.flags,
           ss.constraint_violation_type, ss.constraint_violation_since
  order by healthy desc, monitor_priority, avg_step desc, avg_rtt
`

type GetMonitorPriorityRow struct {
	ID                       uint32                 `json:"id"`
	IDToken                  sql.NullString         `json:"id_token"`
	TlsName                  sql.NullString         `json:"tls_name"`
	AccountID                sql.NullInt32          `json:"account_id"`
	MonitorIp                sql.NullString         `json:"monitor_ip"`
	AvgRtt                   interface{}            `json:"avg_rtt"`
	MonitorPriority          float64                `json:"monitor_priority"`
	AvgStep                  interface{}            `json:"avg_step"`
	Healthy                  interface{}            `json:"healthy"`
	MonitorStatus            MonitorsStatus         `json:"monitor_status"`
	Status                   NullServerScoresStatus `json:"status"`
	Count                    int64                  `json:"count"`
	AccountFlags             *json.RawMessage       `json:"account_flags"`
	ConstraintViolationType  sql.NullString         `json:"constraint_violation_type"`
	ConstraintViolationSince sql.NullTime           `json:"constraint_violation_since"`
}

func (q *Queries) GetMonitorPriority(ctx context.Context, serverID uint32) ([]GetMonitorPriorityRow, error) {
	rows, err := q.db.QueryContext(ctx, getMonitorPriority, serverID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var items []GetMonitorPriorityRow
	for rows.Next() {
		var i GetMonitorPriorityRow
		if err := rows.Scan(
			&i.ID,
			&i.IDToken,
			&i.TlsName,
			&i.AccountID,
			&i.MonitorIp,
			&i.AvgRtt,
			&i.MonitorPriority,
			&i.AvgStep,
			&i.Healthy,
			&i.MonitorStatus,
			&i.Status,
			&i.Count,
			&i.AccountFlags,
			&i.ConstraintViolationType,
			&i.ConstraintViolationSince,
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

const getMonitorTLSNameIP = `-- name: GetMonitorTLSNameIP :one
SELECT
  monitors.id, monitors.id_token, monitors.type, monitors.user_id, monitors.account_id, monitors.hostname, monitors.location, monitors.ip, monitors.ip_version, monitors.tls_name, monitors.api_key, monitors.status, monitors.config, monitors.client_version, monitors.last_seen, monitors.last_submit, monitors.created_on, monitors.deleted_on, monitors.is_current,
  accounts.id, accounts.id_token, accounts.name, accounts.organization_name, accounts.organization_url, accounts.public_profile, accounts.url_slug, accounts.flags, accounts.created_on, accounts.modified_on, accounts.stripe_customer_id
FROM monitors
LEFT JOIN accounts ON monitors.account_id = accounts.id
WHERE monitors.tls_name = ?
  -- todo: remove this when v3 monitors are gone
  and (monitors.ip = ? OR "" = ?)
  AND monitors.is_current = 1
  AND monitors.deleted_on is null
LIMIT 1
`

type GetMonitorTLSNameIPParams struct {
	TlsName sql.NullString `json:"tls_name"`
	Ip      sql.NullString `json:"ip"`
}

type GetMonitorTLSNameIPRow struct {
	Monitor Monitor `json:"monitor"`
	Account Account `json:"account"`
}

func (q *Queries) GetMonitorTLSNameIP(ctx context.Context, arg GetMonitorTLSNameIPParams) (GetMonitorTLSNameIPRow, error) {
	row := q.db.QueryRowContext(ctx, getMonitorTLSNameIP, arg.TlsName, arg.Ip, arg.Ip)
	var i GetMonitorTLSNameIPRow
	err := row.Scan(
		&i.Monitor.ID,
		&i.Monitor.IDToken,
		&i.Monitor.Type,
		&i.Monitor.UserID,
		&i.Monitor.AccountID,
		&i.Monitor.Hostname,
		&i.Monitor.Location,
		&i.Monitor.Ip,
		&i.Monitor.IpVersion,
		&i.Monitor.TlsName,
		&i.Monitor.ApiKey,
		&i.Monitor.Status,
		&i.Monitor.Config,
		&i.Monitor.ClientVersion,
		&i.Monitor.LastSeen,
		&i.Monitor.LastSubmit,
		&i.Monitor.CreatedOn,
		&i.Monitor.DeletedOn,
		&i.Monitor.IsCurrent,
		&i.Account.ID,
		&i.Account.IDToken,
		&i.Account.Name,
		&i.Account.OrganizationName,
		&i.Account.OrganizationUrl,
		&i.Account.PublicProfile,
		&i.Account.UrlSlug,
		&i.Account.Flags,
		&i.Account.CreatedOn,
		&i.Account.ModifiedOn,
		&i.Account.StripeCustomerID,
	)
	return i, err
}

const getMonitorsTLSName = `-- name: GetMonitorsTLSName :many
SELECT id, id_token, type, user_id, account_id, hostname, location, ip, ip_version, tls_name, api_key, status, config, client_version, last_seen, last_submit, created_on, deleted_on, is_current FROM monitors
WHERE tls_name = ?
  AND is_current = 1
  AND deleted_on is null
`

func (q *Queries) GetMonitorsTLSName(ctx context.Context, tlsName sql.NullString) ([]Monitor, error) {
	rows, err := q.db.QueryContext(ctx, getMonitorsTLSName, tlsName)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var items []Monitor
	for rows.Next() {
		var i Monitor
		if err := rows.Scan(
			&i.ID,
			&i.IDToken,
			&i.Type,
			&i.UserID,
			&i.AccountID,
			&i.Hostname,
			&i.Location,
			&i.Ip,
			&i.IpVersion,
			&i.TlsName,
			&i.ApiKey,
			&i.Status,
			&i.Config,
			&i.ClientVersion,
			&i.LastSeen,
			&i.LastSubmit,
			&i.CreatedOn,
			&i.DeletedOn,
			&i.IsCurrent,
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

const getScorerLogScores = `-- name: GetScorerLogScores :many
select ls.id, ls.monitor_id, ls.server_id, ls.ts, ls.score, ls.step, ls.offset, ls.rtt, ls.attributes from
  log_scores ls use index (primary),
  monitors m
WHERE
  ls.id >  ? AND
  ls.id < (?+10000) AND
  m.type = 'monitor' AND
  monitor_id = m.id
ORDER by ls.id
LIMIT ?
`

type GetScorerLogScoresParams struct {
	LogScoreID uint64 `json:"log_score_id"`
	Limit      int32  `json:"limit"`
}

func (q *Queries) GetScorerLogScores(ctx context.Context, arg GetScorerLogScoresParams) ([]LogScore, error) {
	rows, err := q.db.QueryContext(ctx, getScorerLogScores, arg.LogScoreID, arg.LogScoreID, arg.Limit)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var items []LogScore
	for rows.Next() {
		var i LogScore
		if err := rows.Scan(
			&i.ID,
			&i.MonitorID,
			&i.ServerID,
			&i.Ts,
			&i.Score,
			&i.Step,
			&i.Offset,
			&i.Rtt,
			&i.Attributes,
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

const getScorerNextLogScoreID = `-- name: GetScorerNextLogScoreID :one
select ls.id from
  log_scores ls use index (primary),
  monitors m
WHERE
  ls.id > ? AND
  m.type = 'monitor' AND
  monitor_id = m.id
ORDER by id
limit 1
`

// this is very slow when there's a backlog, so
// only run it when there are no results to make
// sure we don't get stuck behind a bunch of scoring
// ids.
// https://github.com/kyleconroy/sqlc/issues/1965
func (q *Queries) GetScorerNextLogScoreID(ctx context.Context, logScoreID uint64) (uint64, error) {
	row := q.db.QueryRowContext(ctx, getScorerNextLogScoreID, logScoreID)
	var id uint64
	err := row.Scan(&id)
	return id, err
}

const getScorerRecentScores = `-- name: GetScorerRecentScores :many
 select ls.id, ls.monitor_id, ls.server_id, ls.ts, ls.score, ls.step, ls.offset, ls.rtt, ls.attributes
   from log_scores ls
   inner join
   (select ls2.monitor_id, max(ls2.ts) as sts
      from log_scores ls2,
         monitors m,
         server_scores ss
      where ls2.server_id = ?
         and ls2.monitor_id=m.id and m.type = 'monitor'
         and (ls2.monitor_id=ss.monitor_id and ls2.server_id=ss.server_id)
         and ss.status in (?,?)
         and ls2.ts <= ?
         and ls2.ts >= date_sub(?, interval ? second)
      group by ls2.monitor_id
   ) as g
   where
     ls.server_id = ? AND
     g.sts = ls.ts AND
     g.monitor_id = ls.monitor_id
  order by ls.ts
`

type GetScorerRecentScoresParams struct {
	ServerID       uint32             `json:"server_id"`
	MonitorStatus  ServerScoresStatus `json:"monitor_status"`
	MonitorStatus2 ServerScoresStatus `json:"monitor_status_2"`
	Ts             time.Time          `json:"ts"`
	TimeLookback   interface{}        `json:"time_lookback"`
}

func (q *Queries) GetScorerRecentScores(ctx context.Context, arg GetScorerRecentScoresParams) ([]LogScore, error) {
	rows, err := q.db.QueryContext(ctx, getScorerRecentScores,
		arg.ServerID,
		arg.MonitorStatus,
		arg.MonitorStatus2,
		arg.Ts,
		arg.Ts,
		arg.TimeLookback,
		arg.ServerID,
	)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var items []LogScore
	for rows.Next() {
		var i LogScore
		if err := rows.Scan(
			&i.ID,
			&i.MonitorID,
			&i.ServerID,
			&i.Ts,
			&i.Score,
			&i.Step,
			&i.Offset,
			&i.Rtt,
			&i.Attributes,
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

const getScorerStatus = `-- name: GetScorerStatus :many
select s.id, s.scorer_id, s.log_score_id, s.modified_on,m.hostname from scorer_status s, monitors m
WHERE m.type = 'score' and (m.id=s.scorer_id)
`

type GetScorerStatusRow struct {
	ID         uint32    `json:"id"`
	ScorerID   uint32    `json:"scorer_id"`
	LogScoreID uint64    `json:"log_score_id"`
	ModifiedOn time.Time `json:"modified_on"`
	Hostname   string    `json:"hostname"`
}

func (q *Queries) GetScorerStatus(ctx context.Context) ([]GetScorerStatusRow, error) {
	rows, err := q.db.QueryContext(ctx, getScorerStatus)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var items []GetScorerStatusRow
	for rows.Next() {
		var i GetScorerStatusRow
		if err := rows.Scan(
			&i.ID,
			&i.ScorerID,
			&i.LogScoreID,
			&i.ModifiedOn,
			&i.Hostname,
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

const getScorers = `-- name: GetScorers :many
SELECT m.id as ID, s.id as status_id,
  m.status, s.log_score_id, m.hostname
FROM monitors m, scorer_status s
WHERE
  m.type = 'score'
  and m.status = 'active'
  and (m.id=s.scorer_id)
`

type GetScorersRow struct {
	ID         uint32         `json:"id"`
	StatusID   uint32         `json:"status_id"`
	Status     MonitorsStatus `json:"status"`
	LogScoreID uint64         `json:"log_score_id"`
	Hostname   string         `json:"hostname"`
}

func (q *Queries) GetScorers(ctx context.Context) ([]GetScorersRow, error) {
	rows, err := q.db.QueryContext(ctx, getScorers)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var items []GetScorersRow
	for rows.Next() {
		var i GetScorersRow
		if err := rows.Scan(
			&i.ID,
			&i.StatusID,
			&i.Status,
			&i.LogScoreID,
			&i.Hostname,
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

const getServer = `-- name: GetServer :one
SELECT id, ip, ip_version, user_id, account_id, hostname, stratum, in_pool, in_server_list, netspeed, netspeed_target, created_on, updated_on, score_ts, score_raw, deletion_on, flags FROM servers WHERE id=?
`

func (q *Queries) GetServer(ctx context.Context, id uint32) (Server, error) {
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
		&i.NetspeedTarget,
		&i.CreatedOn,
		&i.UpdatedOn,
		&i.ScoreTs,
		&i.ScoreRaw,
		&i.DeletionOn,
		&i.Flags,
	)
	return i, err
}

const getServerIP = `-- name: GetServerIP :one
SELECT id, ip, ip_version, user_id, account_id, hostname, stratum, in_pool, in_server_list, netspeed, netspeed_target, created_on, updated_on, score_ts, score_raw, deletion_on, flags FROM servers WHERE ip=?
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
		&i.NetspeedTarget,
		&i.CreatedOn,
		&i.UpdatedOn,
		&i.ScoreTs,
		&i.ScoreRaw,
		&i.DeletionOn,
		&i.Flags,
	)
	return i, err
}

const getServerScore = `-- name: GetServerScore :one
SELECT id, monitor_id, server_id, score_ts, score_raw, stratum, status, queue_ts, created_on, modified_on, constraint_violation_type, constraint_violation_since FROM server_scores
  WHERE
    server_id=? AND
    monitor_id=?
`

type GetServerScoreParams struct {
	ServerID  uint32 `json:"server_id"`
	MonitorID uint32 `json:"monitor_id"`
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
		&i.Status,
		&i.QueueTs,
		&i.CreatedOn,
		&i.ModifiedOn,
		&i.ConstraintViolationType,
		&i.ConstraintViolationSince,
	)
	return i, err
}

const getServers = `-- name: GetServers :many
SELECT s.id, s.ip, s.ip_version, s.user_id, s.account_id, s.hostname, s.stratum, s.in_pool, s.in_server_list, s.netspeed, s.netspeed_target, s.created_on, s.updated_on, s.score_ts, s.score_raw, s.deletion_on, s.flags
    FROM servers s
    LEFT JOIN server_scores ss
        ON (s.id=ss.server_id)
WHERE (monitor_id = ?
    AND s.ip_version = ?
    AND (ss.queue_ts IS NULL
          OR (ss.score_raw > -90 AND ss.status = "active"
               AND ss.queue_ts < DATE_SUB( NOW(), INTERVAL ? second))
          OR (ss.score_raw > -90 AND ss.status = "testing"
              AND ss.queue_ts < DATE_SUB( NOW(), INTERVAL ? second))
          OR (ss.queue_ts < DATE_SUB( NOW(), INTERVAL 120 minute)))
    AND (s.score_ts IS NULL OR
        (s.score_ts < DATE_SUB( NOW(), INTERVAL ? second) ))
    AND (deletion_on IS NULL or deletion_on > NOW()))
ORDER BY ss.queue_ts
LIMIT  ?
OFFSET ?
`

type GetServersParams struct {
	MonitorID              uint32           `json:"monitor_id"`
	IpVersion              ServersIpVersion `json:"ip_version"`
	IntervalSeconds        interface{}      `json:"interval_seconds"`
	IntervalSecondsTesting interface{}      `json:"interval_seconds_testing"`
	IntervalSecondsAll     interface{}      `json:"interval_seconds_all"`
	Limit                  int32            `json:"limit"`
	Offset                 int32            `json:"offset"`
}

func (q *Queries) GetServers(ctx context.Context, arg GetServersParams) ([]Server, error) {
	rows, err := q.db.QueryContext(ctx, getServers,
		arg.MonitorID,
		arg.IpVersion,
		arg.IntervalSeconds,
		arg.IntervalSecondsTesting,
		arg.IntervalSecondsAll,
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
			&i.NetspeedTarget,
			&i.CreatedOn,
			&i.UpdatedOn,
			&i.ScoreTs,
			&i.ScoreRaw,
			&i.DeletionOn,
			&i.Flags,
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

const getServersMonitorReview = `-- name: GetServersMonitorReview :many
select server_id from servers_monitor_review
where (next_review <= NOW() OR next_review is NULL)
order by next_review
limit 10
`

func (q *Queries) GetServersMonitorReview(ctx context.Context) ([]uint32, error) {
	rows, err := q.db.QueryContext(ctx, getServersMonitorReview)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var items []uint32
	for rows.Next() {
		var server_id uint32
		if err := rows.Scan(&server_id); err != nil {
			return nil, err
		}
		items = append(items, server_id)
	}
	if err := rows.Close(); err != nil {
		return nil, err
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}
	return items, nil
}

const getSystemSetting = `-- name: GetSystemSetting :one
select value from system_settings where ` + "`" + `key` + "`" + ` = ?
`

func (q *Queries) GetSystemSetting(ctx context.Context, key string) (string, error) {
	row := q.db.QueryRowContext(ctx, getSystemSetting, key)
	var value string
	err := row.Scan(&value)
	return value, err
}

const insertLogScore = `-- name: InsertLogScore :execresult
INSERT INTO log_scores
  (server_id, monitor_id, ts, score, step, offset, rtt, attributes)
  values (?, ?, ?, ?, ?, ?, ?, ?)
`

type InsertLogScoreParams struct {
	ServerID   uint32          `json:"server_id"`
	MonitorID  sql.NullInt32   `json:"monitor_id"`
	Ts         time.Time       `json:"ts"`
	Score      float64         `json:"score"`
	Step       float64         `json:"step"`
	Offset     sql.NullFloat64 `json:"offset"`
	Rtt        sql.NullInt32   `json:"rtt"`
	Attributes sql.NullString  `json:"attributes"`
}

func (q *Queries) InsertLogScore(ctx context.Context, arg InsertLogScoreParams) (sql.Result, error) {
	return q.db.ExecContext(ctx, insertLogScore,
		arg.ServerID,
		arg.MonitorID,
		arg.Ts,
		arg.Score,
		arg.Step,
		arg.Offset,
		arg.Rtt,
		arg.Attributes,
	)
}

const insertScorer = `-- name: InsertScorer :execresult
insert into monitors
   (type, user_id, account_id,
    hostname, location, ip, ip_version,
    tls_name, api_key, status, config, client_version, created_on)
    VALUES ('score', NULL, NULL,
            ?, '', NULL, NULL,
            ?, NULL, 'active',
            '', '', NOW())
`

type InsertScorerParams struct {
	Hostname string         `json:"hostname"`
	TlsName  sql.NullString `json:"tls_name"`
}

func (q *Queries) InsertScorer(ctx context.Context, arg InsertScorerParams) (sql.Result, error) {
	return q.db.ExecContext(ctx, insertScorer, arg.Hostname, arg.TlsName)
}

const insertScorerStatus = `-- name: InsertScorerStatus :exec
insert into scorer_status
   (scorer_id, log_score_id, modified_on)
   values (?,?,NOW())
`

type InsertScorerStatusParams struct {
	ScorerID   uint32 `json:"scorer_id"`
	LogScoreID uint64 `json:"log_score_id"`
}

func (q *Queries) InsertScorerStatus(ctx context.Context, arg InsertScorerStatusParams) error {
	_, err := q.db.ExecContext(ctx, insertScorerStatus, arg.ScorerID, arg.LogScoreID)
	return err
}

const insertServerScore = `-- name: InsertServerScore :exec
insert into server_scores
  (monitor_id, server_id, score_raw, created_on)
  values (?, ?, ?, ?)
ON DUPLICATE KEY UPDATE
  score_raw = VALUES(score_raw)
`

type InsertServerScoreParams struct {
	MonitorID uint32    `json:"monitor_id"`
	ServerID  uint32    `json:"server_id"`
	ScoreRaw  float64   `json:"score_raw"`
	CreatedOn time.Time `json:"created_on"`
}

func (q *Queries) InsertServerScore(ctx context.Context, arg InsertServerScoreParams) error {
	_, err := q.db.ExecContext(ctx, insertServerScore,
		arg.MonitorID,
		arg.ServerID,
		arg.ScoreRaw,
		arg.CreatedOn,
	)
	return err
}

const updateMonitorSeen = `-- name: UpdateMonitorSeen :exec
UPDATE monitors
  SET last_seen = ?
  WHERE id = ?
`

type UpdateMonitorSeenParams struct {
	LastSeen sql.NullTime `json:"last_seen"`
	ID       uint32       `json:"id"`
}

func (q *Queries) UpdateMonitorSeen(ctx context.Context, arg UpdateMonitorSeenParams) error {
	_, err := q.db.ExecContext(ctx, updateMonitorSeen, arg.LastSeen, arg.ID)
	return err
}

const updateMonitorSubmit = `-- name: UpdateMonitorSubmit :exec
UPDATE monitors
  SET last_submit = ?, last_seen = ?
  WHERE id = ?
`

type UpdateMonitorSubmitParams struct {
	LastSubmit sql.NullTime `json:"last_submit"`
	LastSeen   sql.NullTime `json:"last_seen"`
	ID         uint32       `json:"id"`
}

func (q *Queries) UpdateMonitorSubmit(ctx context.Context, arg UpdateMonitorSubmitParams) error {
	_, err := q.db.ExecContext(ctx, updateMonitorSubmit, arg.LastSubmit, arg.LastSeen, arg.ID)
	return err
}

const updateMonitorVersion = `-- name: UpdateMonitorVersion :exec
UPDATE monitors
  SET client_version = ?
  WHERE id = ?
`

type UpdateMonitorVersionParams struct {
	ClientVersion string `json:"client_version"`
	ID            uint32 `json:"id"`
}

func (q *Queries) UpdateMonitorVersion(ctx context.Context, arg UpdateMonitorVersionParams) error {
	_, err := q.db.ExecContext(ctx, updateMonitorVersion, arg.ClientVersion, arg.ID)
	return err
}

const updateScorerStatus = `-- name: UpdateScorerStatus :exec
update scorer_status
  set log_score_id = ?
  where scorer_id = ?
`

type UpdateScorerStatusParams struct {
	LogScoreID uint64 `json:"log_score_id"`
	ScorerID   uint32 `json:"scorer_id"`
}

func (q *Queries) UpdateScorerStatus(ctx context.Context, arg UpdateScorerStatusParams) error {
	_, err := q.db.ExecContext(ctx, updateScorerStatus, arg.LogScoreID, arg.ScorerID)
	return err
}

const updateServer = `-- name: UpdateServer :exec
UPDATE servers
  SET score_ts  = ?,
      score_raw = ?
  WHERE
    id = ?
    AND (score_ts < ? OR score_ts is NULL)
`

type UpdateServerParams struct {
	ScoreTs  sql.NullTime `json:"score_ts"`
	ScoreRaw float64      `json:"score_raw"`
	ID       uint32       `json:"id"`
}

func (q *Queries) UpdateServer(ctx context.Context, arg UpdateServerParams) error {
	_, err := q.db.ExecContext(ctx, updateServer,
		arg.ScoreTs,
		arg.ScoreRaw,
		arg.ID,
		arg.ScoreTs,
	)
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
	ID       uint64       `json:"id"`
}

func (q *Queries) UpdateServerScore(ctx context.Context, arg UpdateServerScoreParams) error {
	_, err := q.db.ExecContext(ctx, updateServerScore, arg.ScoreTs, arg.ScoreRaw, arg.ID)
	return err
}

const updateServerScoreConstraintViolation = `-- name: UpdateServerScoreConstraintViolation :exec
UPDATE server_scores
SET constraint_violation_type = ?,
    constraint_violation_since = ?
WHERE server_id = ? AND monitor_id = ?
`

type UpdateServerScoreConstraintViolationParams struct {
	ConstraintViolationType  sql.NullString `json:"constraint_violation_type"`
	ConstraintViolationSince sql.NullTime   `json:"constraint_violation_since"`
	ServerID                 uint32         `json:"server_id"`
	MonitorID                uint32         `json:"monitor_id"`
}

func (q *Queries) UpdateServerScoreConstraintViolation(ctx context.Context, arg UpdateServerScoreConstraintViolationParams) error {
	_, err := q.db.ExecContext(ctx, updateServerScoreConstraintViolation,
		arg.ConstraintViolationType,
		arg.ConstraintViolationSince,
		arg.ServerID,
		arg.MonitorID,
	)
	return err
}

const updateServerScoreQueue = `-- name: UpdateServerScoreQueue :exec
UPDATE server_scores
  SET queue_ts  = ?
  WHERE
    monitor_id = ?
    AND server_id IN (/*SLICE:server_ids*/?)
    AND (queue_ts < ?
         OR queue_ts is NULL)
`

type UpdateServerScoreQueueParams struct {
	QueueTs   sql.NullTime `json:"queue_ts"`
	MonitorID uint32       `json:"monitor_id"`
	ServerIds []uint32     `json:"server_ids"`
}

func (q *Queries) UpdateServerScoreQueue(ctx context.Context, arg UpdateServerScoreQueueParams) error {
	query := updateServerScoreQueue
	var queryParams []interface{}
	queryParams = append(queryParams, arg.QueueTs)
	queryParams = append(queryParams, arg.MonitorID)
	if len(arg.ServerIds) > 0 {
		for _, v := range arg.ServerIds {
			queryParams = append(queryParams, v)
		}
		query = strings.Replace(query, "/*SLICE:server_ids*/?", strings.Repeat(",?", len(arg.ServerIds))[1:], 1)
	} else {
		query = strings.Replace(query, "/*SLICE:server_ids*/?", "NULL", 1)
	}
	queryParams = append(queryParams, arg.QueueTs)
	_, err := q.db.ExecContext(ctx, query, queryParams...)
	return err
}

const updateServerScoreStatus = `-- name: UpdateServerScoreStatus :exec
update server_scores
  set status = ?
  where monitor_id = ? and server_id = ?
`

type UpdateServerScoreStatusParams struct {
	Status    ServerScoresStatus `json:"status"`
	MonitorID uint32             `json:"monitor_id"`
	ServerID  uint32             `json:"server_id"`
}

func (q *Queries) UpdateServerScoreStatus(ctx context.Context, arg UpdateServerScoreStatusParams) error {
	_, err := q.db.ExecContext(ctx, updateServerScoreStatus, arg.Status, arg.MonitorID, arg.ServerID)
	return err
}

const updateServerScoreStratum = `-- name: UpdateServerScoreStratum :exec
UPDATE server_scores
  SET stratum = ?
  WHERE id = ?
`

type UpdateServerScoreStratumParams struct {
	Stratum sql.NullInt16 `json:"stratum"`
	ID      uint64        `json:"id"`
}

func (q *Queries) UpdateServerScoreStratum(ctx context.Context, arg UpdateServerScoreStratumParams) error {
	_, err := q.db.ExecContext(ctx, updateServerScoreStratum, arg.Stratum, arg.ID)
	return err
}

const updateServerStratum = `-- name: UpdateServerStratum :exec
UPDATE servers
  SET stratum = ?
  WHERE
    id = ?
    and stratum != ?
`

type UpdateServerStratumParams struct {
	Stratum sql.NullInt16 `json:"stratum"`
	ID      uint32        `json:"id"`
}

func (q *Queries) UpdateServerStratum(ctx context.Context, arg UpdateServerStratumParams) error {
	_, err := q.db.ExecContext(ctx, updateServerStratum, arg.Stratum, arg.ID, arg.Stratum)
	return err
}

const updateServersMonitorReview = `-- name: UpdateServersMonitorReview :exec
update servers_monitor_review
  set last_review=NOW(), next_review=?
  where server_id=?
`

type UpdateServersMonitorReviewParams struct {
	NextReview sql.NullTime `json:"next_review"`
	ServerID   uint32       `json:"server_id"`
}

func (q *Queries) UpdateServersMonitorReview(ctx context.Context, arg UpdateServersMonitorReviewParams) error {
	_, err := q.db.ExecContext(ctx, updateServersMonitorReview, arg.NextReview, arg.ServerID)
	return err
}

const updateServersMonitorReviewChanged = `-- name: UpdateServersMonitorReviewChanged :exec
update servers_monitor_review
  set last_review=NOW(), last_change=NOW(), next_review=?
  where server_id=?
`

type UpdateServersMonitorReviewChangedParams struct {
	NextReview sql.NullTime `json:"next_review"`
	ServerID   uint32       `json:"server_id"`
}

func (q *Queries) UpdateServersMonitorReviewChanged(ctx context.Context, arg UpdateServersMonitorReviewChangedParams) error {
	_, err := q.db.ExecContext(ctx, updateServersMonitorReviewChanged, arg.NextReview, arg.ServerID)
	return err
}
