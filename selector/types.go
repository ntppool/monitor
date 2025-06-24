package selector

import (
	"time"

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

// constraintViolationType identifies the type of constraint violation
type constraintViolationType string

const (
	violationNone              constraintViolationType = ""                    // No violation
	violationNetworkSameSubnet constraintViolationType = "network_same_subnet" // Monitor and server in same subnet
	violationAccount           constraintViolationType = "account"             // Same account
	violationLimit             constraintViolationType = "limit"               // Account limit exceeded
	violationNetworkDiversity  constraintViolationType = "network_diversity"   // Multiple monitors in same /44 or /20 network
)

// constraintViolation describes a constraint violation
type constraintViolation struct {
	Type            constraintViolationType
	Since           time.Time
	IsGrandfathered bool
	Details         string
}

// monitorCandidate represents a monitor being evaluated for a server
type monitorCandidate struct {
	ID                       uint32
	AccountID                *uint32
	IP                       string
	GlobalStatus             ntpdb.MonitorsStatus
	ServerStatus             ntpdb.ServerScoresStatus
	HasMetrics               bool
	IsHealthy                bool
	RTT                      float64
	ConstraintViolationType  *string
	ConstraintViolationSince *time.Time
}

// serverInfo contains server details needed for constraint checking
type serverInfo struct {
	ID        uint32
	AccountID *uint32
	IP        string
	IPVersion string
}

// evaluatedMonitor combines a monitor candidate with its constraint evaluation
type evaluatedMonitor struct {
	monitor          monitorCandidate
	violation        *constraintViolation
	recommendedState candidateState
}

// monitorCategories groups monitors by their current status
type monitorCategories struct {
	active              []evaluatedMonitor
	testing             []evaluatedMonitor
	candidate           []evaluatedMonitor
	available           []evaluatedMonitor // Not assigned to this server
	blocked             []evaluatedMonitor // Constraint violations
	globallyActiveCount int                // Count of globally active monitors
}

// monitorChange represents a state transition for a monitor
type monitorChange struct {
	MonitorID uint32
	From      ntpdb.ServerScoresStatus
	To        ntpdb.ServerScoresStatus
	Reason    string
}
