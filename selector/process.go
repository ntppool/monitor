package selector

import (
	"context"
	"fmt"

	"go.ntppool.org/monitor/ntpdb"
)

// Constants for monitor selection
const (
	targetActiveMonitors       = 7 // Target number of active monitors per server
	baseTestingTarget          = 5 // Base number of testing monitors
	minGloballyActiveInTesting = 4 // Minimum globally active monitors in testing pool
	bootStrapModeLimit         = 4 // If active monitors <= this, add new ones faster
)

// statusChange represents a planned status transition
type statusChange struct {
	monitorID  uint32
	fromStatus ntpdb.ServerScoresStatus
	toStatus   ntpdb.ServerScoresStatus
	reason     string
}

// changeLimits defines separate limits for different types of status changes
type changeLimits struct {
	activeRemovals  int // active → testing demotions
	testingRemovals int // testing → candidate demotions
	promotions      int // testing → active, candidate → testing
}

// loadServerInfo loads server details including IP and account
func (sl *Selector) loadServerInfo(
	ctx context.Context,
	db *ntpdb.Queries,
	serverID uint32,
) (*serverInfo, error) {
	server, err := db.GetServer(ctx, serverID)
	if err != nil {
		return nil, fmt.Errorf("failed to get server: %w", err)
	}

	var accountID *uint32
	if server.AccountID.Valid {
		id := uint32(server.AccountID.Int32)
		accountID = &id
	}

	return &serverInfo{
		ID:        serverID,
		IP:        server.Ip,
		AccountID: accountID,
		IPVersion: string(server.IpVersion),
	}, nil
}

