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

// Rule 2 (Gradual Constraint Removal): Gradual removal of candidateOut monitors
func (sl *Selector) applyRule2GradualConstraintRemoval(
	ctx context.Context,
	selCtx selectionContext,
	activeMonitors []evaluatedMonitor,
	testingMonitors []evaluatedMonitor,
	currentActiveMonitors int,
) []statusChange {
	var changes []statusChange

	// First remove active monitors (demote to testing) - use activeRemovals limit
	activeRemovalsRemaining := selCtx.limits.activeRemovals
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

	// Then remove testing monitors (demote to candidate)
	testingRemovalsRemaining := selCtx.limits.testingRemovals

	// When zero active monitors, allow more aggressive cleanup of testing violations
	if currentActiveMonitors == 0 {
		testingViolationCount := 0
		for _, em := range testingMonitors {
			if em.recommendedState == candidateOut {
				testingViolationCount++
			}
		}
		testingRemovalsRemaining = min(testingViolationCount, selCtx.limits.testingRemovals)
	}

	// Iterate backwards to demote worst performers first
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

	return changes
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

		// Phase 1: Capacity-based promotion (existing logic)
		promotionsNeeded := min(min(changesRemaining, 2), testingCapacity) // Respect testing capacity
		promoted := 0

		// Try globally active monitors first, then globally testing
		candidateGroups := createCandidateGroups(candidateMonitors)

		for _, group := range candidateGroups {
			for _, em := range group.monitors {
				if promoted >= promotionsNeeded {
					break
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
		}

		// Phase 2: Performance-based replacement (new logic)
		// Only attempt replacement if we have budget remaining and viable candidates
		// Calculate remaining budget after capacity promotions
		remainingBudget := selCtx.limits.promotions - promoted
		if remainingBudget > 0 && len(candidateMonitors) > 0 && len(testingMonitors) > 0 {
			replacementChanges := sl.attemptTestingReplacements(
				ctx,
				selCtx,
				candidateMonitors,
				testingMonitors,
				workingAccountLimits,
				remainingBudget,
			)
			changes = append(changes, replacementChanges...)
			// Note: workingTestingCount doesn't change for replacements (1 out, 1 in)
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

// Rule 7 (Out-of-Order Optimization): Handle out-of-order situations (currently disabled)
func (sl *Selector) applyRule7OutOfOrderOptimization(
	ctx context.Context,
	selCtx selectionContext,
) ruleResult {
	// Rule 7 (Out-of-Order Optimization): Handle out-of-order situations (disabled for now to respect change limits)
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
//	Rule 7 (Out-of-Order Optimization): Handle out-of-order situations (disabled for now)
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
	rule3Result := sl.applyRule3TestingToActivePromotion(ctx, selCtx, testingMonitors, workingAccountLimits, state.activeCount, state.testingCount)
	allChanges = append(allChanges, rule3Result.changes...)
	state.activeCount = rule3Result.activeCount
	state.testingCount = rule3Result.testingCount

	// Update promotion budget after Rule 3
	promotionsUsedByRule3 := len(rule3Result.changes)
	selCtx.limits.promotions = max(0, selCtx.limits.promotions-promotionsUsedByRule3)

	// Rule 5 (Candidate to Testing Promotion): Promote candidates to testing
	rule5Result := sl.applyRule5CandidateToTestingPromotion(ctx, selCtx, candidateMonitors, testingMonitors, workingAccountLimits, state.activeCount, state.testingCount)
	allChanges = append(allChanges, rule5Result.changes...)
	state.testingCount = rule5Result.testingCount

	// Rule 2.5 (Testing Pool Management): Demote excess testing monitors based on dynamic target
	rule2_5Result := sl.applyRule2_5TestingPoolManagement(ctx, selCtx, testingMonitors, state.activeCount, state.testingCount, allChanges)
	allChanges = append(allChanges, rule2_5Result.changes...)
	state.testingCount = rule2_5Result.testingCount

	// Rule 6 (Bootstrap Promotion): Bootstrap case - if no testing monitors exist, promote candidates
	rule6Result := sl.applyRule6BootstrapPromotion(ctx, selCtx, testingMonitors, candidateMonitors, workingAccountLimits, state.activeCount, state.testingCount)
	allChanges = append(allChanges, rule6Result.changes...)
	state.testingCount = rule6Result.testingCount

	// Rule 7 (Out-of-Order Optimization): Handle out-of-order situations (disabled)
	_ = sl.applyRule7OutOfOrderOptimization(ctx, selCtx)

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

// attemptTestingReplacements compares candidates with existing testing monitors
// and replaces worse-performing testing monitors with better candidates
func (sl *Selector) attemptTestingReplacements(
	ctx context.Context,
	selCtx selectionContext,
	candidateMonitors []evaluatedMonitor,
	testingMonitors []evaluatedMonitor,
	workingAccountLimits map[uint32]*accountLimit,
	changesRemaining int,
) []statusChange {
	var changes []statusChange
	replacements := 0

	// Only consider healthy testing monitors for replacement (not ones with violations)
	eligibleTestingMonitors := make([]evaluatedMonitor, 0)
	for _, em := range testingMonitors {
		if em.recommendedState != candidateOut &&
			(em.currentViolation == nil || em.currentViolation.Type == violationNone) {
			eligibleTestingMonitors = append(eligibleTestingMonitors, em)
		}
	}

	if len(eligibleTestingMonitors) == 0 {
		return changes
	}

	// Try globally active candidates first, then globally testing
	candidateGroups := createCandidateGroups(candidateMonitors)

	for _, group := range candidateGroups {
		for _, candidate := range group.monitors {
			if replacements >= changesRemaining {
				break
			}

			// Check if candidate can be promoted to testing
			candidateReq := promotionRequest{
				monitor:           &candidate.monitor,
				server:            selCtx.server,
				workingLimits:     workingAccountLimits,
				assignedMonitors:  selCtx.assignedMonitors,
				emergencyOverride: selCtx.emergencyOverride,
				fromStatus:        ntpdb.ServerScoresStatusCandidate,
				toStatus:          ntpdb.ServerScoresStatusTesting,
				baseReason:        "candidate replacement",
				emergencyReason:   getEmergencyReason("candidate replacement", ntpdb.ServerScoresStatusTesting, false),
			}

			if !sl.attemptPromotion(candidateReq).success {
				continue // This candidate can't be promoted due to constraints
			}

			// Find the worst-performing testing monitor that this candidate can replace
			// We iterate from the end (worst performers) to find the first one we can replace
			for i := len(eligibleTestingMonitors) - 1; i >= 0; i-- {
				testingMonitor := eligibleTestingMonitors[i]

				// Skip if this testing monitor is already better than the candidate
				// Since monitors are sorted by performance, if candidate is at position X in candidates
				// and testing monitor is at position Y in testing, we can only replace if candidate
				// outperforms the testing monitor (appears earlier in the combined sorted order)
				if !sl.candidateOutperformsTestingMonitor(candidate, testingMonitor) {
					continue
				}

				// Create a temporary copy of working limits to test the replacement
				tempWorkingLimits := sl.copyAccountLimits(workingAccountLimits)

				// Apply the demotion to temp limits
				// Note: For demotion from testing to candidate, we don't need constraint checking
				// since candidates have no constraints. We just update the account limits.
				sl.updateAccountLimitsForPromotion(tempWorkingLimits, &testingMonitor.monitor,
					ntpdb.ServerScoresStatusTesting, ntpdb.ServerScoresStatusCandidate)

				// Now test if the candidate can be promoted with the updated limits
				candidateReq.workingLimits = tempWorkingLimits
				if promotionResult := sl.attemptPromotion(candidateReq); promotionResult.success {
					// Both moves are valid - perform the replacement
					changes = append(changes, statusChange{
						monitorID:  testingMonitor.monitor.ID,
						fromStatus: ntpdb.ServerScoresStatusTesting,
						toStatus:   ntpdb.ServerScoresStatusCandidate,
						reason:     "replaced by better candidate",
					})
					changes = append(changes, statusChange{
						monitorID:  candidate.monitor.ID,
						fromStatus: ntpdb.ServerScoresStatusCandidate,
						toStatus:   ntpdb.ServerScoresStatusTesting,
						reason:     "replacement promotion",
					})

					// Update working limits for future iterations
					sl.updateAccountLimitsForPromotion(workingAccountLimits, &testingMonitor.monitor,
						ntpdb.ServerScoresStatusTesting, ntpdb.ServerScoresStatusCandidate)
					sl.updateAccountLimitsForPromotion(workingAccountLimits, &candidate.monitor,
						ntpdb.ServerScoresStatusCandidate, ntpdb.ServerScoresStatusTesting)

					// Remove the replaced testing monitor from eligible list
					eligibleTestingMonitors = append(eligibleTestingMonitors[:i], eligibleTestingMonitors[i+1:]...)
					replacements++

					break // Move to next candidate
				}
			}
		}
	}

	return changes
}
