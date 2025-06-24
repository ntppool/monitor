package selector

import (
	"context"
	"fmt"
	"time"

	"go.ntppool.org/monitor/ntpdb"
)

// Constants for monitor selection
const (
	targetActiveMonitors       = 7 // Target number of active monitors per server
	targetTestingMonitors      = 5 // Target number of testing monitors
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

// processServerNew implements the main selection algorithm for a single server
// This is the new implementation that will replace the existing processServer
func (sl *Selector) processServerNew(
	ctx context.Context,
	db *ntpdb.Queries,
	serverID uint32,
) (bool, error) {
	start := time.Now()
	sl.log.Debug("processing server", "serverID", serverID)

	// Step 1: Load server information
	server, err := sl.loadServerInfo(ctx, db, serverID)
	if err != nil {
		return false, fmt.Errorf("failed to load server info: %w", err)
	}

	// Step 2: Get all monitors (assigned and available)
	assignedMonitors, err := db.GetMonitorPriority(ctx, serverID)
	if err != nil {
		return false, fmt.Errorf("failed to get monitor priority: %w", err)
	}

	availableMonitors, err := sl.findAvailableMonitors(ctx, db, serverID)
	if err != nil {
		return false, fmt.Errorf("failed to find available monitors: %w", err)
	}

	// Step 3: Build account limits from assigned monitors
	accountLimits := sl.buildAccountLimitsFromMonitors(assignedMonitors)

	// Step 4: Evaluate all monitors against constraints
	evaluatedMonitors := make([]evaluatedMonitor, 0, len(assignedMonitors)+len(availableMonitors))

	// Process assigned monitors
	for _, row := range assignedMonitors {
		monitor := convertMonitorPriorityToCandidate(row)

		// Check constraints for current state
		violation := sl.checkConstraints(&monitor, server, accountLimits, monitor.ServerStatus, assignedMonitors)

		if violation.Type != violationNone {
			violation.IsGrandfathered = sl.isGrandfathered(&monitor, server, violation)

			// Track grandfathered violations in metrics
			if violation.IsGrandfathered && sl.metrics != nil {
				sl.metrics.TrackConstraintViolation(&monitor, violation.Type, serverID, true)
			}
		}

		state := sl.determineState(&monitor, violation)

		evaluatedMonitors = append(evaluatedMonitors, evaluatedMonitor{
			monitor:          monitor,
			violation:        violation,
			recommendedState: state,
		})
	}

	// Process available monitors
	for _, monitor := range availableMonitors {
		// Check constraints for potential candidate assignment
		targetState := ntpdb.ServerScoresStatusCandidate
		violation := sl.checkConstraints(&monitor, server, accountLimits, targetState, assignedMonitors)

		if violation.Type != violationNone {
			violation.IsGrandfathered = sl.isGrandfathered(&monitor, server, violation)

			// Track grandfathered violations in metrics
			if violation.IsGrandfathered && sl.metrics != nil {
				sl.metrics.TrackConstraintViolation(&monitor, violation.Type, serverID, true)
			}
		}

		state := sl.determineState(&monitor, violation)

		evaluatedMonitors = append(evaluatedMonitors, evaluatedMonitor{
			monitor:          monitor,
			violation:        violation,
			recommendedState: state,
		})
	}

	// Step 5: Apply selection rules
	changes := sl.applySelectionRules(evaluatedMonitors)

	// Step 6: Execute changes
	// Create a map from monitor ID to monitor candidate for metrics tracking
	monitorMap := make(map[uint32]*monitorCandidate)
	for _, em := range evaluatedMonitors {
		monitorMap[em.monitor.ID] = &em.monitor
	}

	changeCount := 0
	failedChanges := 0
	for _, change := range changes {
		monitor := monitorMap[change.monitorID]
		if err := sl.applyStatusChange(ctx, db, serverID, change, monitor); err != nil {
			failedChanges++
			sl.log.Error("failed to apply status change",
				"serverID", serverID,
				"monitorID", change.monitorID,
				"from", change.fromStatus,
				"to", change.toStatus,
				"error", err)
			// Continue with other changes
		} else {
			changeCount++
			sl.log.Info("applied status change",
				"serverID", serverID,
				"monitorID", change.monitorID,
				"from", change.fromStatus,
				"to", change.toStatus,
				"reason", change.reason)
		}
	}

	// Track constraint violations
	if err := sl.trackConstraintViolations(db, serverID, evaluatedMonitors); err != nil {
		sl.log.Error("failed to track constraint violations", "error", err)
		// Don't fail the whole operation for tracking errors
	}

	// Track performance metrics
	if sl.metrics != nil {
		duration := time.Since(start).Seconds()

		// Count globally active monitors
		globallyActiveCount := 0
		for _, em := range evaluatedMonitors {
			if em.monitor.GlobalStatus == ntpdb.MonitorsStatusActive {
				globallyActiveCount++
			}
		}

		// Track monitor pool sizes
		activeCount := 0
		testingCount := 0
		candidateCount := 0
		for _, em := range evaluatedMonitors {
			switch em.monitor.ServerStatus {
			case ntpdb.ServerScoresStatusActive:
				activeCount++
			case ntpdb.ServerScoresStatusTesting:
				testingCount++
			case ntpdb.ServerScoresStatusCandidate:
				candidateCount++
			}
		}

		sl.metrics.RecordProcessingMetrics(
			serverID,
			duration,
			len(evaluatedMonitors),
			changeCount,
			failedChanges,
			globallyActiveCount,
		)

		sl.metrics.TrackMonitorPoolSizes(
			serverID,
			activeCount,
			testingCount,
			candidateCount,
			len(availableMonitors),
		)
	}

	// Log summary
	sl.log.Info("server processing complete",
		"serverID", serverID,
		"assignedMonitors", len(assignedMonitors),
		"availableMonitors", len(availableMonitors),
		"evaluatedMonitors", len(evaluatedMonitors),
		"plannedChanges", len(changes),
		"appliedChanges", changeCount,
		"failedChanges", failedChanges)

	return changeCount > 0, nil
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

// findAvailableMonitors finds globally active/testing monitors not assigned to this server
func (sl *Selector) findAvailableMonitors(
	ctx context.Context,
	db *ntpdb.Queries,
	serverID uint32,
) ([]monitorCandidate, error) {
	rows, err := db.GetAvailableMonitors(ctx, serverID)
	if err != nil {
		return nil, fmt.Errorf("failed to get available monitors: %w", err)
	}

	candidates := make([]monitorCandidate, 0, len(rows))
	for _, row := range rows {
		var accountID *uint32
		if row.AccountID.Valid {
			id := uint32(row.AccountID.Int32)
			accountID = &id
		}

		var monitorIP string
		if row.MonitorIp.Valid {
			monitorIP = row.MonitorIp.String
		}

		var idToken, tlsName string
		if row.IDToken.Valid {
			idToken = row.IDToken.String
		}
		if row.TlsName.Valid {
			tlsName = row.TlsName.String
		}

		candidate := monitorCandidate{
			ID:           uint32(row.ID),
			IDToken:      idToken,
			TLSName:      tlsName,
			AccountID:    accountID,
			IP:           monitorIP,
			GlobalStatus: row.GlobalStatus,
			ServerStatus: ntpdb.ServerScoresStatusNew, // Not assigned to this server
			HasMetrics:   false,                       // No metrics for unassigned monitors
			IsHealthy:    false,                       // Can't determine health without metrics
			RTT:          0,
		}

		candidates = append(candidates, candidate)
	}

	return candidates, nil
}

// applySelectionRules determines what status changes should be made
func (sl *Selector) applySelectionRules(evaluatedMonitors []evaluatedMonitor) []statusChange {
	changes := make([]statusChange, 0)

	// Categorize monitors by current status
	var (
		activeMonitors    []evaluatedMonitor
		testingMonitors   []evaluatedMonitor
		candidateMonitors []evaluatedMonitor
		availablePool     []evaluatedMonitor
	)

	for _, em := range evaluatedMonitors {
		switch em.monitor.ServerStatus {
		case ntpdb.ServerScoresStatusActive:
			activeMonitors = append(activeMonitors, em)
		case ntpdb.ServerScoresStatusTesting:
			testingMonitors = append(testingMonitors, em)
		case ntpdb.ServerScoresStatusCandidate:
			candidateMonitors = append(candidateMonitors, em)
		case ntpdb.ServerScoresStatusNew:
			if em.recommendedState != candidateBlock {
				availablePool = append(availablePool, em)
			}
		}
	}

	// Count healthy monitors
	healthyActive := sl.countHealthy(activeMonitors)
	healthyTesting := sl.countHealthy(testingMonitors)

	sl.log.Debug("current monitor counts",
		"active", len(activeMonitors),
		"healthyActive", healthyActive,
		"testing", len(testingMonitors),
		"healthyTesting", healthyTesting,
		"candidates", len(candidateMonitors),
		"available", len(availablePool))

	// Rule 1: Remove monitors that should be blocked immediately
	for _, em := range activeMonitors {
		if em.recommendedState == candidateBlock {
			changes = append(changes, statusChange{
				monitorID:  em.monitor.ID,
				fromStatus: ntpdb.ServerScoresStatusActive,
				toStatus:   ntpdb.ServerScoresStatusNew,
				reason:     "blocked by constraints or global status",
			})
		}
	}

	for _, em := range testingMonitors {
		if em.recommendedState == candidateBlock {
			changes = append(changes, statusChange{
				monitorID:  em.monitor.ID,
				fromStatus: ntpdb.ServerScoresStatusTesting,
				toStatus:   ntpdb.ServerScoresStatusNew,
				reason:     "blocked by constraints or global status",
			})
		}
	}

	// Rule 2: Gradual removal of candidateOut monitors
	toRemove := sl.selectMonitorsForRemoval(activeMonitors, testingMonitors)
	for _, em := range toRemove {
		changes = append(changes, statusChange{
			monitorID:  em.monitor.ID,
			fromStatus: em.monitor.ServerStatus,
			toStatus:   ntpdb.ServerScoresStatusNew,
			reason:     "gradual removal (health or constraints)",
		})
	}

	// Rule 3: Promote from testing to active (respecting constraints)
	currentHealthyActive := healthyActive - len(toRemove)
	if currentHealthyActive < targetActiveMonitors {
		needed := targetActiveMonitors - currentHealthyActive
		toPromote := sl.selectMonitorsForPromotion(testingMonitors, needed)
		for _, em := range toPromote {
			if em.monitor.GlobalStatus == ntpdb.MonitorsStatusActive && em.recommendedState == candidateIn {
				changes = append(changes, statusChange{
					monitorID:  em.monitor.ID,
					fromStatus: ntpdb.ServerScoresStatusTesting,
					toStatus:   ntpdb.ServerScoresStatusActive,
					reason:     "promotion to active",
				})
			}
		}
	}

	// Rule 4: Add new monitors as candidates
	neededCandidates := sl.calculateNeededCandidates(len(activeMonitors), len(testingMonitors), len(candidateMonitors))
	if neededCandidates > 0 && len(availablePool) > 0 {
		toAdd := sl.selectMonitorsToAdd(availablePool, neededCandidates)
		for _, em := range toAdd {
			changes = append(changes, statusChange{
				monitorID:  em.monitor.ID,
				fromStatus: ntpdb.ServerScoresStatusNew,
				toStatus:   ntpdb.ServerScoresStatusCandidate,
				reason:     "new candidate",
			})
		}
	}

	// Rule 5: Promote candidates to testing
	// Ensure testing pool has at least 4 globally active monitors
	globallyActiveInTesting := sl.countGloballyActive(testingMonitors)
	neededTesting := targetTestingMonitors - len(testingMonitors)

	// Also ensure minimum globally active in testing
	if globallyActiveInTesting < minGloballyActiveInTesting {
		minNeeded := minGloballyActiveInTesting - globallyActiveInTesting
		if minNeeded > neededTesting {
			neededTesting = minNeeded
		}
	}

	if neededTesting > 0 && len(candidateMonitors) > 0 {
		toPromote := sl.selectCandidatesForTesting(candidateMonitors, neededTesting)
		for _, em := range toPromote {
			changes = append(changes, statusChange{
				monitorID:  em.monitor.ID,
				fromStatus: ntpdb.ServerScoresStatusCandidate,
				toStatus:   ntpdb.ServerScoresStatusTesting,
				reason:     "candidate to testing",
			})
		}
	}

	// Rule 6: Handle out-of-order situations (testing monitor should replace active)
	changes = sl.handleOutOfOrder(activeMonitors, testingMonitors, changes)

	return changes
}

// applyStatusChange executes a single status change
func (sl *Selector) applyStatusChange(
	ctx context.Context,
	db *ntpdb.Queries,
	serverID uint32,
	change statusChange,
	monitor *monitorCandidate,
) error {
	// Handle different transition types
	switch {
	case change.fromStatus == ntpdb.ServerScoresStatusNew &&
		change.toStatus == ntpdb.ServerScoresStatusCandidate:
		// Insert new server_score record with candidate status
		err := db.InsertServerScore(ctx, ntpdb.InsertServerScoreParams{
			MonitorID: change.monitorID,
			ServerID:  serverID,
			ScoreRaw:  0,
			CreatedOn: time.Now(),
		})
		if err != nil {
			return fmt.Errorf("failed to insert server score: %w", err)
		}
		err = db.UpdateServerScoreStatus(ctx, ntpdb.UpdateServerScoreStatusParams{
			MonitorID: change.monitorID,
			ServerID:  serverID,
			Status:    ntpdb.ServerScoresStatusCandidate,
		})
		if err != nil {
			return err
		}

	case change.toStatus == ntpdb.ServerScoresStatusNew:
		// Remove from server_scores
		// TODO: This needs a DeleteServerScore query to be added
		sl.log.Warn("DeleteServerScore not implemented yet",
			"serverID", serverID,
			"monitorID", change.monitorID)
		return nil

	default:
		// Update existing status
		err := db.UpdateServerScoreStatus(ctx, ntpdb.UpdateServerScoreStatusParams{
			MonitorID: change.monitorID,
			ServerID:  serverID,
			Status:    change.toStatus,
		})
		if err != nil {
			return err
		}
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

func (sl *Selector) selectMonitorsToAdd(
	available []evaluatedMonitor,
	count int,
) []evaluatedMonitor {
	var eligible []evaluatedMonitor

	// Filter to only eligible monitors
	for _, em := range available {
		if em.recommendedState == candidateIn {
			eligible = append(eligible, em)
		}
	}

	// TODO: Sort by preference (globally active first, then network diversity)

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
	// Target: enough to replace both active and testing pools
	targetCandidates := targetActiveMonitors + targetTestingMonitors
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