// applySelectionRules determines what status changes should be made
func (sl *Selector) applySelectionRules(
	ctx context.Context,
	evaluatedMonitors []evaluatedMonitor,
	server *serverInfo,
	accountLimits map[uint32]*accountLimit,
	assignedMonitors []ntpdb.GetMonitorPriorityRow,
) []statusChange {
	changes := make([]statusChange, 0)

	// Categorize monitors by current status
	var (
		activeMonitors    []evaluatedMonitor
		testingMonitors   []evaluatedMonitor
		candidateMonitors []evaluatedMonitor
	)

	for _, em := range evaluatedMonitors {
		switch em.monitor.ServerStatus {
		case ntpdb.ServerScoresStatusActive:
			activeMonitors = append(activeMonitors, em)
		case ntpdb.ServerScoresStatusTesting:
			testingMonitors = append(testingMonitors, em)
		case ntpdb.ServerScoresStatusCandidate:
			candidateMonitors = append(candidateMonitors, em)
		}
	}

	// Count healthy monitors and blocked monitors (for change limit calculation)
	healthyActive := sl.countHealthy(activeMonitors)
	healthyTesting := sl.countHealthy(testingMonitors)
	blockedMonitors := sl.countBlocked(evaluatedMonitors)

	// Calculate per-status change limits
	targetNumber := targetActiveMonitors // 7 from constants
	currentActiveMonitors := len(activeMonitors)

	limits := calculateChangeLimits(currentActiveMonitors, blockedMonitors)

	// Legacy allowedChanges for backward compatibility in logging
	allowedChanges := limits.activeRemovals

	// Emergency safeguards
	if targetNumber > len(evaluatedMonitors) && healthyActive < currentActiveMonitors {
		sl.log.Warn("emergency: not enough healthy monitors available",
			"targetNumber", targetNumber,
			"totalMonitors", len(evaluatedMonitors),
			"healthyActive", healthyActive,
			"currentActive", currentActiveMonitors)
		return []statusChange{} // No changes in emergency situation
	}

	maxRemovals := allowedChanges
	// Safety: don't remove monitors if we're at/below target and don't have enough healthy
	// BUT: always allow demotions from testing to candidate to clean up constraint violations
	if currentActiveMonitors <= targetNumber && healthyActive < targetNumber {
		// Only block removals from active status, not demotions from testing to candidate
		// This allows cleanup of testing monitors with constraint violations
		if currentActiveMonitors > 0 {
			maxRemovals = 0
		}
		// If zero active monitors, allow demotions to clean up testing violations
		// The emergency override logic will handle promotions separately
	}

	// Emergency: never remove all active monitors
	if maxRemovals >= currentActiveMonitors && currentActiveMonitors > 0 {
		maxRemovals = currentActiveMonitors - 1
		sl.log.Warn("emergency: limiting removals to prevent zero active monitors",
			"originalMaxRemovals", allowedChanges,
			"adjustedMaxRemovals", maxRemovals,
			"currentActive", currentActiveMonitors)
	}

	sl.log.Debug("current monitor counts and limits",
		"active", len(activeMonitors),
		"healthyActive", healthyActive,
		"testing", len(testingMonitors),
		"healthyTesting", healthyTesting,
		"candidates", len(candidateMonitors),
		"blocked", blockedMonitors,
		"allowedChanges", allowedChanges,
		"maxRemovals", maxRemovals)

	// Rule 1: Remove monitors that should be blocked immediately
	for _, em := range activeMonitors {
		if em.recommendedState == candidateBlock {
			changes = append(changes, statusChange{
				monitorID:  em.monitor.ID,
				fromStatus: ntpdb.ServerScoresStatusActive,
				toStatus:   ntpdb.ServerScoresStatusCandidate,
				reason:     "blocked by constraints or global status",
			})
		}
	}

	for _, em := range testingMonitors {
		if em.recommendedState == candidateBlock {
			changes = append(changes, statusChange{
				monitorID:  em.monitor.ID,
				fromStatus: ntpdb.ServerScoresStatusTesting,
				toStatus:   ntpdb.ServerScoresStatusCandidate,
				reason:     "blocked by constraints or global status",
			})
		}
	}

	// Rule 2: Gradual removal of candidateOut monitors (with limits)

	// First remove active monitors (demote to testing, not new) - use activeRemovals limit
	// Iterate backwards to demote worst performers first (bottom-up)
	activeRemovalsRemaining := limits.activeRemovals
	for i := len(activeMonitors) - 1; i >= 0; i-- {
		if activeRemovalsRemaining <= 0 {
			break
		}
		em := activeMonitors[i]
		if em.recommendedState == candidateOut {
			changes = append(changes, statusChange{
				monitorID:  em.monitor.ID,
				fromStatus: ntpdb.ServerScoresStatusActive,
				toStatus:   ntpdb.ServerScoresStatusTesting,
				reason:     "gradual removal (health or constraints)",
			})
			activeRemovalsRemaining--
		}
	}

	// Then remove testing monitors (demote to candidate, not new)
	// Use separate limit for testing demotions
	testingRemovalsRemaining := limits.testingRemovals

	// When zero active monitors, allow more aggressive cleanup of testing violations
	if currentActiveMonitors == 0 {
		// Count testing monitors with constraint violations
		testingViolationCount := 0
		for _, em := range testingMonitors {
			if em.recommendedState == candidateOut {
				testingViolationCount++
			}
		}
		// Allow demoting all testing monitors with violations, but respect testing removal limits
		testingRemovalsRemaining = min(testingViolationCount, limits.testingRemovals)
	}

	// Iterate backwards to demote worst performers first (bottom-up)
	for i := len(testingMonitors) - 1; i >= 0; i-- {
		if testingRemovalsRemaining <= 0 {
			break
		}
		em := testingMonitors[i]
		if em.recommendedState == candidateOut {
			changes = append(changes, statusChange{
				monitorID:  em.monitor.ID,
				fromStatus: ntpdb.ServerScoresStatusTesting,
				toStatus:   ntpdb.ServerScoresStatusCandidate,
				reason:     "gradual removal (health or constraints)",
			})
			testingRemovalsRemaining--
		}
	}

	// Rule 2.5: Demote excess testing monitors based on dynamic target
	// Calculate dynamic testing target based on active monitor gap
	activeGap := max(0, targetActiveMonitors-len(activeMonitors))
	dynamicTestingTarget := baseTestingTarget + activeGap

	testingCount := len(testingMonitors)
	if testingCount > dynamicTestingTarget {
		excessTesting := testingCount - dynamicTestingTarget

		// Count how many testing demotions have already been made
		testingDemotionsSoFar := 0
		for _, change := range changes {
			if change.fromStatus == ntpdb.ServerScoresStatusTesting && change.toStatus == ntpdb.ServerScoresStatusCandidate {
				testingDemotionsSoFar++
			}
		}

		dynamicTestingRemovalsRemaining := max(0, limits.testingRemovals-testingDemotionsSoFar)
		demotionsNeeded := min(excessTesting, dynamicTestingRemovalsRemaining)

		if demotionsNeeded > 0 {
			sl.log.Debug("demoting excess testing monitors",
				"currentTesting", testingCount,
				"dynamicTarget", dynamicTestingTarget,
				"activeGap", activeGap,
				"excessTesting", excessTesting,
				"demotionsNeeded", demotionsNeeded)
		}

		// Demote worst-performing healthy testing monitors
		demoted := 0
		for i := len(testingMonitors) - 1; i >= 0 && demoted < demotionsNeeded; i-- {
			em := testingMonitors[i]
			// Skip if already marked for demotion or has violations
			if em.recommendedState != candidateOut && em.currentViolation.Type == violationNone {
				changes = append(changes, statusChange{
					monitorID:  em.monitor.ID,
					fromStatus: ntpdb.ServerScoresStatusTesting,
					toStatus:   ntpdb.ServerScoresStatusCandidate,
					reason:     fmt.Sprintf("excess testing monitors (%d > target %d)", testingCount, dynamicTestingTarget),
				})
				demoted++
				testingCount--
			}
		}
	}

	// Create a working copy of account limits for iterative constraint checking
	// This will be updated as we make promotion decisions
	workingAccountLimits := make(map[uint32]*accountLimit)
	for k, v := range accountLimits {
		workingAccountLimits[k] = &accountLimit{
			AccountID:    v.AccountID,
			MaxPerServer: v.MaxPerServer,
			ActiveCount:  v.ActiveCount,
			TestingCount: v.TestingCount,
		}
	}

	// Rule 3: Promote from testing to active (iterative constraint checking)
	changesRemaining := limits.promotions
	toAdd := targetNumber - currentActiveMonitors

	// Account for demotions (active->testing increases toAdd)
	for _, change := range changes {
		if change.fromStatus == ntpdb.ServerScoresStatusActive && change.toStatus == ntpdb.ServerScoresStatusTesting {
			toAdd++
		}
	}

	if toAdd > 0 && changesRemaining > 0 {
		promotionsNeeded := min(toAdd, changesRemaining)
		promoted := 0

		// Emergency override: if zero active monitors, ignore constraint violations
		emergencyOverride := (currentActiveMonitors == 0)
		if emergencyOverride {
			sl.log.WarnContext(ctx, "emergency override: zero active monitors, ignoring constraints for promotion",
				"testingMonitors", len(testingMonitors))
		}

		// Iteratively check each testing monitor for promotion
		for _, em := range testingMonitors {
			if promoted >= promotionsNeeded {
				break
			}

			// Check if this monitor can be promoted to active using current state
			canPromote := sl.canPromoteToActive(&em.monitor, server, workingAccountLimits, assignedMonitors, emergencyOverride)
			reason := "promotion to active"
			if emergencyOverride && canPromote {
				reason = "emergency promotion: zero active monitors"
			}

			if canPromote {
				changes = append(changes, statusChange{
					monitorID:  em.monitor.ID,
					fromStatus: ntpdb.ServerScoresStatusTesting,
					toStatus:   ntpdb.ServerScoresStatusActive,
					reason:     reason,
				})

				// Update working account limits for next iteration
				sl.updateAccountLimitsForPromotion(workingAccountLimits, &em.monitor,
					ntpdb.ServerScoresStatusTesting, ntpdb.ServerScoresStatusActive)

				promoted++
			}
		}
	}

	// Rule 5: Promote candidates to testing (iterative constraint checking)
	changesRemaining = limits.promotions
	if changesRemaining > 0 && len(candidateMonitors) > 0 {
		promotionsNeeded := min(changesRemaining, 2) // Limit candidate promotions
		promoted := 0

		// Note: workingAccountLimits are already updated from testing→active promotions above

		// Prefer globally active monitors - check constraints dynamically
		for _, em := range candidateMonitors {
			if promoted >= promotionsNeeded {
				break
			}
			if em.monitor.GlobalStatus == ntpdb.MonitorsStatusActive {
				// Check if can promote to testing using current constraint state
				if sl.canPromoteToTesting(&em.monitor, server, workingAccountLimits, assignedMonitors) {
					changes = append(changes, statusChange{
						monitorID:  em.monitor.ID,
						fromStatus: ntpdb.ServerScoresStatusCandidate,
						toStatus:   ntpdb.ServerScoresStatusTesting,
						reason:     "candidate to testing",
					})

					// Update working account limits for next iteration
					sl.updateAccountLimitsForPromotion(workingAccountLimits, &em.monitor,
						ntpdb.ServerScoresStatusCandidate, ntpdb.ServerScoresStatusTesting)

					promoted++
				}
			}
		}

		// Fill remaining with globally testing monitors if needed
		for _, em := range candidateMonitors {
			if promoted >= promotionsNeeded {
				break
			}
			if em.monitor.GlobalStatus == ntpdb.MonitorsStatusTesting {
				// Check if can promote to testing using current constraint state
				if sl.canPromoteToTesting(&em.monitor, server, workingAccountLimits, assignedMonitors) {
					changes = append(changes, statusChange{
						monitorID:  em.monitor.ID,
						fromStatus: ntpdb.ServerScoresStatusCandidate,
						toStatus:   ntpdb.ServerScoresStatusTesting,
						reason:     "candidate to testing",
					})

					// Update working account limits for next iteration
					sl.updateAccountLimitsForPromotion(workingAccountLimits, &em.monitor,
						ntpdb.ServerScoresStatusCandidate, ntpdb.ServerScoresStatusTesting)

					promoted++
				}
			}
		}
	}

	// Rule 6: Bootstrap case - if no testing monitors exist, promote candidates to reach target
	if len(testingMonitors) == 0 && len(candidateMonitors) > 0 {
		// In bootstrap scenario, we can promote up to baseTestingTarget at once
		bootstrapPromotions := baseTestingTarget
		promoted := 0

		sl.log.Info("bootstrap: no testing monitors, promoting candidates to start monitoring",
			"candidatesAvailable", len(candidateMonitors),
			"baseTestingTarget", baseTestingTarget,
			"bootstrapPromotions", bootstrapPromotions)

		// Sort candidates by health first, then by global status
		healthyCandidates := make([]evaluatedMonitor, 0)
		otherCandidates := make([]evaluatedMonitor, 0)

		for _, em := range candidateMonitors {
			if em.monitor.IsHealthy && (em.monitor.GlobalStatus == ntpdb.MonitorsStatusActive || em.monitor.GlobalStatus == ntpdb.MonitorsStatusTesting) {
				healthyCandidates = append(healthyCandidates, em)
			} else if em.monitor.GlobalStatus == ntpdb.MonitorsStatusActive || em.monitor.GlobalStatus == ntpdb.MonitorsStatusTesting {
				otherCandidates = append(otherCandidates, em)
			}
		}

		// Promote healthy candidates first
		for _, em := range healthyCandidates {
			if promoted >= bootstrapPromotions {
				break
			}
			// Bootstrap: respect basic constraints during promotion
			if sl.canPromoteToTesting(&em.monitor, server, workingAccountLimits, assignedMonitors) {
				changes = append(changes, statusChange{
					monitorID:  em.monitor.ID,
					fromStatus: ntpdb.ServerScoresStatusCandidate,
					toStatus:   ntpdb.ServerScoresStatusTesting,
					reason:     "bootstrap: promoting healthy candidate",
				})
				// Update working account limits
				sl.updateAccountLimitsForPromotion(workingAccountLimits, &em.monitor,
					ntpdb.ServerScoresStatusCandidate, ntpdb.ServerScoresStatusTesting)
				promoted++
			}
		}

		// If still need more, promote other candidates
		for _, em := range otherCandidates {
			if promoted >= bootstrapPromotions {
				break
			}
			// Bootstrap: respect basic constraints during promotion
			if sl.canPromoteToTesting(&em.monitor, server, workingAccountLimits, assignedMonitors) {
				changes = append(changes, statusChange{
					monitorID:  em.monitor.ID,
					fromStatus: ntpdb.ServerScoresStatusCandidate,
					toStatus:   ntpdb.ServerScoresStatusTesting,
					reason:     "bootstrap: promoting candidate",
				})
				// Update working account limits
				sl.updateAccountLimitsForPromotion(workingAccountLimits, &em.monitor,
					ntpdb.ServerScoresStatusCandidate, ntpdb.ServerScoresStatusTesting)
				promoted++
			}
		}

		if promoted == 0 && len(candidateMonitors) > 0 {
			sl.log.Warn("bootstrap: unable to promote any candidates due to constraints",
				"candidatesAvailable", len(candidateMonitors))
		}
	}

	// Rule 7: Handle out-of-order situations (disabled for now to respect change limits)
	// TODO: Implement out-of-order logic that respects allowedChanges limits

	sl.log.Debug("planned changes", "totalChanges", len(changes), "allowedChanges", allowedChanges)

	return changes
}

