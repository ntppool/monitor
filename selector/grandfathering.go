package selector

import (
	"go.ntppool.org/monitor/ntpdb"
)

// isGrandfathered determines if a constraint violation should be grandfathered
// Grandfathering allows existing assignments that violate new constraints to remain
// but be marked for gradual removal (candidateOut instead of candidateBlock)
func (sl *Selector) isGrandfathered(
	monitor *monitorCandidate,
	server *serverInfo,
	violation *constraintViolation,
) bool {
	// Only grandfather existing active/testing assignments
	if monitor.ServerStatus != ntpdb.ServerScoresStatusActive &&
		monitor.ServerStatus != ntpdb.ServerScoresStatusTesting {
		return false
	}

	// Network constraints are hardcoded, so they can't be grandfathered
	// (they were always enforced at the same level)
	if violation.Type == violationNetworkSameSubnet {
		return false
	}

	// Account limit violations on existing assignments are grandfathered
	// This allows for gradual adjustment when account limits are reduced
	if violation.Type == violationLimit {
		// Check if this is a long-standing violation
		if monitor.ConstraintViolationType != nil &&
			*monitor.ConstraintViolationType == string(violationLimit) &&
			monitor.ConstraintViolationSince != nil {
			// Already tracked violation, definitely grandfathered
			return true
		}
		// New limit violation on existing assignment
		return true
	}

	// Same account violations can't be grandfathered
	// (this is a hard constraint that should never have been allowed)
	if violation.Type == violationAccount {
		return false
	}

	// Network diversity violations are grandfathered for existing assignments
	// This is a new constraint, so existing assignments that violate it should be
	// gradually removed rather than immediately blocked
	if violation.Type == violationNetworkDiversity {
		return true
	}

	return false
}

// prioritizeGrandfatheredRemovals determines which grandfathered monitors to remove first
// when we need to free up slots. Returns monitors sorted by removal priority.
func (sl *Selector) prioritizeGrandfatheredRemovals(monitors []evaluatedMonitor) []evaluatedMonitor {
	// Sort by multiple criteria:
	// 1. Performance (unhealthy first)
	// 2. Assignment duration (newer assignments first)
	// 3. RTT (higher latency first)

	// For now, just return as-is
	// TODO: Implement sorting logic when we have the full monitor data structure
	return monitors
}
