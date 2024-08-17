// Code generated by sqlc. DO NOT EDIT.
// versions:
//   sqlc v1.27.0

package ntpdb

import (
	"database/sql"
	"database/sql/driver"
	"fmt"
	"time"
)

type MonitorsIpVersion string

const (
	MonitorsIpVersionV4 MonitorsIpVersion = "v4"
	MonitorsIpVersionV6 MonitorsIpVersion = "v6"
)

func (e *MonitorsIpVersion) Scan(src interface{}) error {
	switch s := src.(type) {
	case []byte:
		*e = MonitorsIpVersion(s)
	case string:
		*e = MonitorsIpVersion(s)
	default:
		return fmt.Errorf("unsupported scan type for MonitorsIpVersion: %T", src)
	}
	return nil
}

type NullMonitorsIpVersion struct {
	MonitorsIpVersion MonitorsIpVersion `json:"monitors_ip_version"`
	Valid             bool              `json:"valid"` // Valid is true if MonitorsIpVersion is not NULL
}

// Scan implements the Scanner interface.
func (ns *NullMonitorsIpVersion) Scan(value interface{}) error {
	if value == nil {
		ns.MonitorsIpVersion, ns.Valid = "", false
		return nil
	}
	ns.Valid = true
	return ns.MonitorsIpVersion.Scan(value)
}

// Value implements the driver Valuer interface.
func (ns NullMonitorsIpVersion) Value() (driver.Value, error) {
	if !ns.Valid {
		return nil, nil
	}
	return string(ns.MonitorsIpVersion), nil
}

type MonitorsStatus string

const (
	MonitorsStatusPending MonitorsStatus = "pending"
	MonitorsStatusTesting MonitorsStatus = "testing"
	MonitorsStatusActive  MonitorsStatus = "active"
	MonitorsStatusPaused  MonitorsStatus = "paused"
	MonitorsStatusDeleted MonitorsStatus = "deleted"
)

func (e *MonitorsStatus) Scan(src interface{}) error {
	switch s := src.(type) {
	case []byte:
		*e = MonitorsStatus(s)
	case string:
		*e = MonitorsStatus(s)
	default:
		return fmt.Errorf("unsupported scan type for MonitorsStatus: %T", src)
	}
	return nil
}

type NullMonitorsStatus struct {
	MonitorsStatus MonitorsStatus `json:"monitors_status"`
	Valid          bool           `json:"valid"` // Valid is true if MonitorsStatus is not NULL
}

// Scan implements the Scanner interface.
func (ns *NullMonitorsStatus) Scan(value interface{}) error {
	if value == nil {
		ns.MonitorsStatus, ns.Valid = "", false
		return nil
	}
	ns.Valid = true
	return ns.MonitorsStatus.Scan(value)
}

// Value implements the driver Valuer interface.
func (ns NullMonitorsStatus) Value() (driver.Value, error) {
	if !ns.Valid {
		return nil, nil
	}
	return string(ns.MonitorsStatus), nil
}

type MonitorsType string

const (
	MonitorsTypeMonitor MonitorsType = "monitor"
	MonitorsTypeScore   MonitorsType = "score"
)

func (e *MonitorsType) Scan(src interface{}) error {
	switch s := src.(type) {
	case []byte:
		*e = MonitorsType(s)
	case string:
		*e = MonitorsType(s)
	default:
		return fmt.Errorf("unsupported scan type for MonitorsType: %T", src)
	}
	return nil
}

type NullMonitorsType struct {
	MonitorsType MonitorsType `json:"monitors_type"`
	Valid        bool         `json:"valid"` // Valid is true if MonitorsType is not NULL
}

// Scan implements the Scanner interface.
func (ns *NullMonitorsType) Scan(value interface{}) error {
	if value == nil {
		ns.MonitorsType, ns.Valid = "", false
		return nil
	}
	ns.Valid = true
	return ns.MonitorsType.Scan(value)
}

// Value implements the driver Valuer interface.
func (ns NullMonitorsType) Value() (driver.Value, error) {
	if !ns.Valid {
		return nil, nil
	}
	return string(ns.MonitorsType), nil
}

type ServerScoresStatus string

const (
	ServerScoresStatusNew     ServerScoresStatus = "new"
	ServerScoresStatusTesting ServerScoresStatus = "testing"
	ServerScoresStatusActive  ServerScoresStatus = "active"
)

func (e *ServerScoresStatus) Scan(src interface{}) error {
	switch s := src.(type) {
	case []byte:
		*e = ServerScoresStatus(s)
	case string:
		*e = ServerScoresStatus(s)
	default:
		return fmt.Errorf("unsupported scan type for ServerScoresStatus: %T", src)
	}
	return nil
}

type NullServerScoresStatus struct {
	ServerScoresStatus ServerScoresStatus `json:"server_scores_status"`
	Valid              bool               `json:"valid"` // Valid is true if ServerScoresStatus is not NULL
}

