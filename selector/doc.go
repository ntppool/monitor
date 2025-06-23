// Package selector implements the monitor selection algorithm for the NTP Pool.
//
// The selector is responsible for determining which monitors should actively
// monitor each NTP server in the pool. It enforces constraints to ensure
// monitoring diversity across networks and accounts while maintaining service
// quality.
//
// # Selection Algorithm
//
// The selector uses a state machine to manage monitor assignments:
//   - New: Initial state for unassigned monitors
//   - Testing: Evaluation period for new assignments
//   - Active: Actively monitoring the server
//   - Inactive: No longer monitoring the server
//
// # Constraints
//
// Two primary constraints ensure monitoring diversity:
//   - Network: Maximum 4 monitors per /24 IPv4 (or /48 IPv6) subnet
//   - Account: Maximum 2 monitors per account
//
// # Grandfathering
//
// Existing assignments that violate constraints can continue if they maintain
// good performance. This prevents disruption to established monitoring.
//
// # Usage
//
// Create a selector and run the selection process:
//
//	sel, err := selector.NewSelector(ctx, db, logger)
//	if err != nil {
//	    return err
//	}
//	err = sel.Run()
package selector
