package selector

import (
	"context"
	"fmt"
	"log/slog"

	"go.ntppool.org/monitor/ntpdb"
)

// Constants for monitor selection
const (
	targetActiveMonitors       = 7  // Target number of active monitors per server
	baseTestingTarget          = 5  // Base number of testing monitors
	minGloballyActiveInTesting = 4  // Minimum globally active monitors in testing pool
	bootStrapModeLimit         = 4  // If active monitors <= this, add new ones faster
	minCountForTesting         = 9  // Minimum data points required for candidate->testing promotion
	minCountForActive          = 32 // Minimum data points required for testing->active promotion
)

// replacementType defines the type of performance-based replacement
type replacementType int8

const (
	candidateToTesting replacementType = iota // Rule 5: candidates replace testing monitors
	testingToActive                           // Rule 3: testing monitors replace active monitors
)

// statusChange represents a planned status transition
type statusChange struct {
	monitorID  uint32
	fromStatus ntpdb.ServerScoresStatus
	toStatus   ntpdb.ServerScoresStatus
	reason     string
}

// monitorRemovalConfig configures the gradual removal process for a set of monitors
type monitorRemovalConfig struct {
	monitors      []evaluatedMonitor
	fromStatus    ntpdb.ServerScoresStatus
	toStatus      ntpdb.ServerScoresStatus
	currentCount  int
	safeThreshold int
	removalLimit  int
}

// safetyCheckResult contains analysis of monitors that need removal
type safetyCheckResult struct {
	constraintViolations int
	performanceIssues    int
	totalIssues          int
}

// changeLimits defines separate limits for different types of status changes
type changeLimits struct {
	activeRemovals  int // active → testing demotions
	testingRemovals int // testing → candidate demotions
	promotions      int // testing → active, candidate → testing
}

// selectionContext bundles parameters needed for rule execution
type selectionContext struct {
	evaluatedMonitors []evaluatedMonitor
	server            *serverInfo
	accountLimits     map[uint32]*accountLimit
	assignedMonitors  []ntpdb.GetMonitorPriorityRow
	limits            changeLimits
	targetNumber      int
	emergencyOverride bool
}

// workingState tracks counts and limits during selection processing
type workingState struct {
	activeCount    int
	testingCount   int
	healthyActive  int
	healthyTesting int
	blockedCount   int
	maxRemovals    int
}

// ruleResult represents the outcome of applying a selection rule
type ruleResult struct {
	changes      []statusChange
	activeCount  int // updated active count after changes
	testingCount int // updated testing count after changes
}