// Scan implements the Scanner interface.
func (ns *NullServerScoresStatus) Scan(value interface{}) error {
	if value == nil {
		ns.ServerScoresStatus, ns.Valid = "", false
		return nil
	}
	ns.Valid = true
	return ns.ServerScoresStatus.Scan(value)
}

// Value implements the driver Valuer interface.
func (ns NullServerScoresStatus) Value() (driver.Value, error) {
	if !ns.Valid {
		return nil, nil
	}
	return string(ns.ServerScoresStatus), nil
}

type ServersIpVersion string

const (
	ServersIpVersionV4 ServersIpVersion = "v4"
	ServersIpVersionV6 ServersIpVersion = "v6"
)

func (e *ServersIpVersion) Scan(src interface{}) error {
	switch s := src.(type) {
	case []byte:
		*e = ServersIpVersion(s)
	case string:
		*e = ServersIpVersion(s)
	default:
		return fmt.Errorf("unsupported scan type for ServersIpVersion: %T", src)
	}
	return nil
}

type NullServersIpVersion struct {
	ServersIpVersion ServersIpVersion `json:"servers_ip_version"`
	Valid            bool             `json:"valid"` // Valid is true if ServersIpVersion is not NULL
}

// Scan implements the Scanner interface.
func (ns *NullServersIpVersion) Scan(value interface{}) error {
	if value == nil {
		ns.ServersIpVersion, ns.Valid = "", false
		return nil
	}
	ns.Valid = true
	return ns.ServersIpVersion.Scan(value)
}

// Value implements the driver Valuer interface.
func (ns NullServersIpVersion) Value() (driver.Value, error) {
	if !ns.Valid {
		return nil, nil
	}
	return string(ns.ServersIpVersion), nil
}

type LogScore struct {
	ID         uint64          `json:"id"`
	MonitorID  sql.NullInt32   `json:"monitor_id"`
	ServerID   uint32          `json:"server_id"`
	Ts         time.Time       `json:"ts"`
	Score      float64         `json:"score"`
	Step       float64         `json:"step"`
	Offset     sql.NullFloat64 `json:"offset"`
	Rtt        sql.NullInt32   `json:"rtt"`
	Attributes sql.NullString  `json:"attributes"`
}

type Monitor struct {
	ID            uint32                `json:"id"`
	Type          MonitorsType          `json:"type"`
	UserID        sql.NullInt32         `json:"user_id"`
	AccountID     sql.NullInt32         `json:"account_id"`
	Name          string                `json:"name"`
	Location      string                `json:"location"`
	Ip            sql.NullString        `json:"ip"`
	IpVersion     NullMonitorsIpVersion `json:"ip_version"`
	TlsName       sql.NullString        `json:"tls_name"`
	ApiKey        sql.NullString        `json:"api_key"`
	Status        MonitorsStatus        `json:"status"`
	Config        string                `json:"config"`
	ClientVersion string                `json:"client_version"`
	LastSeen      sql.NullTime          `json:"last_seen"`
	LastSubmit    sql.NullTime          `json:"last_submit"`
	CreatedOn     time.Time             `json:"created_on"`
}

type Server struct {
	ID             uint32           `json:"id"`
	Ip             string           `json:"ip"`
	IpVersion      ServersIpVersion `json:"ip_version"`
	UserID         sql.NullInt32    `json:"user_id"`
	AccountID      sql.NullInt32    `json:"account_id"`
	Hostname       sql.NullString   `json:"hostname"`
	Stratum        sql.NullInt16    `json:"stratum"`
	InPool         uint8            `json:"in_pool"`
	InServerList   uint8            `json:"in_server_list"`
	Netspeed       uint32           `json:"netspeed"`
	NetspeedTarget uint32           `json:"netspeed_target"`
	CreatedOn      time.Time        `json:"created_on"`
	UpdatedOn      time.Time        `json:"updated_on"`
	ScoreTs        sql.NullTime     `json:"score_ts"`
	ScoreRaw       float64          `json:"score_raw"`
	DeletionOn     sql.NullTime     `json:"deletion_on"`
	Flags          string           `json:"flags"`
}

type ServerScore struct {
	ID         uint64             `json:"id"`
	MonitorID  uint32             `json:"monitor_id"`
	ServerID   uint32             `json:"server_id"`
	ScoreTs    sql.NullTime       `json:"score_ts"`
	ScoreRaw   float64            `json:"score_raw"`
	Stratum    sql.NullInt16      `json:"stratum"`
	Status     ServerScoresStatus `json:"status"`
	QueueTs    sql.NullTime       `json:"queue_ts"`
	CreatedOn  time.Time          `json:"created_on"`
	ModifiedOn time.Time          `json:"modified_on"`
}