// calculateChangeLimits determines separate limits for different status change types
func calculateChangeLimits(currentActiveMonitors, blockedMonitors int) changeLimits {
	// Per-status limits can be more generous since they don't compete with each other
	base := 2 // Increased from 1 to allow more efficient processing
	if blockedMonitors > 1 {
		base = 3 // Increased from 2
	}
	if currentActiveMonitors == 0 {
		base = 4 // bootstrap mode (unchanged)
	}

	return changeLimits{
		activeRemovals:  base,
		testingRemovals: base,
		promotions:      base,
	}
}

// applyStatusChange executes a single status change
func (sl *Selector) applyStatusChange(
	ctx context.Context,
	db *ntpdb.Queries,
	serverID uint32,
	change statusChange,
	monitor *monitorCandidate,
) error {
	// All transitions now involve existing server_scores entries
	err := db.UpdateServerScoreStatus(ctx, ntpdb.UpdateServerScoreStatusParams{
		MonitorID: change.monitorID,
		ServerID:  serverID,
		Status:    change.toStatus,
	})
	if err != nil {
		return fmt.Errorf("failed to update server score status: %w", err)
	}

	// Track successful status change in metrics
	if sl.metrics != nil {
		sl.metrics.TrackStatusChange(monitor, change.fromStatus, change.toStatus, serverID, change.reason)
	}

	return nil
}

