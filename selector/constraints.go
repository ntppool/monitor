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
	MonitorLimit          int  `json:"monitor_limit"`            // Total monitors for account
	MonitorPerServerLimit int  `json:"monitor_per_server_limit"` // Max monitors per server
	MonitorEnabled        bool `json:"monitor_enabled"`
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
		if monitor.ServerStatus == ntpdb.ServerScoresStatusActive {
			activeCount--
		} else if monitor.ServerStatus == ntpdb.ServerScoresStatusTesting {
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
			// No limits on candidate status
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
			if monitor.AccountFlags != nil && len(monitor.AccountFlags) > 0 {
				if err := json.Unmarshal(monitor.AccountFlags, &flags); err != nil {
					sl.log.Warn("failed to parse account flags", "accountID", accountID, "error", err)
					flags.MonitorPerServerLimit = defaultAccountLimitPerServer
				}
			}

			limit := flags.MonitorPerServerLimit
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
) bool {
	// Must be globally active or testing
	if monitor.GlobalStatus != ntpdb.MonitorsStatusActive &&
		monitor.GlobalStatus != ntpdb.MonitorsStatusTesting {
		return false
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

// canTransitionTo checks if a monitor can transition to the target state without constraint violations
func (sl *Selector) canTransitionTo(
	monitor *monitorCandidate,
	server *serverInfo,
	accountLimits map[uint32]*accountLimit,
	targetState ntpdb.ServerScoresStatus,
	existingMonitors []ntpdb.GetMonitorPriorityRow,
) (bool, *constraintViolation) {
	violation := sl.checkConstraints(monitor, server, accountLimits, targetState, existingMonitors)

	if violation.Type == violationNone {
		return true, violation
	}

	// Check if this is a grandfathered violation
	if violation.IsGrandfathered = sl.isGrandfathered(monitor, server, violation); violation.IsGrandfathered {
		// Grandfathered violations can stay in current state but shouldn't be promoted
		return targetState == monitor.ServerStatus, violation
	}

	return false, violation
}

// parseAccountFlags parses the JSON flags column from accounts table
func parseAccountFlags(flagsJSON *string) (*accountFlags, error) {
	if flagsJSON == nil || *flagsJSON == "" {
		return &accountFlags{
			MonitorPerServerLimit: defaultAccountLimitPerServer,
			MonitorEnabled:        true,
		}, nil
	}

	var flags accountFlags
	if err := json.Unmarshal([]byte(*flagsJSON), &flags); err != nil {
		return nil, fmt.Errorf("failed to parse account flags: %w", err)
	}

	// Set defaults for missing values
	if flags.MonitorPerServerLimit <= 0 {
		flags.MonitorPerServerLimit = defaultAccountLimitPerServer
	}

	return &flags, nil
}
