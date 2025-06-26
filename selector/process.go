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

	// Count healthy monitors and blocked monitors (for change limit calculation)
	healthyActive := sl.countHealthy(activeMonitors)
	healthyTesting := sl.countHealthy(testingMonitors)
	blockedMonitors := sl.countBlocked(evaluatedMonitors)

	// Calculate change limits based on old selector logic
	targetNumber := targetActiveMonitors         // 7 from constants
	bootStrapModeLimit := (targetNumber / 2) + 1 // 4
	currentActiveMonitors := len(activeMonitors)

	allowedChanges := 1
	if blockedMonitors > 1 {
		allowedChanges = 2
	}
	if currentActiveMonitors == 0 {
		allowedChanges = bootStrapModeLimit
	}

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
	if currentActiveMonitors <= targetNumber && healthyActive < targetNumber {
		maxRemovals = 0
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
		"available", len(availablePool),
		"blocked", blockedMonitors,
		"allowedChanges", allowedChanges,
		"maxRemovals", maxRemovals)

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

	// Rule 2: Gradual removal of candidateOut monitors (with limits)
	removalsRemaining := maxRemovals

	// First remove active monitors (demote to testing, not new)
	for _, em := range activeMonitors {
		if removalsRemaining <= 0 {
			break
		}
		if em.recommendedState == candidateOut {
			changes = append(changes, statusChange{
				monitorID:  em.monitor.ID,
				fromStatus: ntpdb.ServerScoresStatusActive,
				toStatus:   ntpdb.ServerScoresStatusTesting,
				reason:     "gradual removal (health or constraints)",
			})
			removalsRemaining--
		}
	}

	// Then remove testing monitors (demote to candidate, not new)
	for _, em := range testingMonitors {
		if removalsRemaining <= 0 {
			break
		}
		if em.recommendedState == candidateOut {
			changes = append(changes, statusChange{
				monitorID:  em.monitor.ID,
				fromStatus: ntpdb.ServerScoresStatusTesting,
				toStatus:   ntpdb.ServerScoresStatusCandidate,
				reason:     "gradual removal (health or constraints)",
			})
			removalsRemaining--
		}
	}

	// Rule 3: Promote from testing to active (respecting constraints and limits)
	changesRemaining := allowedChanges - len(changes)
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

		for _, em := range testingMonitors {
			if promoted >= promotionsNeeded {
				break
			}
			if em.monitor.GlobalStatus == ntpdb.MonitorsStatusActive && em.recommendedState == candidateIn {
				changes = append(changes, statusChange{
					monitorID:  em.monitor.ID,
					fromStatus: ntpdb.ServerScoresStatusTesting,
					toStatus:   ntpdb.ServerScoresStatusActive,
					reason:     "promotion to active",
				})
				promoted++
			}
		}
	}

	// Rule 4: Add new monitors as candidates (respecting change limits)
	changesRemaining = allowedChanges - len(changes)
	if changesRemaining > 0 && len(availablePool) > 0 {
		candidatesAdded := 0

		for _, em := range availablePool {
			if candidatesAdded >= changesRemaining {
				break
			}
			if em.recommendedState == candidateIn {
				changes = append(changes, statusChange{
					monitorID:  em.monitor.ID,
					fromStatus: ntpdb.ServerScoresStatusNew,
					toStatus:   ntpdb.ServerScoresStatusCandidate,
					reason:     "new candidate",
				})
				candidatesAdded++
			}
		}
	}

	// Rule 5: Promote candidates to testing (respecting change limits)
	changesRemaining = allowedChanges - len(changes)
	if changesRemaining > 0 && len(candidateMonitors) > 0 {
		promotionsNeeded := min(changesRemaining, 2) // Limit candidate promotions
		promoted := 0

		// Prefer globally active monitors
		for _, em := range candidateMonitors {
			if promoted >= promotionsNeeded {
				break
			}
			if em.recommendedState == candidateIn && em.monitor.GlobalStatus == ntpdb.MonitorsStatusActive {
				changes = append(changes, statusChange{
					monitorID:  em.monitor.ID,
					fromStatus: ntpdb.ServerScoresStatusCandidate,
					toStatus:   ntpdb.ServerScoresStatusTesting,
					reason:     "candidate to testing",
				})
				promoted++
			}
		}

		// Fill remaining with testing monitors if needed
		for _, em := range candidateMonitors {
			if promoted >= promotionsNeeded {
				break
			}
			if em.recommendedState == candidateIn && em.monitor.GlobalStatus == ntpdb.MonitorsStatusTesting {
				changes = append(changes, statusChange{
					monitorID:  em.monitor.ID,
					fromStatus: ntpdb.ServerScoresStatusCandidate,
					toStatus:   ntpdb.ServerScoresStatusTesting,
					reason:     "candidate to testing",
				})
				promoted++
			}
		}
	}

	// Rule 6: Handle out-of-order situations (disabled for now to respect change limits)
	// TODO: Implement out-of-order logic that respects allowedChanges limits

	sl.log.Debug("planned changes", "totalChanges", len(changes), "allowedChanges", allowedChanges)

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
		err := db.DeleteServerScore(ctx, ntpdb.DeleteServerScoreParams{
			ServerID:  serverID,
			MonitorID: change.monitorID,
		})
		if err != nil {
			return fmt.Errorf("failed to delete server score: %w", err)
		}

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
