package selector

import (
	"encoding/json"
	"fmt"
	"net/netip"
	"time"

	"go.ntppool.org/monitor/ntpdb"
)

// Hardcoded constraint values
const (
	defaultSubnetV4              = 24 // IPv4 subnet constraint (/24)
	defaultSubnetV6              = 48 // IPv6 subnet constraint (/48)
	defaultAccountLimitPerServer = 2  // Default max monitors per server per account

	// Network diversity constraints
	diversitySubnetV4 = 20 // IPv4 network diversity constraint (/20)
	diversitySubnetV6 = 44 // IPv6 network diversity constraint (/44)
)

// accountFlags represents the JSON structure in accounts.flags column
type accountFlags struct {
	MonitorLimit           int  `json:"monitor_limit"`             // Total monitors for account
	MonitorsPerServerLimit int  `json:"monitors_per_server_limit"` // Max monitors per server
	MonitorEnabled         bool `json:"monitor_enabled"`
}

// accountLimit tracks monitor limits and current usage for an account
type accountLimit struct {
	AccountID    uint32
	MaxPerServer int
	ActiveCount  int // Current active monitors for this account on this server
	TestingCount int // Current testing monitors for this account on this server
}

// checkNetworkConstraint verifies that monitor and server are not in the same subnet
func (sl *Selector) checkNetworkConstraint(
	monitorIP string,
	serverIP string,
) error {
	if monitorIP == "" || serverIP == "" {
		return nil // Can't check without IPs
	}

	mAddr, err := netip.ParseAddr(monitorIP)
	if err != nil {
		return fmt.Errorf("invalid monitor IP: %w", err)
	}

	sAddr, err := netip.ParseAddr(serverIP)
	if err != nil {
		return fmt.Errorf("invalid server IP: %w", err)
	}

	// Must be same address family
	if mAddr.Is4() != sAddr.Is4() {
		return nil
	}

	var prefixLen int
	if mAddr.Is4() {
		prefixLen = defaultSubnetV4
	} else {
		prefixLen = defaultSubnetV6
	}

	// Check if in same subnet
	mPrefix, err := mAddr.Prefix(prefixLen)
	if err != nil {
		return fmt.Errorf("invalid prefix length %d: %w", prefixLen, err)
	}

	if mPrefix.Contains(sAddr) {
		return fmt.Errorf("monitor and server in same /%d network", prefixLen)
	}

	return nil
}

// checkNetworkDiversityConstraint verifies that we don't have multiple monitors
// in the same /44 (IPv6) or /20 (IPv4) network for active and testing states
func (sl *Selector) checkNetworkDiversityConstraint(
	monitorID uint32,
	monitorIP string,
	existingMonitors []ntpdb.GetMonitorPriorityRow,
	targetState ntpdb.ServerScoresStatus,
) error {
	if monitorIP == "" {
		return nil // Can't check without IP
	}

	candidateAddr, err := netip.ParseAddr(monitorIP)
	if err != nil {
		return fmt.Errorf("invalid monitor IP: %w", err)
	}

	// Determine diversity prefix length
	var diversityPrefixLen int
	if candidateAddr.Is4() {
		diversityPrefixLen = diversitySubnetV4
	} else {
		diversityPrefixLen = diversitySubnetV6
	}

	// Get candidate's diversity network
	candidatePrefix, err := candidateAddr.Prefix(diversityPrefixLen)
	if err != nil {
		return fmt.Errorf("invalid prefix length %d: %w", diversityPrefixLen, err)
	}

	// Check against existing monitors
	for _, existing := range existingMonitors {
		// Skip self
		if existing.ID == monitorID {
			continue
		}

		// Skip if no IP or status
		if !existing.MonitorIp.Valid || !existing.Status.Valid {
			continue
		}

		// Only check active and testing monitors
		existingStatus := existing.Status.ServerScoresStatus
		if existingStatus != ntpdb.ServerScoresStatusActive &&
			existingStatus != ntpdb.ServerScoresStatusTesting {
			continue
		}

		// Parse existing monitor IP
		existingAddr, err := netip.ParseAddr(existing.MonitorIp.String)
		if err != nil {
			continue // Skip invalid IPs
		}

		// Must be same address family
		if candidateAddr.Is4() != existingAddr.Is4() {
			continue
		}

		// Check if in same diversity network
		if candidatePrefix.Contains(existingAddr) {
			// Special handling: if we're trying to promote to testing and there's already
			// an active monitor in this network, or if we're trying to promote to active
			// and there's already a testing or active monitor, it's a violation
			if (targetState == ntpdb.ServerScoresStatusTesting && existingStatus == ntpdb.ServerScoresStatusActive) ||
				(targetState == ntpdb.ServerScoresStatusActive && (existingStatus == ntpdb.ServerScoresStatusActive || existingStatus == ntpdb.ServerScoresStatusTesting)) ||
				(targetState == ntpdb.ServerScoresStatusTesting && existingStatus == ntpdb.ServerScoresStatusTesting) {
				return fmt.Errorf("monitor would conflict with existing %s monitor in same /%d network (%s)",
					existingStatus, diversityPrefixLen, candidatePrefix.String())
			}
		}
	}

	return nil
}

