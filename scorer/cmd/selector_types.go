package cmd

import (
	"go.ntppool.org/monitor/ntpdb"
)

// newStatus represents a monitor's current and proposed state for a server
type newStatus struct {
	MonitorID     uint32
	MonitorStatus ntpdb.MonitorsStatus
	CurrentStatus ntpdb.ServerScoresStatus
	NewState      candidateState
	RTT           float64
}

// newStatusList is a slice of newStatus entries
type newStatusList []newStatus