// Helper methods for selection logic

func (sl *Selector) countHealthy(monitors []evaluatedMonitor) int {
	count := 0
	for _, em := range monitors {
		if em.monitor.IsHealthy && em.recommendedState == candidateIn {
			count++
		}
	}
	return count
}

func (sl *Selector) countGloballyActive(monitors []evaluatedMonitor) int {
	count := 0
	for _, em := range monitors {
		if em.monitor.GlobalStatus == ntpdb.MonitorsStatusActive {
			count++
		}
	}
	return count
}

func (sl *Selector) countBlocked(monitors []evaluatedMonitor) int {
	count := 0
	for _, em := range monitors {
		if em.recommendedState == candidateBlock {
			count++
		}
	}
	return count
}

// min returns the smaller of two integers
func min(a, b int) int {
	if a < b {
		return a
	}
	return b
}

func (sl *Selector) selectMonitorsForRemoval(
	active []evaluatedMonitor,
	testing []evaluatedMonitor,
) []evaluatedMonitor {
	var toRemove []evaluatedMonitor

	// Select from active monitors marked for removal
	for _, em := range active {
		if em.recommendedState == candidateOut {
			toRemove = append(toRemove, em)
		}
	}

	// Select from testing monitors marked for removal
	for _, em := range testing {
		if em.recommendedState == candidateOut {
			toRemove = append(toRemove, em)
		}
	}

	// TODO: Sort by priority (oldest grandfathered violations first)

	return toRemove
}