// checkAccountConstraints verifies account-based constraints
func (sl *Selector) checkAccountConstraints(
	monitor *monitorCandidate,
	server *serverInfo,
	accountLimits map[uint32]*accountLimit,
	targetState ntpdb.ServerScoresStatus, // What state are we trying to move to?
) error {
	// Check same account constraint
	if monitor.AccountID != nil && server.AccountID != nil {
		if *monitor.AccountID == *server.AccountID {
			return fmt.Errorf("monitor from same account as server")
		}
	}

	// Check account limits (per server, per state)
	if monitor.AccountID != nil {
		limit, exists := accountLimits[*monitor.AccountID]
		if !exists {
			// No specific limit loaded, this shouldn't happen
			// as loadAccountLimits should have populated all accounts
			return fmt.Errorf("account limit not loaded for account %d", *monitor.AccountID)
		}

		// State-based limits:
		// - active = X
		// - testing = X+1
		// - active+testing = X+1
		// - no limit on candidates

		maxActive := limit.MaxPerServer
		maxTesting := limit.MaxPerServer + 1
		maxTotal := limit.MaxPerServer + 1

		// Get current counts (excluding this monitor if already assigned)
		activeCount := limit.ActiveCount
		testingCount := limit.TestingCount

		// Don't count self
		switch monitor.ServerStatus {
		case ntpdb.ServerScoresStatusActive:
			activeCount--
		case ntpdb.ServerScoresStatusTesting:
			testingCount--
		}
		// Note: We don't track/limit candidate counts

		// Check limits based on target state
		switch targetState {
		case ntpdb.ServerScoresStatusActive:
			if activeCount >= maxActive {
				return fmt.Errorf("account %d at active limit (%d/%d)",
					limit.AccountID, activeCount, maxActive)
			}
			// Also check total limit
			if activeCount+testingCount >= maxTotal {
				return fmt.Errorf("account %d at total limit (%d/%d)",
					limit.AccountID, activeCount+testingCount, maxTotal)
			}

		case ntpdb.ServerScoresStatusTesting:
			if testingCount >= maxTesting {
				return fmt.Errorf("account %d at testing limit (%d/%d)",
					limit.AccountID, testingCount, maxTesting)
			}
			// Also check total limit
			if activeCount+testingCount >= maxTotal {
				return fmt.Errorf("account %d at total limit (%d/%d)",
					limit.AccountID, activeCount+testingCount, maxTotal)
			}

		case ntpdb.ServerScoresStatusCandidate:
			// No limits on candidate status - candidates are unconstrained by design
			// This allows monitors to be selected as candidates without worrying about limits,
			// and constraints are only checked when promoting to testing/active
			return nil

		default:
			// For other states (new), no limit checks needed
			return nil
		}
	}

	return nil
}

