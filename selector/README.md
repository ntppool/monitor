# Selector Package

The selector package implements the monitor selection algorithm for the NTP Pool monitoring system. It manages the process of selecting which monitors should actively monitor each NTP server in the pool.

## Overview

The selector evaluates NTP servers and assigns monitoring responsibilities based on:
- Server performance scores
- Network diversity constraints
- Account limits
- Monitor availability and health

## Key Components

### Selection Algorithm

The selector implements a state machine with the following states:
- **New**: Initial state for monitors not yet monitoring a server
- **Testing**: Transitional state for monitors being evaluated
- **Active**: Monitors actively monitoring the server
- **Inactive**: Monitors no longer monitoring the server

### Constraints

The selector enforces two primary constraints to ensure diversity:

1. **Network Constraint**: Maximum 4 monitors per /24 IPv4 subnet (or /48 IPv6)
2. **Account Constraint**: Maximum 2 monitors per account

These constraints ensure that monitoring is distributed across different networks and operators.

### Grandfathering

Existing monitor assignments that violate constraints are "grandfathered" - they can remain active as long as they maintain good performance scores. This prevents disruption to established monitoring relationships.

## Usage

### Command Line

```bash
# Run selector continuously
monitor-scorer selector server

# Run selector once
monitor-scorer selector once

# Dry run mode (no database changes)
monitor-scorer selector once --dry-run
```

### Programmatic

```go
import "go.ntppool.org/monitor/selector"

// Create a new selector
sel, err := selector.NewSelector(ctx, db, logger)
if err != nil {
    return err
}

// Run selection process
err = sel.Run()
```

## Selection Process

1. **Identify Servers**: Find servers that need monitor selection review
2. **Evaluate Candidates**: For each server, evaluate potential monitors based on:
   - Monitor health and availability
   - Current assignments
   - Constraint violations
   - Performance history
3. **State Transitions**: Move monitors through the state machine:
   - New → Testing (when score ≥ 10 for 30+ minutes)
   - Testing → Active (after successful testing period)
   - Active → Inactive (when performance degrades)
4. **Constraint Enforcement**: Block new assignments that would violate constraints
5. **Grandfathering**: Allow existing violations to continue if performing well

## Configuration

### Environment Variables

- `MONITOR_SCORER_DB`: Database connection string
- `MONITOR_SCORER_LOG_LEVEL`: Logging level (debug, info, warn, error)

### Database Tables

The selector primarily works with:
- `servers`: NTP server information
- `monitors`: Monitor information and status
- `server_scores`: Per-server, per-monitor scoring data
- `servers_monitor_review`: Queue for servers needing selection review
- `monitor_constraint_violations`: Tracking of constraint violations

## Testing

```bash
# Run unit tests
go test ./selector/...

# Run integration tests (requires test database)
go test ./selector/... -tags=integration

# Run load tests
go test ./selector/... -tags=load
```

## Performance Considerations

- The selector processes servers in batches to minimize database load
- Selection decisions are cached to avoid redundant constraint checks
- Database queries are optimized with appropriate indexes
- The process is designed to handle thousands of servers efficiently

## Future Enhancements

- Support for "pending" monitor state in selection algorithm
- Dynamic constraint adjustment based on monitor availability
- Geographic diversity constraints
- Performance-based automatic constraint relaxation