func (sl *Selector) selectMonitorsForPromotion(
	candidates []evaluatedMonitor,
	count int,
) []evaluatedMonitor {
	var eligible []evaluatedMonitor

	// Filter to only eligible monitors
	for _, em := range candidates {
		if em.recommendedState == candidateIn &&
			em.monitor.GlobalStatus == ntpdb.MonitorsStatusActive &&
			em.monitor.IsHealthy {
			eligible = append(eligible, em)
		}
	}

	// TODO: Sort by quality metrics (RTT, history length, etc.)

	// Return up to count monitors
	if len(eligible) <= count {
		return eligible
	}
	return eligible[:count]
}

func (sl *Selector) selectCandidatesForTesting(
	candidates []evaluatedMonitor,
	count int,
) []evaluatedMonitor {
	var eligible []evaluatedMonitor

	// Prefer globally active monitors
	for _, em := range candidates {
		if em.recommendedState == candidateIn && em.monitor.GlobalStatus == ntpdb.MonitorsStatusActive {
			eligible = append(eligible, em)
		}
	}

	// If not enough, add globally testing monitors
	if len(eligible) < count {
		for _, em := range candidates {
			if em.recommendedState == candidateIn && em.monitor.GlobalStatus == ntpdb.MonitorsStatusTesting {
				eligible = append(eligible, em)
			}
		}
	}

	// Return up to count monitors
	if len(eligible) <= count {
		return eligible
	}
	return eligible[:count]
}