// checkConstraints performs all constraint validation
func (sl *Selector) checkConstraints(
	monitor *monitorCandidate,
	server *serverInfo,
	accountLimits map[uint32]*accountLimit,
	targetState ntpdb.ServerScoresStatus,
	existingMonitors []ntpdb.GetMonitorPriorityRow,
) *constraintViolation {
	// Check network constraint
	if err := sl.checkNetworkConstraint(monitor.IP, server.IP); err != nil {
		violation := &constraintViolation{
			Type:    violationNetworkSameSubnet,
			Details: err.Error(),
		}
		// If we have a stored violation of the same type, preserve the timestamp
		if monitor.ConstraintViolationType != nil &&
			*monitor.ConstraintViolationType == string(violationNetworkSameSubnet) &&
			monitor.ConstraintViolationSince != nil {
			violation.Since = *monitor.ConstraintViolationSince
		} else {
			violation.Since = time.Now()
		}

		// Track constraint violation in metrics
		if sl.metrics != nil {
			sl.metrics.TrackConstraintViolation(monitor, violation.Type, server.ID, false)
		}

		return violation
	}

	// Check account constraints
	if err := sl.checkAccountConstraints(monitor, server, accountLimits, targetState); err != nil {
		// Determine specific violation type
		violationType := violationAccount // Same account
		if monitor.AccountID != nil {
			if _, exists := accountLimits[*monitor.AccountID]; exists {
				// If we have limit info and still failed, it's a limit violation
				violationType = violationLimit
			}
		}

		violation := &constraintViolation{
			Type:    violationType,
			Details: err.Error(),
		}
		// If we have a stored violation of the same type, preserve the timestamp
		if monitor.ConstraintViolationType != nil &&
			*monitor.ConstraintViolationType == string(violationType) &&
			monitor.ConstraintViolationSince != nil {
			violation.Since = *monitor.ConstraintViolationSince
		} else {
			violation.Since = time.Now()
		}

		// Track constraint violation in metrics
		if sl.metrics != nil {
			sl.metrics.TrackConstraintViolation(monitor, violation.Type, server.ID, false)
		}

		return violation
	}

	// Check network diversity constraints
	if err := sl.checkNetworkDiversityConstraint(monitor.ID, monitor.IP, existingMonitors, targetState); err != nil {
		violation := &constraintViolation{
			Type:    violationNetworkDiversity,
			Details: err.Error(),
		}
		// If we have a stored violation of the same type, preserve the timestamp
		if monitor.ConstraintViolationType != nil &&
			*monitor.ConstraintViolationType == string(violationNetworkDiversity) &&
			monitor.ConstraintViolationSince != nil {
			violation.Since = *monitor.ConstraintViolationSince
		} else {
			violation.Since = time.Now()
		}

		// Track constraint violation in metrics
		if sl.metrics != nil {
			sl.metrics.TrackConstraintViolation(monitor, violation.Type, server.ID, false)
		}

		return violation
	}

	return &constraintViolation{
		Type: violationNone,
	}
}

// buildAccountLimitsFromMonitors builds account limits from the monitor priority results
func (sl *Selector) buildAccountLimitsFromMonitors(monitors []ntpdb.GetMonitorPriorityRow) map[uint32]*accountLimit {
	limits := make(map[uint32]*accountLimit)

	for _, monitor := range monitors {
		// Skip monitors without accounts
		if !monitor.AccountID.Valid {
			continue
		}

		accountID := uint32(monitor.AccountID.Int32)

		// Initialize account limit if not seen before
		if _, exists := limits[accountID]; !exists {
			// Parse account flags to get limit
			var flags accountFlags
			if len(monitor.AccountFlags) > 0 {
				if err := json.Unmarshal(monitor.AccountFlags, &flags); err != nil {
					sl.log.Warn("failed to parse account flags", "accountID", accountID, "error", err)
					flags.MonitorsPerServerLimit = defaultAccountLimitPerServer
				}
			}

			limit := flags.MonitorsPerServerLimit
			if limit <= 0 {
				limit = defaultAccountLimitPerServer
			}

			limits[accountID] = &accountLimit{
				AccountID:    accountID,
				MaxPerServer: limit,
				ActiveCount:  0,
				TestingCount: 0,
			}
		}

		// Count active/testing monitors
		if monitor.Status.Valid {
			switch monitor.Status.ServerScoresStatus {
			case ntpdb.ServerScoresStatusActive:
				limits[accountID].ActiveCount++
			case ntpdb.ServerScoresStatusTesting:
				limits[accountID].TestingCount++
			}
		}
	}

	return limits
}