// loadServerInfo loads server details including IP and account
func (sl *Selector) loadServerInfo(
	ctx context.Context,
	db ntpdb.QuerierTx,
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

// Rule 1 (Immediate Blocking): Remove monitors that should be blocked immediately
func (sl *Selector) applyRule1ImmediateBlocking(
	ctx context.Context,
	selCtx selectionContext,
	activeMonitors []evaluatedMonitor,
	testingMonitors []evaluatedMonitor,
) []statusChange {
	var changes []statusChange

	// Block active monitors marked for immediate blocking
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

	// Block testing monitors marked for immediate blocking
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

	return changes
}

// analyzeSafetyViolations analyzes monitors to categorize removal reasons
func (sl *Selector) analyzeSafetyViolations(monitors []evaluatedMonitor) safetyCheckResult {
	result := safetyCheckResult{}
	for _, em := range monitors {
		if em.recommendedState == candidateOut {
			if em.currentViolation != nil && em.currentViolation.Type != violationNone {
				result.constraintViolations++
			} else {
				result.performanceIssues++
			}
		}
	}
	result.totalIssues = result.constraintViolations + result.performanceIssues
	return result
}

// processRemovals handles gradual removal for monitors
func (sl *Selector) processRemovals(
	ctx context.Context,
	monitors []evaluatedMonitor,
	fromStatus, toStatus ntpdb.ServerScoresStatus,
	currentCount, safeThreshold, removalLimit, activeCount int,
) []statusChange {
	safety := sl.analyzeSafetyViolations(monitors)
	if safety.totalIssues == 0 {
		return nil
	}

	countAfterConstraints := currentCount - safety.constraintViolations
	skipPerformance := countAfterConstraints <= safeThreshold && safety.performanceIssues > 0

	// Calculate and apply removals
	removalsAllowed := sl.calculateRemovalLimit(
		ctx, fromStatus, currentCount, safeThreshold, removalLimit,
		safety, skipPerformance, activeCount,
	)
	return sl.applyRemovals(monitors, fromStatus, toStatus, safety, skipPerformance, removalsAllowed)
}

// calculateRemovalLimit determines safe removal count
func (sl *Selector) calculateRemovalLimit(
	ctx context.Context,
	fromStatus ntpdb.ServerScoresStatus,
	currentCount, safeThreshold, removalLimit int,
	safety safetyCheckResult,
	skipPerformance bool,
	activeCount int,
) int {
	// Special: zero active allows aggressive testing cleanup
	if fromStatus == ntpdb.ServerScoresStatusTesting && activeCount == 0 && currentCount > safeThreshold {
		return min(min(safety.totalIssues, removalLimit), currentCount-safeThreshold)
	}

	// Skip if at/below threshold with only performance issues
	if (currentCount <= safeThreshold && safety.constraintViolations == 0 && safety.totalIssues > 0) ||
		(skipPerformance && safety.constraintViolations == 0) {
		statusName := "active"
		if fromStatus == ntpdb.ServerScoresStatusTesting {
			statusName = "testing"
		}
		sl.log.WarnContext(ctx, fmt.Sprintf("skipping %s monitor removal due to safety threshold", statusName),
			"current", currentCount, "safeThreshold", safeThreshold,
			"performanceRemovals", safety.performanceIssues,
			"constraintViolations", safety.constraintViolations)
		return 0
	}

	// Calculate based on constraints and performance
	if safety.constraintViolations > 0 && (currentCount <= safeThreshold || skipPerformance) {
		return min(removalLimit, safety.constraintViolations)
	}

	maxSafeRemovals := currentCount - safeThreshold
	if safety.constraintViolations > 0 || !skipPerformance {
		return min(removalLimit, maxSafeRemovals)
	}
	return 0
}

// applyRemovals applies removal logic to monitors
func (sl *Selector) applyRemovals(
	monitors []evaluatedMonitor,
	fromStatus, toStatus ntpdb.ServerScoresStatus,
	safety safetyCheckResult,
	skipPerformance bool,
	removalsAllowed int,
) []statusChange {
	if removalsAllowed <= 0 {
		return nil
	}

	var changes []statusChange
	// Iterate backwards to demote worst performers first
	for i := len(monitors) - 1; i >= 0 && removalsAllowed > 0; i-- {
		em := monitors[i]
		if em.recommendedState != candidateOut {
			continue
		}

		// Skip performance-based removals if only allowing constraint removals
		if skipPerformance && (em.currentViolation == nil || em.currentViolation.Type == violationNone) {
			continue
		}

		changes = append(changes, statusChange{
			monitorID:  em.monitor.ID,
			fromStatus: fromStatus,
			toStatus:   toStatus,
			reason:     "gradual removal (health or constraints)",
		})
		removalsAllowed--
	}

	return changes
}

// Rule 2 (Gradual Constraint Removal): Gradual removal of candidateOut monitors
func (sl *Selector) applyRule2GradualConstraintRemoval(
	ctx context.Context,
	selCtx selectionContext,
	activeMonitors []evaluatedMonitor,
	testingMonitors []evaluatedMonitor,
	currentActiveMonitors int,
) []statusChange {
	// Process active removals
	activeChanges := sl.processRemovals(ctx, activeMonitors,
		ntpdb.ServerScoresStatusActive, ntpdb.ServerScoresStatusTesting,
		currentActiveMonitors, max(1, selCtx.targetNumber-2),
		selCtx.limits.activeRemovals, currentActiveMonitors)

	// Calculate testing count after demotions
	testingCount := len(testingMonitors)
	for _, c := range activeChanges {
		if c.toStatus == ntpdb.ServerScoresStatusTesting {
			testingCount++
		}
	}

	// Process testing removals with updated count
	testingChanges := sl.processRemovals(ctx, testingMonitors,
		ntpdb.ServerScoresStatusTesting, ntpdb.ServerScoresStatusCandidate,
		testingCount, max(1, baseTestingTarget-2),
		selCtx.limits.testingRemovals, currentActiveMonitors)

	return append(activeChanges, testingChanges...)
}

// Rule 1.5 (Active Excess Demotion): Demote excess healthy active monitors when over target
func (sl *Selector) applyRule1_5ActiveExcessDemotion(
	ctx context.Context,
	selCtx selectionContext,
	activeMonitors []evaluatedMonitor,
	workingActiveCount int,
	workingTestingCount int,
	demotionsSoFar int,
) ruleResult {
	var changes []statusChange

	if workingActiveCount > selCtx.targetNumber &&
		workingActiveCount > 1 && // Never reduce to 0
		!selCtx.emergencyOverride &&
		selCtx.limits.activeRemovals > demotionsSoFar {

		excessActive := workingActiveCount - selCtx.targetNumber

		// Reserve demotion budget for constraint violations
		constraintDemotionsNeeded := 0
		for _, em := range activeMonitors {
			if em.recommendedState == candidateOut {
				constraintDemotionsNeeded++
			}
		}
		reservedDemotions := min(constraintDemotionsNeeded, selCtx.limits.activeRemovals-demotionsSoFar)
		availableDemotions := max(0, selCtx.limits.activeRemovals-demotionsSoFar-reservedDemotions)
		demotionsNeeded := min(excessActive, availableDemotions)

		if demotionsNeeded > 0 {
			// Use existing sort order - take worst performers from end of list
			startIndex := len(activeMonitors) - demotionsNeeded
			actualDemotions := 0

			for i := startIndex; i < len(activeMonitors) && actualDemotions < demotionsNeeded; i++ {
				em := activeMonitors[i]
				// Only demote healthy monitors (not ones already marked for demotion)
				if em.recommendedState != candidateOut {
					changes = append(changes, statusChange{
						monitorID:  em.monitor.ID,
						fromStatus: ntpdb.ServerScoresStatusActive,
						toStatus:   ntpdb.ServerScoresStatusTesting,
						reason:     "excess active demotion",
					})
					workingActiveCount--
					workingTestingCount++
					actualDemotions++
				}
			}

			sl.log.InfoContext(ctx, "demoted excess active monitors",
				"count", actualDemotions,
				"newActiveCount", workingActiveCount,
				"newTestingCount", workingTestingCount)
		}
	}

	return ruleResult{
		changes:      changes,
		activeCount:  workingActiveCount,
		testingCount: workingTestingCount,
	}
}

// Rule 3 (Testing to Active Promotion): Promote from testing to active
func (sl *Selector) applyRule3TestingToActivePromotion(
	ctx context.Context,
	selCtx selectionContext,
	testingMonitors []evaluatedMonitor,
	activeMonitors []evaluatedMonitor,
	workingAccountLimits map[uint32]*accountLimit,
	workingActiveCount int,
	workingTestingCount int,
) ruleResult {
	var changes []statusChange
	changesRemaining := selCtx.limits.promotions
	toAdd := max(0, selCtx.targetNumber-workingActiveCount)

	if toAdd > 0 && changesRemaining > 0 {
		promotionsNeeded := min(toAdd, changesRemaining)
		promoted := 0

		if selCtx.emergencyOverride {
			sl.log.WarnContext(ctx, "emergency override: zero active monitors, ignoring constraints for promotion",
				"testingMonitors", len(testingMonitors))
		}

		// Iteratively check each testing monitor for promotion
		for _, em := range testingMonitors {
			if promoted >= promotionsNeeded {
				break
			}

			// Check count requirement for testing->active promotion
			if em.monitor.Count < int64(minCountForActive) {
				continue // Skip this monitor, insufficient data points
			}

			req := promotionRequest{
				monitor:           &em.monitor,
				server:            selCtx.server,
				workingLimits:     workingAccountLimits,
				assignedMonitors:  selCtx.assignedMonitors,
				emergencyOverride: selCtx.emergencyOverride,
				fromStatus:        ntpdb.ServerScoresStatusTesting,
				toStatus:          ntpdb.ServerScoresStatusActive,
				baseReason:        "promotion to active",
				emergencyReason:   getEmergencyReason("promotion to active", ntpdb.ServerScoresStatusActive, false),
			}

			if result := sl.attemptPromotion(req); result.success {
				changes = append(changes, *result.change)
				workingActiveCount += result.activeIncrement
				workingTestingCount += result.testingIncrement
				promoted++
			}
		}
	}

	// Phase 2: Performance-based replacement (new logic)
	remainingBudget := selCtx.limits.promotions - len(changes)
	if remainingBudget > 0 && len(testingMonitors) > 0 && len(activeMonitors) > 0 {
		replacementChanges := sl.attemptPerformanceReplacement(
			ctx, selCtx, testingMonitors, activeMonitors,
			workingAccountLimits, remainingBudget, testingToActive,
		)
		changes = append(changes, replacementChanges...)

		// Log Phase 2 results
		if len(replacementChanges) > 0 {
			sl.log.InfoContext(ctx, "rule 3 phase 2 complete",
				slog.Uint64("serverID", uint64(selCtx.server.ID)),
				slog.Int("swaps", len(replacementChanges)/2),
				slog.Int("total_changes", len(changes)),
			)
		}
	}

	return ruleResult{
		changes:      changes,
		activeCount:  workingActiveCount,
		testingCount: workingTestingCount,
	}
}

// Rule 5 (Candidate to Testing Promotion): Promote candidates to testing
func (sl *Selector) applyRule5CandidateToTestingPromotion(
	ctx context.Context,
	selCtx selectionContext,
	candidateMonitors []evaluatedMonitor,
	testingMonitors []evaluatedMonitor,
	workingAccountLimits map[uint32]*accountLimit,
	workingActiveCount int,
	workingTestingCount int,
) ruleResult {
	var changes []statusChange
	changesRemaining := selCtx.limits.promotions

	if changesRemaining > 0 && len(candidateMonitors) > 0 {
		// Calculate dynamic testing target to avoid over-promoting
		activeGap := max(0, selCtx.targetNumber-workingActiveCount)
		dynamicTestingTarget := baseTestingTarget + activeGap
		testingCapacity := max(0, dynamicTestingTarget-workingTestingCount)

		sl.log.InfoContext(ctx, "Rule 5 Phase 1: capacity-based promotion analysis",
			slog.Int("changesRemaining", changesRemaining),
			slog.Int("candidates", len(candidateMonitors)),
			slog.Int("testing", len(testingMonitors)),
			slog.Int("workingActiveCount", workingActiveCount),
			slog.Int("workingTestingCount", workingTestingCount),
			slog.Int("targetNumber", selCtx.targetNumber),
			slog.Int("activeGap", activeGap),
			slog.Int("baseTestingTarget", baseTestingTarget),
			slog.Int("dynamicTestingTarget", dynamicTestingTarget),
			slog.Int("testingCapacity", testingCapacity),
		)

		// Phase 1: Capacity-based promotion (existing logic)
		promotionsNeeded := min(min(changesRemaining, 2), testingCapacity) // Respect testing capacity
		promoted := 0

		sl.log.InfoContext(ctx, "Rule 5 Phase 1: promotion budget",
			slog.Int("promotionsNeeded", promotionsNeeded),
		)

		// Try globally active candidates first, then globally testing (they are pre-sorted by performance)
		for _, em := range candidateMonitors {
			if promoted >= promotionsNeeded {
				break
			}

			// Only consider globally active or testing candidates
			if em.monitor.GlobalStatus != ntpdb.MonitorsStatusActive && em.monitor.GlobalStatus != ntpdb.MonitorsStatusTesting {
				continue
			}

			// Check count requirement for candidate->testing promotion
			if em.monitor.Count < int64(minCountForTesting) {
				continue // Skip this monitor, insufficient data points
			}

			req := promotionRequest{
				monitor:           &em.monitor,
				server:            selCtx.server,
				workingLimits:     workingAccountLimits,
				assignedMonitors:  selCtx.assignedMonitors,
				emergencyOverride: selCtx.emergencyOverride,
				fromStatus:        ntpdb.ServerScoresStatusCandidate,
				toStatus:          ntpdb.ServerScoresStatusTesting,
				baseReason:        "candidate to testing",
				emergencyReason:   getEmergencyReason("candidate to testing", ntpdb.ServerScoresStatusTesting, false),
			}

			if result := sl.attemptPromotion(req); result.success {
				changes = append(changes, *result.change)
				workingTestingCount += result.testingIncrement
				promoted++
				changesRemaining--
			}
		}

		// Phase 2: Performance-based replacement (new logic)
		// Only attempt replacement if we have budget remaining and viable candidates
		// Calculate remaining budget after capacity promotions
		remainingBudget := selCtx.limits.promotions - promoted

		sl.log.DebugContext(ctx, "Rule 5 Phase 2: performance replacement analysis",
			slog.Int("promoted_in_phase1", promoted),
			slog.Int("remainingBudget", remainingBudget),
			slog.Int("candidates", len(candidateMonitors)),
			slog.Int("testing", len(testingMonitors)),
		)

		if remainingBudget > 0 && len(candidateMonitors) > 0 && len(testingMonitors) > 0 {
			sl.log.DebugContext(ctx, "Rule 5 Phase 2: attempting performance-based replacement")
			replacementChanges := sl.attemptPerformanceReplacement(
				ctx,
				selCtx,
				candidateMonitors,
				testingMonitors,
				workingAccountLimits,
				remainingBudget,
				candidateToTesting,
			)
			changes = append(changes, replacementChanges...)
			// Note: workingTestingCount doesn't change for replacements (1 out, 1 in)

			sl.log.InfoContext(ctx, "Rule 5 Phase 2: replacement results",
				slog.Int("replacementChanges", len(replacementChanges)),
			)
		} else {
			sl.log.InfoContext(ctx, "Rule 5 Phase 2: skipping replacement",
				slog.Bool("hasBudget", remainingBudget > 0),
				slog.Bool("hasCandidates", len(candidateMonitors) > 0),
				slog.Bool("hasTesting", len(testingMonitors) > 0),
			)
		}
	}

	return ruleResult{
		changes:      changes,
		activeCount:  workingActiveCount, // activeCount stays same for candidate->testing
		testingCount: workingTestingCount,
	}
}

// Rule 2.5 (Testing Pool Management): Demote excess testing monitors based on dynamic target
func (sl *Selector) applyRule2_5TestingPoolManagement(
	ctx context.Context,
	selCtx selectionContext,
	testingMonitors []evaluatedMonitor,
	workingActiveCount int,
	workingTestingCount int,
	existingChanges []statusChange,
) ruleResult {
	var changes []statusChange

	// Calculate dynamic testing target based on working active monitor gap
	activeGap := max(0, selCtx.targetNumber-workingActiveCount)
	dynamicTestingTarget := baseTestingTarget + activeGap

	if workingTestingCount > dynamicTestingTarget {
		excessTesting := workingTestingCount - dynamicTestingTarget

		// Count how many testing demotions have already been made
		testingDemotionsSoFar := 0
		for _, change := range existingChanges {
			if change.fromStatus == ntpdb.ServerScoresStatusTesting && change.toStatus == ntpdb.ServerScoresStatusCandidate {
				testingDemotionsSoFar++
			}
		}

		dynamicTestingRemovalsRemaining := max(0, selCtx.limits.testingRemovals-testingDemotionsSoFar)
		demotionsNeeded := min(excessTesting, dynamicTestingRemovalsRemaining)

		if demotionsNeeded > 0 {
			sl.log.InfoContext(ctx, "demoting excess testing monitors",
				"currentTesting", workingTestingCount,
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
			if em.recommendedState != candidateOut && (em.currentViolation == nil || em.currentViolation.Type == violationNone) {
				changes = append(changes, statusChange{
					monitorID:  em.monitor.ID,
					fromStatus: ntpdb.ServerScoresStatusTesting,
					toStatus:   ntpdb.ServerScoresStatusCandidate,
					reason:     fmt.Sprintf("excess testing monitors (%d > target %d)", workingTestingCount, dynamicTestingTarget),
				})
				demoted++
				workingTestingCount--
			}
		}
	}

	return ruleResult{
		changes:      changes,
		activeCount:  workingActiveCount, // activeCount stays same
		testingCount: workingTestingCount,
	}
}

// Rule 6 (Bootstrap Promotion): Bootstrap case - if no testing monitors exist, promote candidates
func (sl *Selector) applyRule6BootstrapPromotion(
	ctx context.Context,
	selCtx selectionContext,
	testingMonitors []evaluatedMonitor,
	candidateMonitors []evaluatedMonitor,
	workingAccountLimits map[uint32]*accountLimit,
	workingActiveCount int,
	workingTestingCount int,
) ruleResult {
	var changes []statusChange

	// Bootstrap case - if no testing monitors exist, promote candidates to reach target
	if len(testingMonitors) == 0 && len(candidateMonitors) > 0 {
		// In bootstrap scenario, we can promote up to baseTestingTarget at once
		bootstrapPromotions := baseTestingTarget
		promoted := 0

		sl.log.Info("bootstrap: no testing monitors, promoting candidates to start monitoring",
			"candidatesAvailable", len(candidateMonitors),
			"baseTestingTarget", baseTestingTarget,
			"bootstrapPromotions", bootstrapPromotions)

		// Sort candidates by health first, then by global status
		healthyCandidates, otherCandidates := filterBootstrapCandidates(candidateMonitors)

		// Bootstrap candidate groups: healthy first, then others
		bootstrapGroups := []struct {
			monitors []evaluatedMonitor
			name     string
		}{
			{healthyCandidates, "healthy"},
			{otherCandidates, "other"},
		}

		for _, group := range bootstrapGroups {
			for _, em := range group.monitors {
				if promoted >= bootstrapPromotions {
					break
				}

				baseReason := "bootstrap: promoting candidate"
				if group.name == "healthy" {
					baseReason = "bootstrap: promoting healthy candidate"
				}

				req := promotionRequest{
					monitor:           &em.monitor,
					server:            selCtx.server,
					workingLimits:     workingAccountLimits,
					assignedMonitors:  selCtx.assignedMonitors,
					emergencyOverride: selCtx.emergencyOverride,
					fromStatus:        ntpdb.ServerScoresStatusCandidate,
					toStatus:          ntpdb.ServerScoresStatusTesting,
					baseReason:        baseReason,
					emergencyReason:   getEmergencyReason(baseReason, ntpdb.ServerScoresStatusTesting, true),
				}

				if result := sl.attemptPromotion(req); result.success {
					changes = append(changes, *result.change)
					workingTestingCount += result.testingIncrement
					promoted++
				}
			}
		}

		if promoted == 0 && len(candidateMonitors) > 0 {
			sl.log.Warn("bootstrap: unable to promote any candidates due to constraints",
				"candidatesAvailable", len(candidateMonitors))
		}
	}

	return ruleResult{
		changes:      changes,
		activeCount:  workingActiveCount, // activeCount stays same for candidate->testing
		testingCount: workingTestingCount,
	}
}

// Rule 7 (Constraint Resolution): Check paused monitors for constraint resolution
func (sl *Selector) applyRule7ConstraintResolution(
	ctx context.Context,
	selCtx selectionContext,
	pausedMonitors []evaluatedMonitor,
) []statusChange {
	var changes []statusChange

	for _, eval := range pausedMonitors {
		monitor := eval.monitor

		// Get pause reason from the monitor
		var pauseReasonValue pauseReason
		if monitor.PauseReason != nil {
			pauseReasonValue = pauseReason(*monitor.PauseReason)
		} else {
			// Default to constraint violation for backward compatibility
			pauseReasonValue = pauseConstraintViolation
		}

		// Check if we should evaluate this monitor based on timing
		if !sl.shouldCheckConstraintResolution(monitor, pauseReasonValue) {
			continue
		}

		// Check if constraints are now resolved
		if sl.checkConstraintResolution(monitor, selCtx.server, selCtx.accountLimits, selCtx.assignedMonitors) {
			changes = append(changes, statusChange{
				monitorID:  monitor.ID,
				fromStatus: ntpdb.ServerScoresStatusPaused,
				toStatus:   ntpdb.ServerScoresStatusCandidate,
				reason:     "constraint resolution: unpausing resolved constraint",
			})

			sl.log.Info("unpausing monitor due to constraint resolution",
				"serverID", selCtx.server.ID,
				"monitorID", monitor.ID,
				"pauseReason", pauseReasonValue,
				"violationType", monitor.ConstraintViolationType)

			// Limit constraint resolution changes per run to prevent oscillation
			if len(changes) >= 2 {
				break
			}
		}
		// Always update last constraint check timestamp for evaluated monitors
		// Note: This will be handled by the status change tracking or in the main selector loop
	}

	return changes
}

// Rule 8 (Out-of-Order Optimization): Handle out-of-order situations (currently disabled)
func (sl *Selector) applyRule8OutOfOrderOptimization(
	ctx context.Context,
	selCtx selectionContext,
) ruleResult {
	// Rule 8 (Out-of-Order Optimization): Handle out-of-order situations (disabled for now to respect change limits)
	// TODO: Implement out-of-order logic that respects allowedChanges limits
	return ruleResult{
		changes:      []statusChange{},
		activeCount:  0, // No changes
		testingCount: 0, // No changes
	}
}

// initializeWorkingCounts sets up initial working state from monitor counts
func (sl *Selector) initializeWorkingCounts(
	activeMonitors []evaluatedMonitor,
	testingMonitors []evaluatedMonitor,
	evaluatedMonitors []evaluatedMonitor,
) workingState {
	return workingState{
		activeCount:    len(activeMonitors),
		testingCount:   len(testingMonitors),
		healthyActive:  sl.countHealthy(activeMonitors),
		healthyTesting: sl.countHealthy(testingMonitors),
		blockedCount:   sl.countBlocked(evaluatedMonitors),
	}
}

// updateWorkingCountsForChanges applies planned changes to working counts
func (sl *Selector) updateWorkingCountsForChanges(
	state workingState,
	changes []statusChange,
) workingState {
	for _, change := range changes {
		if change.fromStatus == ntpdb.ServerScoresStatusActive && change.toStatus != ntpdb.ServerScoresStatusActive {
			state.activeCount--
		}
		if change.toStatus == ntpdb.ServerScoresStatusTesting {
			state.testingCount++
		}
		if change.fromStatus == ntpdb.ServerScoresStatusTesting && change.toStatus != ntpdb.ServerScoresStatusTesting {
			state.testingCount--
		}
	}
	return state
}

// calculateSafetyLimits applies emergency safeguards and safety checks
func (sl *Selector) calculateSafetyLimits(
	ctx context.Context,
	state workingState,
	targetNumber int,
	limits changeLimits,
	evaluatedMonitors []evaluatedMonitor,
) workingState {
	// Emergency safeguards
	if targetNumber > len(evaluatedMonitors) && state.healthyActive < state.activeCount {
		sl.log.Warn("emergency: not enough healthy monitors available",
			"targetNumber", targetNumber,
			"totalMonitors", len(evaluatedMonitors),
			"healthyActive", state.healthyActive,
			"currentActive", state.activeCount)
		state.maxRemovals = 0 // No changes in emergency situation
		return state
	}

	maxRemovals := limits.activeRemovals

	// Safety: don't remove monitors if we're at/below target and don't have enough healthy
	if state.activeCount <= targetNumber && state.healthyActive < targetNumber {
		if state.activeCount > 0 {
			maxRemovals = 0
		}
	}

	// Emergency: never remove all active monitors
	if maxRemovals >= state.activeCount && state.activeCount > 0 {
		maxRemovals = state.activeCount - 1
		sl.log.Warn("emergency: limiting removals to prevent zero active monitors",
			"originalMaxRemovals", limits.activeRemovals,
			"adjustedMaxRemovals", maxRemovals,
			"currentActive", state.activeCount)
	}

	state.maxRemovals = maxRemovals
	return state
}

// performFinalValidation validates final counts against targets
func (sl *Selector) performFinalValidation(
	ctx context.Context,
	state workingState,
	targetActiveMonitors int,
	server *serverInfo,
) {
	if state.activeCount > targetActiveMonitors {
		sl.log.ErrorContext(ctx, "CRITICAL: active monitor count exceeds target after selection",
			"finalActiveCount", state.activeCount,
			"target", targetActiveMonitors,
			"serverID", server.ID)
	}

	finalTestingTarget := baseTestingTarget + max(0, targetActiveMonitors-state.activeCount)
	if state.testingCount > finalTestingTarget {
		sl.log.ErrorContext(ctx, "CRITICAL: testing monitor count exceeds target after selection",
			"finalTestingCount", state.testingCount,
			"target", finalTestingTarget,
			"serverID", server.ID)
	}
}

// applySelectionRules determines what status changes should be made
//
// Selection Rules (executed in order):
//
//	Rule 1 (Immediate Blocking): Remove monitors that should be blocked immediately
//	Rule 2 (Gradual Constraint Removal): Gradual removal of candidateOut monitors (with limits)
//	Rule 1.5 (Active Excess Demotion): Demote excess healthy active monitors when over target
//	Rule 3 (Testing to Active Promotion): Promote from testing to active (iterative constraint checking)
//	Rule 5 (Candidate to Testing Promotion): Promote candidates to testing (iterative constraint checking)
//	Rule 2.5 (Testing Pool Management): Demote excess testing monitors based on dynamic target
//	Rule 6 (Bootstrap Promotion): Bootstrap case - if no testing monitors exist, promote candidates to reach target
//	Rule 7 (Constraint Resolution): Check paused monitors for constraint resolution
//	Rule 8 (Out-of-Order Optimization): Handle out-of-order situations (disabled for now)
func (sl *Selector) applySelectionRules(
	ctx context.Context,
	evaluatedMonitors []evaluatedMonitor,
	server *serverInfo,
	accountLimits map[uint32]*accountLimit,
	assignedMonitors []ntpdb.GetMonitorPriorityRow,
) []statusChange {
	// Categorize monitors by current status
	var (
		activeMonitors    []evaluatedMonitor
		testingMonitors   []evaluatedMonitor
		candidateMonitors []evaluatedMonitor
		pausedMonitors    []evaluatedMonitor
	)

	for _, em := range evaluatedMonitors {
		switch em.monitor.ServerStatus {
		case ntpdb.ServerScoresStatusActive:
			activeMonitors = append(activeMonitors, em)
		case ntpdb.ServerScoresStatusTesting:
			testingMonitors = append(testingMonitors, em)
		case ntpdb.ServerScoresStatusCandidate:
			candidateMonitors = append(candidateMonitors, em)
		case ntpdb.ServerScoresStatusPaused:
			pausedMonitors = append(pausedMonitors, em)
		}
	}

	// Initialize working state and limits
	targetNumber := targetActiveMonitors
	limits := calculateChangeLimits(len(activeMonitors), sl.countBlocked(evaluatedMonitors))
	state := sl.initializeWorkingCounts(activeMonitors, testingMonitors, evaluatedMonitors)
	emergencyOverride := (len(activeMonitors) == 0)

	// Apply safety limits and emergency safeguards
	state = sl.calculateSafetyLimits(ctx, state, targetNumber, limits, evaluatedMonitors)

	// Check if there are any constraint violations to process
	hasConstraintViolations := false
	for _, em := range evaluatedMonitors {
		if em.recommendedState == candidateOut {
			hasConstraintViolations = true
			break
		}
	}

	// Only block ALL changes in truly dire emergency situations:
	// 1. We need more monitors than exist AND
	// 2. We have no healthy monitors to work with AND
	// 3. There are no constraint violations to clean up
	emergencyBlockAll := targetNumber > len(evaluatedMonitors) &&
		state.healthyActive == 0 && state.healthyTesting == 0 &&
		!hasConstraintViolations

	if emergencyBlockAll {
		return []statusChange{} // Emergency: no changes
	}

	// Log initial state
	sl.log.Debug("current monitor counts and limits",
		"active", state.activeCount,
		"healthyActive", state.healthyActive,
		"testing", state.testingCount,
		"healthyTesting", state.healthyTesting,
		"candidates", len(candidateMonitors),
		"blocked", state.blockedCount,
		"allowedChanges", limits.activeRemovals,
		"maxRemovals", state.maxRemovals)

	// Create working copy of account limits for iterative constraint checking
	workingAccountLimits := make(map[uint32]*accountLimit)
	for k, v := range accountLimits {
		workingAccountLimits[k] = &accountLimit{
			AccountID:    v.AccountID,
			MaxPerServer: v.MaxPerServer,
			ActiveCount:  v.ActiveCount,
			TestingCount: v.TestingCount,
		}
	}

	// Create selection context for all rules
	selCtx := selectionContext{
		evaluatedMonitors: evaluatedMonitors,
		server:            server,
		accountLimits:     accountLimits,
		assignedMonitors:  assignedMonitors,
		limits:            limits,
		targetNumber:      targetNumber,
		emergencyOverride: emergencyOverride,
	}

	var allChanges []statusChange

	// Rule 1 (Immediate Blocking): Remove monitors that should be blocked immediately
	rule1Changes := sl.applyRule1ImmediateBlocking(ctx, selCtx, activeMonitors, testingMonitors)
	allChanges = append(allChanges, rule1Changes...)
	state = sl.updateWorkingCountsForChanges(state, rule1Changes)

	// Rule 2 (Gradual Constraint Removal): Gradual removal of candidateOut monitors
	rule2Changes := sl.applyRule2GradualConstraintRemoval(ctx, selCtx, activeMonitors, testingMonitors, len(activeMonitors))
	allChanges = append(allChanges, rule2Changes...)
	state = sl.updateWorkingCountsForChanges(state, rule2Changes)

	// Count demotions so far for Rule 1.5
	demotionsSoFar := 0
	for _, change := range allChanges {
		if change.fromStatus == ntpdb.ServerScoresStatusActive && change.toStatus != ntpdb.ServerScoresStatusActive {
			demotionsSoFar++
		}
	}

	// Rule 1.5 (Active Excess Demotion): Demote excess healthy active monitors when over target
	rule1_5Result := sl.applyRule1_5ActiveExcessDemotion(ctx, selCtx, activeMonitors, state.activeCount, state.testingCount, demotionsSoFar)
	allChanges = append(allChanges, rule1_5Result.changes...)
	state.activeCount = rule1_5Result.activeCount
	state.testingCount = rule1_5Result.testingCount

	// Rule 3 (Testing to Active Promotion): Promote from testing to active
	sl.log.InfoContext(ctx, "Rule 3: starting testing to active promotion",
		slog.Int("promotion_budget_before", selCtx.limits.promotions),
		slog.Int("active_count", state.activeCount),
		slog.Int("testing_count", state.testingCount),
	)
	rule3Result := sl.applyRule3TestingToActivePromotion(ctx, selCtx, testingMonitors, activeMonitors, workingAccountLimits, state.activeCount, state.testingCount)
	allChanges = append(allChanges, rule3Result.changes...)
	state.activeCount = rule3Result.activeCount
	state.testingCount = rule3Result.testingCount

	// Update promotion budget after Rule 3
	promotionsUsedByRule3 := len(rule3Result.changes)
	selCtx.limits.promotions = max(0, selCtx.limits.promotions-promotionsUsedByRule3)

	sl.log.InfoContext(ctx, "Rule 3: completed testing to active promotion",
		slog.Int("promotions_used", promotionsUsedByRule3),
		slog.Int("promotion_budget_remaining", selCtx.limits.promotions),
	)

	// Rule 5 (Candidate to Testing Promotion): Promote candidates to testing
	sl.log.InfoContext(ctx, "Rule 5: starting candidate to testing promotion",
		slog.Int("promotion_budget", selCtx.limits.promotions),
		slog.Int("candidates", len(candidateMonitors)),
		slog.Int("testing", len(testingMonitors)),
	)
	rule5Result := sl.applyRule5CandidateToTestingPromotion(ctx, selCtx, candidateMonitors, testingMonitors, workingAccountLimits, state.activeCount, state.testingCount)
	allChanges = append(allChanges, rule5Result.changes...)
	state.testingCount = rule5Result.testingCount

	// Rule 2.5 (Testing Pool Management): Demote excess testing monitors based on dynamic target
	rule2_5Result := sl.applyRule2_5TestingPoolManagement(ctx, selCtx, testingMonitors, state.activeCount, state.testingCount, allChanges)
	allChanges = append(allChanges, rule2_5Result.changes...)
	state.testingCount = rule2_5Result.testingCount

	// Rule 7 (Constraint Resolution): Check paused monitors for constraint resolution
	rule7Changes := sl.applyRule7ConstraintResolution(ctx, selCtx, pausedMonitors)
	allChanges = append(allChanges, rule7Changes...)

	// Rule 6 (Bootstrap Promotion): Bootstrap case - if no testing monitors exist, promote candidates
	rule6Result := sl.applyRule6BootstrapPromotion(ctx, selCtx, testingMonitors, candidateMonitors, workingAccountLimits, state.activeCount, state.testingCount)
	allChanges = append(allChanges, rule6Result.changes...)
	state.testingCount = rule6Result.testingCount

	// Rule 8 (Out-of-Order Optimization): Handle out-of-order situations (disabled)
	_ = sl.applyRule8OutOfOrderOptimization(ctx, selCtx)

	// Final safety validation
	sl.performFinalValidation(ctx, state, targetNumber, server)

	// Legacy logging for backward compatibility
	allowedChanges := limits.activeRemovals
	sl.log.Debug("planned changes",
		"totalChanges", len(allChanges),
		"allowedChanges", allowedChanges,
		"finalActiveCount", state.activeCount,
		"finalTestingCount", state.testingCount)

	return allChanges
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
	db ntpdb.QuerierTx,
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

func (sl *Selector) countCandidateOut(monitors []evaluatedMonitor) int {
	count := 0
	for _, em := range monitors {
		if em.recommendedState == candidateOut {
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

// max returns the larger of two integers
func max(a, b int) int {
	if a > b {
		return a
	}
	return b
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

// attemptPerformanceReplacement is the unified function for performance-based monitor replacements.
// It handles both candidate->testing (Rule 5) and testing->active (Rule 3) replacements.
func (sl *Selector) attemptPerformanceReplacement(
	ctx context.Context,
	selCtx selectionContext,
	replacerMonitors []evaluatedMonitor, // candidates or testing monitors (pre-sorted by performance)
	targetMonitors []evaluatedMonitor, // testing or active monitors
	workingAccountLimits map[uint32]*accountLimit,
	changesRemaining int,
	repType replacementType,
) []statusChange {
	var changes []statusChange
	replacements := 0

	// Determine status transitions and logging context based on replacement type
	var (
		logContext                                string
		replacerFromStatus, replacerToStatus      ntpdb.ServerScoresStatus
		targetFromStatus, targetToStatus          ntpdb.ServerScoresStatus
		replacerPromoteReason, targetDemoteReason string
	)

	switch repType {
	case candidateToTesting:
		logContext = "Rule 5 Phase 2: attempting candidate-testing replacements"
		replacerFromStatus = ntpdb.ServerScoresStatusCandidate
		replacerToStatus = ntpdb.ServerScoresStatusTesting
		targetFromStatus = ntpdb.ServerScoresStatusTesting
		targetToStatus = ntpdb.ServerScoresStatusCandidate
		replacerPromoteReason = "replacement promotion"
		targetDemoteReason = "replaced by better candidate"
	case testingToActive:
		logContext = "attempting active-testing replacements"
		replacerFromStatus = ntpdb.ServerScoresStatusTesting
		replacerToStatus = ntpdb.ServerScoresStatusActive
		targetFromStatus = ntpdb.ServerScoresStatusActive
		targetToStatus = ntpdb.ServerScoresStatusTesting
		replacerPromoteReason = "active-testing swap (promote)"
		targetDemoteReason = "active-testing swap (demote)"
	}

	// Log entry
	sl.log.InfoContext(ctx, logContext,
		slog.Uint64("serverID", uint64(selCtx.server.ID)),
		slog.Int("replacer_count", len(replacerMonitors)),
		slog.Int("target_count", len(targetMonitors)),
		slog.Int("budget", changesRemaining),
	)

	// Filter eligible target monitors (same logic for both rules)
	eligibleTargetMonitors := make([]evaluatedMonitor, 0)
	for _, em := range targetMonitors {
		if em.recommendedState != candidateOut &&
			(em.currentViolation == nil || em.currentViolation.Type == violationNone) {
			eligibleTargetMonitors = append(eligibleTargetMonitors, em)
		}
	}

	if len(eligibleTargetMonitors) == 0 {
		sl.log.DebugContext(ctx, "no eligible target monitors for replacement")
		return changes
	}

	// Filter eligible replacer monitors - both rules filter to GlobalStatus active || testing
	eligibleReplacerMonitors := make([]evaluatedMonitor, 0)
	sl.log.InfoContext(ctx, "filtering replacer monitors",
		slog.Int("total_replacers", len(replacerMonitors)),
	)
	for _, em := range replacerMonitors {
		// Special attention to monitors 106 and 160
		isSpecialMonitor := em.monitor.ID == 106 || em.monitor.ID == 160
		logLevel := slog.LevelDebug
		if isSpecialMonitor {
			logLevel = slog.LevelInfo
		}

		sl.log.LogAttrs(ctx, logLevel, "evaluating replacer monitor eligibility",
			slog.Uint64("monitorID", uint64(em.monitor.ID)),
			slog.String("globalStatus", string(em.monitor.GlobalStatus)),
			slog.Int("priority", em.monitor.Priority),
			slog.Int64("count", em.monitor.Count),
			slog.Bool("isEligible", em.monitor.GlobalStatus == ntpdb.MonitorsStatusActive || em.monitor.GlobalStatus == ntpdb.MonitorsStatusTesting),
			slog.Bool("isSpecialMonitor", isSpecialMonitor),
		)

		if em.monitor.GlobalStatus == ntpdb.MonitorsStatusActive || em.monitor.GlobalStatus == ntpdb.MonitorsStatusTesting {
			eligibleReplacerMonitors = append(eligibleReplacerMonitors, em)
			if isSpecialMonitor {
				sl.log.DebugContext(ctx, "SPECIAL MONITOR PASSED ELIGIBILITY",
					slog.Uint64("monitorID", uint64(em.monitor.ID)),
				)
			}
		} else if isSpecialMonitor {
			sl.log.DebugContext(ctx, "SPECIAL MONITOR FAILED ELIGIBILITY",
				slog.Uint64("monitorID", uint64(em.monitor.ID)),
				slog.String("globalStatus", string(em.monitor.GlobalStatus)),
				slog.String("expected", "active or testing"),
			)
		}
	}

	sl.log.InfoContext(ctx, "replacer monitor filtering results",
		slog.Int("eligible_replacers", len(eligibleReplacerMonitors)),
	)

	if len(eligibleReplacerMonitors) == 0 {
		sl.log.InfoContext(ctx, "no eligible replacer monitors for replacement - all filtered out")
		return changes
	}

	// Process each eligible target monitor (worst first) - iterate backwards since they're sorted best first
	for i := len(eligibleTargetMonitors) - 1; i >= 0; i-- {
		targetMonitor := eligibleTargetMonitors[i]
		targetIdx := i

		// Check budget - candidate->testing uses 1 per replacement, testing->active uses 2
		budgetNeeded := 1
		if repType == testingToActive {
			budgetNeeded = 2
		}

		if replacements*budgetNeeded >= changesRemaining {
			sl.log.DebugContext(ctx, "replacement budget exhausted",
				slog.Int("replacements", replacements),
				slog.Int("budget_used", replacements*budgetNeeded),
			)
			break
		}

		// Try each replacer (best first) that outperforms this target until one passes constraints
		var swapExecuted bool

		for idx, replacer := range eligibleReplacerMonitors {
			isSpecialReplacer := replacer.monitor.ID == 106 || replacer.monitor.ID == 160

			if isSpecialReplacer {
				sl.log.DebugContext(ctx, "EVALUATING SPECIAL MONITOR FOR REPLACEMENT",
					slog.Uint64("replacerMonitorID", uint64(replacer.monitor.ID)),
					slog.Int("replacer_priority", replacer.monitor.Priority),
					slog.Uint64("targetMonitorID", uint64(targetMonitor.monitor.ID)),
					slog.Int("target_priority", targetMonitor.monitor.Priority),
				)
			}

			if !sl.monitorOutperformsMonitor(ctx, replacer, targetMonitor) {
				if isSpecialReplacer {
					sl.log.DebugContext(ctx, "SPECIAL MONITOR FAILED PERFORMANCE CHECK",
						slog.Uint64("replacerMonitorID", uint64(replacer.monitor.ID)),
						slog.Int("replacer_priority", replacer.monitor.Priority),
						slog.Int("target_priority", targetMonitor.monitor.Priority),
					)
				}
				// Since replacers are sorted best first, if this one doesn't outperform,
				// none of the remaining ones will either
				break
			} else if isSpecialReplacer {
				sl.log.DebugContext(ctx, "SPECIAL MONITOR PASSED PERFORMANCE CHECK",
					slog.Uint64("replacerMonitorID", uint64(replacer.monitor.ID)),
					slog.Int("priority_improvement", targetMonitor.monitor.Priority-replacer.monitor.Priority),
				)
			}

			// Check count requirements for promotion
			var minRequiredCount int64
			switch repType {
			case candidateToTesting:
				minRequiredCount = int64(minCountForTesting)
				if replacer.monitor.Count < minRequiredCount {
					logMsg := "replacer skipped: insufficient data points"
					if isSpecialReplacer {
						logMsg = "SPECIAL MONITOR SKIPPED: insufficient data points"
					}
					sl.log.InfoContext(ctx, logMsg,
						slog.Uint64("replacerMonitorID", uint64(replacer.monitor.ID)),
						slog.Int64("count", replacer.monitor.Count),
						slog.Int64("minRequired", minRequiredCount),
					)
					continue // Skip this replacer, insufficient data points
				} else if isSpecialReplacer {
					sl.log.DebugContext(ctx, "SPECIAL MONITOR PASSED COUNT CHECK",
						slog.Uint64("replacerMonitorID", uint64(replacer.monitor.ID)),
						slog.Int64("count", replacer.monitor.Count),
						slog.Int64("minRequired", minRequiredCount),
					)
				}
			case testingToActive:
				minRequiredCount = int64(minCountForActive)
				if replacer.monitor.Count < minRequiredCount {
					sl.log.InfoContext(ctx, "replacer skipped: insufficient data points",
						slog.Uint64("replacerMonitorID", uint64(replacer.monitor.ID)),
						slog.Int64("count", replacer.monitor.Count),
						slog.Int64("minRequired", minRequiredCount),
					)
					continue // Skip this replacer, insufficient data points
				}
			}

			replacerMonitor := replacer

			// Log consideration
			sl.log.InfoContext(ctx, "considering performance-based swap",
				slog.Uint64("replacerMonitorID", uint64(replacerMonitor.monitor.ID)),
				slog.Int("replacer_priority", replacerMonitor.monitor.Priority),
				slog.Uint64("targetMonitorID", uint64(targetMonitor.monitor.ID)),
				slog.Int("target_priority", targetMonitor.monitor.Priority),
				slog.String("replacement_type", logContext),
			)

			// Create temporary copy of working limits to test the replacement
			tempWorkingLimits := sl.copyAccountLimits(workingAccountLimits)

			// Debug: Log original account limits for the replacer's account
			if replacerMonitor.monitor.AccountID != nil {
				if originalLimit, exists := workingAccountLimits[*replacerMonitor.monitor.AccountID]; exists {
					sl.log.DebugContext(ctx, "original account limits before replacement",
						slog.Uint64("replacerMonitorID", uint64(replacerMonitor.monitor.ID)),
						slog.Uint64("accountID", uint64(*replacerMonitor.monitor.AccountID)),
						slog.Int("originalActive", originalLimit.ActiveCount),
						slog.Int("originalTesting", originalLimit.TestingCount),
						slog.Int("originalTotal", originalLimit.ActiveCount+originalLimit.TestingCount),
					)
				}
			}

			// Apply the target demotion to temp limits first
			sl.updateAccountLimitsForPromotion(tempWorkingLimits, &targetMonitor.monitor,
				targetFromStatus, targetToStatus)

			// Debug: Log temp account limits after simulated demotion
			if replacerMonitor.monitor.AccountID != nil {
				if tempLimit, exists := tempWorkingLimits[*replacerMonitor.monitor.AccountID]; exists {
					sl.log.DebugContext(ctx, "temp account limits after simulated demotion",
						slog.Uint64("replacerMonitorID", uint64(replacerMonitor.monitor.ID)),
						slog.Uint64("targetMonitorID", uint64(targetMonitor.monitor.ID)),
						slog.Uint64("accountID", uint64(*replacerMonitor.monitor.AccountID)),
						slog.Int("tempActive", tempLimit.ActiveCount),
						slog.Int("tempTesting", tempLimit.TestingCount),
						slog.Int("tempTotal", tempLimit.ActiveCount+tempLimit.TestingCount),
						slog.String("demotionTransition", fmt.Sprintf("%s->%s", targetFromStatus, targetToStatus)),
					)
				}
			}

			// Now test if the replacer can be promoted with the updated limits
			replacerReq := promotionRequest{
				monitor:           &replacerMonitor.monitor,
				server:            selCtx.server,
				workingLimits:     tempWorkingLimits,
				assignedMonitors:  selCtx.assignedMonitors,
				emergencyOverride: selCtx.emergencyOverride,
				fromStatus:        replacerFromStatus,
				toStatus:          replacerToStatus,
				baseReason:        replacerPromoteReason,
				emergencyReason:   getEmergencyReason(replacerPromoteReason, replacerToStatus, false),
			}

			if promotionResult := sl.attemptPromotion(replacerReq); promotionResult.success {
				// Both moves are valid - perform the replacement
				changes = append(changes, statusChange{
					monitorID:  targetMonitor.monitor.ID,
					fromStatus: targetFromStatus,
					toStatus:   targetToStatus,
					reason:     targetDemoteReason,
				})
				changes = append(changes, statusChange{
					monitorID:  replacerMonitor.monitor.ID,
					fromStatus: replacerFromStatus,
					toStatus:   replacerToStatus,
					reason:     replacerPromoteReason,
				})

				// Update working limits for future iterations
				sl.updateAccountLimitsForPromotion(workingAccountLimits, &targetMonitor.monitor,
					targetFromStatus, targetToStatus)
				sl.updateAccountLimitsForPromotion(workingAccountLimits, &replacerMonitor.monitor,
					replacerFromStatus, replacerToStatus)

				// Remove used replacer from eligible list to prevent double-use
				eligibleReplacerMonitors = append(eligibleReplacerMonitors[:idx], eligibleReplacerMonitors[idx+1:]...)
				replacements++
				swapExecuted = true

				sl.log.InfoContext(ctx, "planned performance-based swap",
					slog.Uint64("replacerMonitorID", uint64(replacerMonitor.monitor.ID)),
					slog.Uint64("targetMonitorID", uint64(targetMonitor.monitor.ID)),
					slog.Int("priority_improvement", targetMonitor.monitor.Priority-replacerMonitor.monitor.Priority),
				)

				break // Move to next target monitor
			} else {
				sl.log.InfoContext(ctx, "performance-based swap blocked by constraints",
					slog.Uint64("replacerMonitorID", uint64(replacerMonitor.monitor.ID)),
					slog.Uint64("targetMonitorID", uint64(targetMonitor.monitor.ID)),
					slog.Int("replacer_priority", replacerMonitor.monitor.Priority),
					slog.Int("target_priority", targetMonitor.monitor.Priority),
				)
				// Continue to next replacer
			}
		}

		// If we executed a swap, remove the replaced target from eligible list
		if swapExecuted {
			// We need to adjust the target index since we're iterating backwards
			// and we've already removed a replacer which doesn't affect target indices
			eligibleTargetMonitors = append(eligibleTargetMonitors[:targetIdx], eligibleTargetMonitors[targetIdx+1:]...)
		}
	}

	return changes
}
