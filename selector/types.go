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
	Type    constraintViolationType
	Since   time.Time
	Details string
}

// monitorCandidate represents a monitor being evaluated for a server
type monitorCandidate struct {
	ID                       uint32
	IDToken                  string // Unique identifier for metrics
	TLSName                  string // Human-readable name for metrics
	AccountID                *uint32
	IP                       string
	GlobalStatus             ntpdb.MonitorsStatus
	ServerStatus             ntpdb.ServerScoresStatus
	HasMetrics               bool
	IsHealthy                bool
	RTT                      float64
	Priority                 float64 // Database-calculated monitor priority (RTT + step accuracy)
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
	currentViolation *constraintViolation // Constraints for current state
	recommendedState candidateState       // Legacy field for backward compatibility
}