// checkAccountConstraintsIterative performs iterative per-category account constraint checking
// This determines which specific monitors exceed the account limits by sorting them by priority
// and only flagging the worst-performing monitors that exceed the per-category limits.
func (sl *Selector) checkAccountConstraintsIterative(
	monitors []ntpdb.GetMonitorPriorityRow,
	server *serverInfo,
) map[uint32]*constraintViolation {
	violations := make(map[uint32]*constraintViolation)

	// Group monitors by account and category
	type monitorInfo struct {
		row      ntpdb.GetMonitorPriorityRow
		priority float64
	}

	accountGroups := make(map[uint32]map[ntpdb.ServerScoresStatus][]monitorInfo)

	for _, monitor := range monitors {
		// Skip monitors without accounts or status
		if !monitor.AccountID.Valid || !monitor.Status.Valid {
			continue
		}

		accountID := uint32(monitor.AccountID.Int32)
		status := monitor.Status.ServerScoresStatus

		// Only check active and testing monitors (candidates are exempt)
		if status != ntpdb.ServerScoresStatusActive && status != ntpdb.ServerScoresStatusTesting {
			continue
		}

		// Initialize account group if needed
		if accountGroups[accountID] == nil {
			accountGroups[accountID] = make(map[ntpdb.ServerScoresStatus][]monitorInfo)
		}

		// Add monitor to appropriate category
		accountGroups[accountID][status] = append(accountGroups[accountID][status], monitorInfo{
			row:      monitor,
			priority: monitor.MonitorPriority,
		})
	}

	// Check each account's limits per category
	for accountID, statusGroups := range accountGroups {
		// Get account limit from any monitor in this account
		var accountLimit int = defaultAccountLimitPerServer

		// Find a monitor from this account to get the flags
		for _, statusList := range statusGroups {
			if len(statusList) > 0 {
				monitor := statusList[0].row
				if monitor.AccountID.Valid && uint32(monitor.AccountID.Int32) == accountID && len(monitor.AccountFlags) > 0 {
					var flags accountFlags
					if err := json.Unmarshal(monitor.AccountFlags, &flags); err == nil && flags.MonitorsPerServerLimit > 0 {
						accountLimit = flags.MonitorsPerServerLimit
					}
				}
				break
			}
		}

		// Check each category separately
		for status, monitorList := range statusGroups {
			var categoryLimit int
			switch status {
			case ntpdb.ServerScoresStatusActive:
				categoryLimit = accountLimit // Max X active monitors
			case ntpdb.ServerScoresStatusTesting:
				categoryLimit = accountLimit + 1 // Max X+1 testing monitors
			default:
				continue // Skip other statuses
			}

			// If we're over the limit, flag the worst performers
			if len(monitorList) > categoryLimit {
				// Sort by priority (worst priority = highest number = worst performance)
				// We want to flag the worst performers first
				for i := 0; i < len(monitorList)-1; i++ {
					for j := i + 1; j < len(monitorList); j++ {
						if monitorList[i].priority < monitorList[j].priority {
							monitorList[i], monitorList[j] = monitorList[j], monitorList[i]
						}
					}
				}

				// Flag the excess monitors (worst performers)
				excess := len(monitorList) - categoryLimit
				for i := 0; i < excess; i++ {
					monitorID := monitorList[i].row.ID

					violation := &constraintViolation{
						Type:    violationLimit,
						Details: fmt.Sprintf("account %d exceeds %s limit (%d/%d)", accountID, status, len(monitorList), categoryLimit),
					}

					// Preserve existing violation timestamp if it exists
					if monitorList[i].row.ConstraintViolationType.Valid &&
						monitorList[i].row.ConstraintViolationType.String == string(violationLimit) &&
						monitorList[i].row.ConstraintViolationSince.Valid {
						violation.Since = monitorList[i].row.ConstraintViolationSince.Time
					} else {
						violation.Since = time.Now()
					}

					violations[monitorID] = violation
				}
			}
		}
	}

	return violations
}

