# Selector Architecture Design

## Overview

The selector package implements the monitor selection algorithm for the NTP Pool monitoring system. It manages which monitors are assigned to which servers based on constraints, performance metrics, and availability using a sophisticated four-stage workflow with grandfathering support for existing assignments.

## Core Architecture

### State Machine Design

#### Global Monitor States (monitors.status)
- **pending** - Not approved for any monitoring (gradually phased out)
- **testing** - Approved, being evaluated globally
- **active** - Fully approved for monitoring
- **paused** - Temporarily disabled (stop all work immediately)
- **deleted** - Removed from system

#### Server-Monitor States (server_scores.status)
- **candidate** - Selected for potential assignment to server
- **testing** - Actively monitoring and being evaluated for server
- **active** - Confirmed for long-term monitoring of server

#### Internal Candidate States
```go
type candidateState uint8

const (
    candidateUnknown candidateState = iota
    candidateIn                    // Should be promoted/kept active
    candidateOut                   // Should be demoted (gradual)
    candidateBlock                 // Should be removed immediately
    candidatePending               // Should remain as candidate
)
```

### State Determination Hierarchy

The system follows a strict hierarchy when determining monitor state:

1. **Global Status Check** (primary filter)
   - pending → gradual removal (candidateOut)
   - paused → immediate removal (candidateBlock)
   - deleted → immediate removal (candidateBlock)
   - testing/active → continue to constraint checking

2. **State Consistency Check**
   - Detect inconsistent global vs server states
   - Mark inconsistencies for gradual removal

3. **Constraint Validation**
   - Apply network, account, and limit constraints
   - Grandfathered violations → gradual removal
   - New violations → immediate blocking or gradual removal

4. **Performance Evaluation**
   - Check health and performance metrics
   - Poor performance → gradual removal

5. **Promotion Eligibility**
   - Only globally active monitors can become server-active
   - Globally testing can stay in server-testing

## Constraint System

### Constraint Types

```go
type constraintViolationType string

const (
    violationNone        constraintViolationType = ""
    violationNetwork     constraintViolationType = "network"      // Same subnet
    violationAccount     constraintViolationType = "account"      // Same account
    violationLimit       constraintViolationType = "limit"        // Account limit exceeded
    violationDiversity   constraintViolationType = "diversity"    // Network diversity
)
```

### Hard vs Soft Constraints

**Hard Constraints (candidateBlock)** - Immediate removal:
- New violations on unassigned monitors
- Globally paused or deleted monitors
- Non-grandfathered violations on new assignments

**Soft Constraints (candidateOut)** - Gradual removal:
- Globally pending monitors (allow clean transitions)
- Grandfathered violations (existing assignments that violate new constraints)
- Performance/health issues
- State inconsistencies
- Account limit violations on existing assignments

### Network Constraints

- **IPv4**: /24 subnet constraint (hardcoded)
- **IPv6**: /48 subnet constraint (hardcoded)
- Monitor and server cannot be in the same subnet
- Uses `net/netip` for efficient IP operations

### Account Constraints

- **Same Account Rule**: Monitor and server cannot belong to same account
- **Per-Server Limits**: Configurable via `accounts.flags` JSON
  - Active monitors: max X per account per server
  - Testing monitors: max X+1 per account per server
  - Total active + testing: max X+1 per account per server
  - Candidates: no limit

### Network Diversity Constraints

- Multiple monitors from same /20 (IPv4) or /44 (IPv6) network
- Prevents over-concentration in single network blocks
- Applied iteratively to find worst performers when violations occur

## Grandfathering System

### Purpose
Maintains operational stability when constraints change by allowing existing assignments that violate new constraints to continue temporarily.

### Grandfathering Rules
- Only applies to existing active/testing assignments
- Network constraints cannot be grandfathered (hardcoded values)
- Account limit violations on existing assignments are grandfathered
- Same account violations cannot be grandfathered
- Grandfathered violations trigger gradual removal (candidateOut)

### Implementation
```go
func (sl *selector) isGrandfathered(
    monitor *monitorCandidate,
    server *serverInfo,
    violation *constraintViolation,
) bool {
    // Only grandfather existing active/testing assignments
    if monitor.ServerStatus != active && monitor.ServerStatus != testing {
        return false
    }

    // Network constraints are hardcoded, can't be grandfathered
    if violation.Type == violationNetwork {
        return false
    }

    // Account limit violations on existing assignments are grandfathered
    return violation.Type == violationLimit
}
```

## Selection Algorithm

### Monitor Categorization

The system categorizes all monitors based on their current state and constraint status:

- **Active**: Currently monitoring with server-active status
- **Testing**: Currently monitoring with server-testing status
- **Candidate**: Selected for server but not yet monitoring
- **Available**: Eligible monitors not assigned to this server
- **Blocked**: Monitors with constraint violations preventing assignment

### Selection Rules Engine

The selector applies rules in a specific order to maintain system stability:

1. **Rule 1 (Immediate Blocking)**: Remove monitors that should be blocked immediately
2. **Rule 2 (Gradual Constraint Removal)**: Gradual removal of candidateOut monitors
3. **Rule 1.5 (Active Excess Demotion)**: Demote excess healthy active monitors
4. **Rule 3 (Testing to Active Promotion)**: Promote from testing to active
5. **Rule 5 (Candidate to Testing Promotion)**: Promote candidates to testing and replace worse-performing testing monitors
6. **Rule 2.5 (Testing Pool Management)**: Demote excess testing monitors
7. **Rule 6 (Bootstrap Promotion)**: Bootstrap case promotions

### Helper Function Architecture

The system uses centralized helper functions to ensure consistent behavior:

```go
type promotionResult struct {
    promoted bool
    reason   string
}

// Unified promotion logic with count tracking
func attemptPromotion(monitor *Monitor, targetStatus Status,
    workingCounts *workingCounts, emergency bool) promotionResult

// Consistent emergency reason generation
func getEmergencyReason(targetStatus Status) string

// Monitor filtering by global status
func filterMonitorsByGlobalStatus(monitors []Monitor, status Status) []Monitor
```

### Emergency Override Hierarchy

The system implements a three-level safety hierarchy:

1. **Level 1**: Immediate safety (zero active monitors)
   - Allows constraint violations to proceed for system recovery
   - Uses `emergencyOverride := len(activeMonitors) == 0`

2. **Level 2**: Constraint violations
   - Can be overridden in emergencies
   - Normal operation blocks invalid promotions

3. **Level 3**: Capacity limits
   - Respected even in emergencies
   - Prevents over-promotion beyond targets

### Working Count Tracking

The system maintains mathematical consistency through working count tracking:

```go
type workingCounts struct {
    active  int
    testing int
}

// Update after each status change
func (wc *workingCounts) applyChange(change statusChange) {
    switch change.fromStatus {
    case active: wc.active--
    case testing: wc.testing--
    }

    switch change.toStatus {
    case active: wc.active++
    case testing: wc.testing++
    }
}
```

## Monitor Pool Management

### Target Counts
- **Active monitors**: 7 per server (configurable)
- **Testing monitors**: Dynamic based on active gap
  - Base: 5 monitors
  - Dynamic: +1 for each missing active monitor
  - Formula: `targetTesting = 5 + max(0, targetActive - actualActive)`

### Testing Pool Requirements
- Minimum 4 globally active monitors in testing pool
- Ensures sufficient promotion pipeline
- Bootstrap logic promotes candidates when requirements not met

### Change Limits

The system uses per-status-group change limits for independent processing:

```go
type changeLimits struct {
    activeRemovals  int // active → testing demotions
    testingRemovals int // testing → candidate demotions
    promotions      int // testing → active, candidate → testing
}
```

Base limits of 2 changes per group with increased limits when blocked monitors present.

## Database Schema Integration

### Core Tables

**server_scores**: Primary relationship table
- `status`: enum('new','candidate','testing','active')
- `constraint_violation_type`: varchar(50) for tracking violations
- `constraint_violation_since`: datetime for grandfathering

**monitors**: Global monitor management
- `status`: enum('pending','testing','active','paused','deleted')
- `account_id`: for account constraint checking
- `ip`: for network constraint checking

**accounts**: Account configuration
- `flags`: JSON column containing monitor limits
  ```json
  {
    "monitor_limit": 5,
    "monitor_per_server_limit": 2,
    "monitor_enabled": true
  }
  ```

### Key Queries

**GetMonitorPriority**: Returns all monitors with server_scores entries including global status, account info, and performance metrics

**GetAvailableMonitors**: Returns monitors eligible for assignment (planned for removal in architectural simplification)

## Metrics and Observability

### Status Change Tracking
- `selector_status_changes_total{monitor_id_token, from_status, to_status, reason}`
- Tracks all state transitions with context

### Constraint Violations
- `selector_constraint_violations_total{constraint_type, is_grandfathered}`
- `selector_grandfathered_violations{constraint_type}`

### Performance Metrics
- `selector_process_duration_seconds{server_id}`
- `selector_monitors_evaluated_total{server_id}`
- `selector_changes_applied_total{server_id}`

### Pool Health
- `selector_monitor_pool_size{status, server_id}`
- `selector_globally_active_monitors{server_id}`

## Architectural Patterns

### Self-Exclusion in Constraint Checks
Always exclude the entity being evaluated from conflict detection:
```go
if existing.ID == currentID { continue } // Skip self
```

### Iterative Constraint Checking
Process monitors sequentially to prevent simultaneous violations:
- Update working counts after each change
- Check constraints against updated state
- Prevent race conditions in limit enforcement

### State Context Evaluation
- Check constraints against **target state** for promotions
- Check constraints against **current state** for maintenance
- Don't check constraints when demoting (constraints may be why we're demoting)

### Lazy Constraint Evaluation
Only evaluate constraints when needed for decisions to optimize performance.

## Future Architectural Direction

### Planned Simplification: Eliminate "New" Status

The architecture is planned to be simplified by removing the conceptual "new" status:

**Current Issues**:
- Persistent constraint violation warnings for monitors with `serverStatus=new`
- Complex dual-state system with available pool evaluation
- False constraint violations blocking valid assignments

**Proposed Solution**:
- Rely entirely on `server_scores` entries managed by external API
- Selector only handles promotion/demotion of assigned monitors
- Eliminate "available pool" logic and `GetAvailableMonitors` query
- Check constraints only for existing assignments

This change will:
- Reduce code complexity by ~200+ lines
- Eliminate false constraint violations
- Clarify responsibility boundaries between assignment and selection
- Improve maintainability and performance

## Design Principles

### Global Status First
The system ALWAYS respects global monitor status as the primary filter:
1. Check `monitors.status` (pending/paused/deleted handling)
2. Check state consistency
3. Apply constraints
4. Evaluate performance
5. Make promotion decisions

### Operational Stability
- Gradual transitions prevent service disruptions
- Grandfathering system handles constraint changes safely
- Emergency overrides ensure system recovery capability
- Change limits prevent mass removal scenarios

### Mathematical Consistency
- Working counts track all state changes accurately
- Consistent promotion patterns across all rules
- Helper functions ensure uniform behavior
- Capacity checks prevent over-promotion

This architecture provides a robust, maintainable foundation for monitor selection while ensuring operational stability and system correctness.
