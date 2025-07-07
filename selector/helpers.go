package selector

import (
	"go.ntppool.org/monitor/ntpdb"
)

// promotionRequest represents all data needed for a monitor promotion attempt
type promotionRequest struct {
	monitor           *monitorCandidate
	server            *serverInfo
	workingLimits     map[uint32]*accountLimit
	assignedMonitors  []ntpdb.GetMonitorPriorityRow
	emergencyOverride bool
	fromStatus        ntpdb.ServerScoresStatus
	toStatus          ntpdb.ServerScoresStatus
	baseReason        string
	emergencyReason   string
}

// promotionResult represents the outcome of a promotion attempt
type promotionResult struct {
	change           *statusChange
	success          bool
	activeIncrement  int // +1 if promoting to active, -1 if demoting from active, 0 otherwise
	testingIncrement int // +1 if promoting to testing, -1 if demoting from testing, 0 otherwise
}

// attemptPromotion attempts to promote a monitor and returns the result with count changes
func (sl *Selector) attemptPromotion(req promotionRequest) promotionResult {
	var canPromote bool

	switch req.toStatus {
	case ntpdb.ServerScoresStatusActive:
		canPromote = sl.canPromoteToActive(req.monitor, req.server, req.workingLimits, req.assignedMonitors, req.emergencyOverride)
	case ntpdb.ServerScoresStatusTesting:
		canPromote = sl.canPromoteToTesting(req.monitor, req.server, req.workingLimits, req.assignedMonitors, req.emergencyOverride)
	default:
		return promotionResult{success: false}
	}

	if !canPromote {
		return promotionResult{success: false}
	}

	reason := req.baseReason
	if req.emergencyOverride {
		reason = req.emergencyReason
	}

	change := statusChange{
		monitorID:  req.monitor.ID,
		fromStatus: req.fromStatus,
		toStatus:   req.toStatus,
		reason:     reason,
	}

	// Update working account limits
	sl.updateAccountLimitsForPromotion(req.workingLimits, req.monitor, req.fromStatus, req.toStatus)

	// Calculate count changes
	var activeIncrement, testingIncrement int

	// Handle changes from active status
	if req.fromStatus == ntpdb.ServerScoresStatusActive {
		activeIncrement = -1
	}
	// Handle changes to active status
	if req.toStatus == ntpdb.ServerScoresStatusActive {
		activeIncrement = 1
	}

	// Handle changes from testing status
	if req.fromStatus == ntpdb.ServerScoresStatusTesting {
		testingIncrement = -1
	}
	// Handle changes to testing status
	if req.toStatus == ntpdb.ServerScoresStatusTesting {
		testingIncrement = 1
	}

	return promotionResult{
		change:           &change,
		success:          true,
		activeIncrement:  activeIncrement,
		testingIncrement: testingIncrement,
	}
}

// getEmergencyReason returns the appropriate emergency reason text
func getEmergencyReason(baseReason string, toStatus ntpdb.ServerScoresStatus, isBootstrap bool) string {
	prefix := "emergency promotion"
	if isBootstrap {
		prefix = "bootstrap emergency promotion"
	}

	switch toStatus {
	case ntpdb.ServerScoresStatusActive:
		return prefix + ": zero active monitors"
	case ntpdb.ServerScoresStatusTesting:
		return prefix + " to testing: zero active monitors"
	default:
		return baseReason
	}
}

// filterMonitorsByGlobalStatus filters evaluated monitors by their global status
func filterMonitorsByGlobalStatus(monitors []evaluatedMonitor, status ntpdb.MonitorsStatus) []evaluatedMonitor {
	var filtered []evaluatedMonitor
	for _, em := range monitors {
		if em.monitor.GlobalStatus == status {
			filtered = append(filtered, em)
		}
	}
	return filtered
}

// filterBootstrapCandidates separates candidate monitors into healthy and other categories
// Only returns monitors that are eligible for bootstrap (globally active or testing)
func filterBootstrapCandidates(monitors []evaluatedMonitor) (healthy, other []evaluatedMonitor) {
	for _, em := range monitors {
		isEligible := em.monitor.GlobalStatus == ntpdb.MonitorsStatusActive ||
			em.monitor.GlobalStatus == ntpdb.MonitorsStatusTesting

		if !isEligible {
			continue
		}

		if em.monitor.IsHealthy {
			healthy = append(healthy, em)
		} else {
			other = append(other, em)
		}
	}
	return
}

// candidateGroup represents a group of candidate monitors with metadata
type candidateGroup struct {
	monitors []evaluatedMonitor
	name     string
}

// createCandidateGroups creates ordered groups of candidate monitors for promotion
func createCandidateGroups(candidateMonitors []evaluatedMonitor) []candidateGroup {
	return []candidateGroup{
		{filterMonitorsByGlobalStatus(candidateMonitors, ntpdb.MonitorsStatusActive), "active"},
		{filterMonitorsByGlobalStatus(candidateMonitors, ntpdb.MonitorsStatusTesting), "testing"},
	}
}

// candidateOutperformsTestingMonitor compares performance between a candidate and testing monitor
// Returns true if the candidate is better performing than the testing monitor
func (sl *Selector) candidateOutperformsTestingMonitor(candidate, testingMonitor evaluatedMonitor) bool {
	// First compare by health - healthy monitors are always better than unhealthy
	if candidate.monitor.IsHealthy && !testingMonitor.monitor.IsHealthy {
		return true
	}
	if !candidate.monitor.IsHealthy && testingMonitor.monitor.IsHealthy {
		return false
	}

	// If health is equal, compare by RTT (lower is better)
	// Note: This is a simplified comparison. The full priority calculation
	// happens in the database and monitors are pre-sorted by performance.
	// Since we don't have access to the calculated priority here, we use RTT
	// as the primary performance indicator.
	return candidate.monitor.RTT < testingMonitor.monitor.RTT
}

// copyAccountLimits creates a deep copy of account limits map for testing scenarios
func (sl *Selector) copyAccountLimits(original map[uint32]*accountLimit) map[uint32]*accountLimit {
	copy := make(map[uint32]*accountLimit)
	for k, v := range original {
		copy[k] = &accountLimit{
			AccountID:    v.AccountID,
			MaxPerServer: v.MaxPerServer,
			ActiveCount:  v.ActiveCount,
			TestingCount: v.TestingCount,
		}
	}
	return copy
}