func (sl *Selector) calculateNeededCandidates(active, testing, candidates int) int {
	// We want a buffer of candidates ready to be promoted
	// Target: enough to replace both active and testing pools (using base testing target)
	targetCandidates := targetActiveMonitors + baseTestingTarget
	current := candidates

	if current < targetCandidates {
		return targetCandidates - current
	}
	return 0
}

func (sl *Selector) handleOutOfOrder(
	active []evaluatedMonitor,
	testing []evaluatedMonitor,
	changes []statusChange,
) []statusChange {
	// Build newStatusList to check for out-of-order situations
	nsl := newStatusList{}

	// Add current statuses (accounting for planned changes)
	statusMap := make(map[uint32]ntpdb.ServerScoresStatus)
	for _, em := range active {
		statusMap[em.monitor.ID] = em.monitor.ServerStatus
	}
	for _, em := range testing {
		statusMap[em.monitor.ID] = em.monitor.ServerStatus
	}

	// Apply planned changes to map
	for _, change := range changes {
		statusMap[change.monitorID] = change.toStatus
	}

	// Build list for IsOutOfOrder check
	for _, em := range append(active, testing...) {
		nsl = append(nsl, newStatus{
			MonitorID:     em.monitor.ID,
			CurrentStatus: statusMap[em.monitor.ID],
			NewState:      em.recommendedState,
		})
	}

	// Check if out of order
	if bestID, replaceID, found := nsl.IsOutOfOrder(); found {
		// Add swap changes
		changes = append(changes, statusChange{
			monitorID:  bestID,
			fromStatus: ntpdb.ServerScoresStatusTesting,
			toStatus:   ntpdb.ServerScoresStatusActive,
			reason:     "out-of-order promotion",
		})
		changes = append(changes, statusChange{
			monitorID:  replaceID,
			fromStatus: ntpdb.ServerScoresStatusActive,
			toStatus:   ntpdb.ServerScoresStatusTesting,
			reason:     "out-of-order demotion",
		})
	}

	return changes
}
