package cmd

import (
	"encoding/json"
	"fmt"
	"net/netip"

	"go.ntppool.org/monitor/ntpdb"
)

// Hardcoded constraint values
const (
	defaultSubnetV4              = 24 // IPv4 subnet constraint (/24)
	defaultSubnetV6              = 48 // IPv6 subnet constraint (/48)
	defaultAccountLimitPerServer = 2  // Default max monitors per server per account
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
func (sl *selector) checkNetworkConstraint(
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

// checkAccountConstraints verifies account-based constraints
func (sl *selector) checkAccountConstraints(
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
func (sl *selector) checkConstraints(
	monitor *monitorCandidate,
	server *serverInfo,
	accountLimits map[uint32]*accountLimit,
	targetState ntpdb.ServerScoresStatus,
) *constraintViolation {
	// Check network constraint
	if err := sl.checkNetworkConstraint(monitor.IP, server.IP); err != nil {
		return &constraintViolation{
			Type:    violationNetwork,
			Details: err.Error(),
		}
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

		return &constraintViolation{
			Type:    violationType,
			Details: err.Error(),
		}
	}

	return &constraintViolation{
		Type: violationNone,
	}
}

// buildAccountLimitsFromMonitors builds account limits from the monitor priority results
func (sl *selector) buildAccountLimitsFromMonitors(monitors []ntpdb.GetMonitorPriorityRow) map[uint32]*accountLimit {
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
