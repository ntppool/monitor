package selector

//go:generate go tool github.com/dmarkham/enumer -type=candidateState

import (
	"go.ntppool.org/monitor/ntpdb"
)

// candidateState represents the recommended action for a monitor
type candidateState uint8

const (
	candidateUnknown candidateState = iota
	candidateIn                     // Should be promoted/kept active
	candidateOut                    // Should be demoted (gradual)
	candidateBlock                  // Should be removed immediately
	candidatePending                // Should remain as candidate
)

// IsOutOfOrder returns the "most out of order" of the currently active monitors.
// The second return parameter is the ID of the better monitor candidate,
// the first return parameter the ID to be replaced. The last parameter
// is false if no relevant replacement was found.
func (nsl newStatusList) IsOutOfOrder() (uint32, uint32, bool) {
	best := uint32(0)
	replace := uint32(0)

	for _, ns := range nsl {
		if ns.NewState != candidateIn {
			continue
		}
		switch ns.CurrentStatus {
		case ntpdb.ServerScoresStatusActive:
			// only replace if we found a replacement
			if best != 0 {
				replace = ns.MonitorID
			}

		case ntpdb.ServerScoresStatusTesting:
			if best == 0 {
				best = ns.MonitorID
			}

		}

	}

	if best == 0 || replace == 0 {
		return 0, 0, false
	}

	return best, replace, true
}

// determineState determines the appropriate candidate state based on global status and constraints
func (sl *Selector) determineState(
	monitor *monitorCandidate,
	violation *constraintViolation,
) candidateState {
	// STEP 1: Check global monitor status FIRST (respecting monitors.status)
	switch monitor.GlobalStatus {
	case ntpdb.MonitorsStatusPending:
		// Pending monitors should gradually phase out to allow clean transition
		if monitor.ServerStatus == ntpdb.ServerScoresStatusActive ||
			monitor.ServerStatus == ntpdb.ServerScoresStatusTesting {
			sl.log.Info("pending monitor will be gradually removed",
				"monitorID", monitor.ID,
				"globalStatus", monitor.GlobalStatus,
				"serverStatus", monitor.ServerStatus)
			return candidateOut // Gradual removal
		}
		// Pending monitors not assigned to this server should gradually phase out
		return candidateOut

	case ntpdb.MonitorsStatusPaused:
		// Paused monitors should stop all work immediately
		return candidateBlock

	case ntpdb.MonitorsStatusDeleted:
		// Deleted monitors must be removed
		return candidateBlock

	case ntpdb.MonitorsStatusTesting, ntpdb.MonitorsStatusActive:
		// Only these monitors can proceed to constraint checking
		// Continue to STEP 2

	default:
		// Unknown status = block
		sl.log.Warn("unknown global monitor status",
			"monitorID", monitor.ID,
			"status", monitor.GlobalStatus)
		return candidateBlock
	}

	// STEP 2: Check for state inconsistencies
	if sl.hasStateInconsistency(monitor) {
		sl.log.Warn("inconsistent monitor state detected",
			"monitorID", monitor.ID,
			"globalStatus", monitor.GlobalStatus,
			"serverStatus", monitor.ServerStatus)
		return candidateOut
	}

	// STEP 3: Apply constraint validation (only for globally active/testing monitors)
	if violation.Type != violationNone {
		if violation.IsGrandfathered {
			// Grandfathered violations get gradual removal
			return candidateOut
		}

		// New violations on unassigned monitors = block
		if monitor.ServerStatus == ntpdb.ServerScoresStatusNew {
			return candidateBlock
		}

		// New violations on assigned monitors = gradual removal
		return candidateOut
	}

	// STEP 4: Performance/health based logic
	if monitor.HasMetrics && !monitor.IsHealthy {
		return candidateOut
	}

	// STEP 5: Only globally active can be promoted to server-active
	if monitor.ServerStatus == ntpdb.ServerScoresStatusTesting &&
		monitor.GlobalStatus != ntpdb.MonitorsStatusActive {
		// Cannot promote to active unless globally active
		return candidatePending // Stay in testing
	}

	// Default: eligible for promotion/retention
	return candidateIn
}

// hasStateInconsistency checks for inconsistent global vs server states
func (sl *Selector) hasStateInconsistency(monitor *monitorCandidate) bool {
	// Monitor globally pending but server-active/testing is inconsistent
	if monitor.GlobalStatus == ntpdb.MonitorsStatusPending &&
		(monitor.ServerStatus == ntpdb.ServerScoresStatusActive ||
			monitor.ServerStatus == ntpdb.ServerScoresStatusTesting) {
		return true
	}

	// Monitor globally deleted but still assigned to server is inconsistent
	if monitor.GlobalStatus == ntpdb.MonitorsStatusDeleted &&
		(monitor.ServerStatus == ntpdb.ServerScoresStatusActive ||
			monitor.ServerStatus == ntpdb.ServerScoresStatusTesting ||
			monitor.ServerStatus == ntpdb.ServerScoresStatusCandidate) {
		return true
	}

	// Monitor globally paused but still assigned to server is inconsistent
	if monitor.GlobalStatus == ntpdb.MonitorsStatusPaused &&
		(monitor.ServerStatus == ntpdb.ServerScoresStatusActive ||
			monitor.ServerStatus == ntpdb.ServerScoresStatusTesting ||
			monitor.ServerStatus == ntpdb.ServerScoresStatusCandidate) {
		return true
	}

	return false
}