// checkNonAccountConstraints performs constraint validation excluding account limit checks
// This is used in conjunction with checkAccountConstraintsIterative to avoid double-checking account limits
func (sl *Selector) checkNonAccountConstraints(
	monitor *monitorCandidate,
	server *serverInfo,
	existingMonitors []ntpdb.GetMonitorPriorityRow,
	targetState ntpdb.ServerScoresStatus,
) *constraintViolation {
	// Check network constraint (same subnet)
	if err := sl.checkNetworkConstraint(monitor.IP, server.IP); err != nil {
		violation := &constraintViolation{
			Type:    violationNetworkSameSubnet,
			Details: err.Error(),
		}
		// If we have a stored violation of the same type, preserve the timestamp
		if monitor.ConstraintViolationType != nil &&
			*monitor.ConstraintViolationType == string(violationNetworkSameSubnet) &&
			monitor.ConstraintViolationSince != nil {
			violation.Since = *monitor.ConstraintViolationSince
		} else {
			violation.Since = time.Now()
		}

		// Track constraint violation in metrics
		if sl.metrics != nil {
			sl.metrics.TrackConstraintViolation(monitor, violation.Type, server.ID, false)
		}

		return violation
	}

	// Check same account constraint (not account limits)
	if monitor.AccountID != nil && server.AccountID != nil {
		if *monitor.AccountID == *server.AccountID {
			violation := &constraintViolation{
				Type:    violationAccount,
				Details: "monitor from same account as server",
			}
			// If we have a stored violation of the same type, preserve the timestamp
			if monitor.ConstraintViolationType != nil &&
				*monitor.ConstraintViolationType == string(violationAccount) &&
				monitor.ConstraintViolationSince != nil {
				violation.Since = *monitor.ConstraintViolationSince
			} else {
				violation.Since = time.Now()
			}

			// Track constraint violation in metrics
			if sl.metrics != nil {
				sl.metrics.TrackConstraintViolation(monitor, violation.Type, server.ID, false)
			}

			return violation
		}
	}

	// Check network diversity constraints
	if err := sl.checkNetworkDiversityConstraint(monitor.ID, monitor.IP, existingMonitors, targetState); err != nil {
		violation := &constraintViolation{
			Type:    violationNetworkDiversity,
			Details: err.Error(),
		}
		// If we have a stored violation of the same type, preserve the timestamp
		if monitor.ConstraintViolationType != nil &&
			*monitor.ConstraintViolationType == string(violationNetworkDiversity) &&
			monitor.ConstraintViolationSince != nil {
			violation.Since = *monitor.ConstraintViolationSince
		} else {
			violation.Since = time.Now()
		}

		// Track constraint violation in metrics
		if sl.metrics != nil {
			sl.metrics.TrackConstraintViolation(monitor, violation.Type, server.ID, false)
		}

		return violation
	}

	return &constraintViolation{
		Type: violationNone,
	}
}

// canPromoteToActive checks if a monitor can be promoted to active status
func (sl *Selector) canPromoteToActive(
	monitor *monitorCandidate,
	server *serverInfo,
	accountLimits map[uint32]*accountLimit,
	existingMonitors []ntpdb.GetMonitorPriorityRow,
	emergencyOverride bool,
) bool {
	// Must be globally active to be promoted to server-active
	if monitor.GlobalStatus != ntpdb.MonitorsStatusActive {
		return false
	}

	// Check if healthy (if we have metrics)
	if monitor.HasMetrics && !monitor.IsHealthy {
		return false
	}

	// In emergency mode, skip constraint checking
	if emergencyOverride {
		return true
	}

	// Check constraints against active state specifically
	violation := sl.checkConstraints(monitor, server, accountLimits, ntpdb.ServerScoresStatusActive, existingMonitors)
	return violation.Type == violationNone || violation.IsGrandfathered
}

// canPromoteToTesting checks if a monitor can be promoted to testing status
func (sl *Selector) canPromoteToTesting(
	monitor *monitorCandidate,
	server *serverInfo,
	accountLimits map[uint32]*accountLimit,
	existingMonitors []ntpdb.GetMonitorPriorityRow,
	emergencyOverride bool,
) bool {
	// Must be globally active or testing
	if monitor.GlobalStatus != ntpdb.MonitorsStatusActive &&
		monitor.GlobalStatus != ntpdb.MonitorsStatusTesting {
		return false
	}

	// In emergency mode, skip constraint checking
	if emergencyOverride {
		return true
	}

	// Check constraints against testing state specifically
	violation := sl.checkConstraints(monitor, server, accountLimits, ntpdb.ServerScoresStatusTesting, existingMonitors)
	return violation.Type == violationNone || violation.IsGrandfathered
}

// updateAccountLimitsForPromotion updates account limits after a monitor promotion
func (sl *Selector) updateAccountLimitsForPromotion(
	accountLimits map[uint32]*accountLimit,
	monitor *monitorCandidate,
	fromState, toState ntpdb.ServerScoresStatus,
) {
	if monitor.AccountID == nil {
		return
	}

	limit, exists := accountLimits[*monitor.AccountID]
	if !exists {
		return
	}

	// Remove from old state count
	switch fromState {
	case ntpdb.ServerScoresStatusActive:
		if limit.ActiveCount > 0 {
			limit.ActiveCount--
		}
	case ntpdb.ServerScoresStatusTesting:
		if limit.TestingCount > 0 {
			limit.TestingCount--
		}
	}

	// Add to new state count
	switch toState {
	case ntpdb.ServerScoresStatusActive:
		limit.ActiveCount++
	case ntpdb.ServerScoresStatusTesting:
		limit.TestingCount++
	}
}
