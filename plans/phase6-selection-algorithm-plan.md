# Phase 6: Selection Algorithm Rewrite - Detailed Plan

## Overview

Phase 6 involves rewriting the main `processServer` logic to integrate all the constraint system components we've built in Phases 1-5. This will replace the current simplistic selection algorithm with a sophisticated system that respects constraints, handles grandfathering, and ensures proper state transitions.

## Current State Analysis

The existing `processServer` function in `selector.go` (lines 150-290) has several limitations:
1. No constraint validation (network, account limits)
2. Simple health-based selection without grandfathering
3. No concept of candidate status or gradual transitions
4. Limited to active/testing states only
5. No tracking of constraint violations

## Implementation Plan

### 1. Create `selector_process.go` File

This new file will contain the rewritten `processServer` logic that orchestrates all components:

```go
// selector_process.go

package cmd

import (
    "context"
    "fmt"
    "time"

    "go.ntppool.org/monitor/ntpdb"
)

// processServer implements the main selection algorithm for a single server
func (sl *selector) processServer(
    ctx context.Context,
    db *ntpdb.Queries,
    serverID uint32,
) (bool, error) {
    // Implementation will follow...
}
```

### 2. Core Algorithm Structure

The new `processServer` will follow these steps:

#### Step 1: Load Server Information
```go
// Get server details including IP and account
server, err := sl.loadServerInfo(ctx, db, serverID)
if err != nil {
    return false, fmt.Errorf("failed to load server info: %w", err)
}
```

#### Step 2: Get All Monitors
```go
// Get assigned monitors (with performance data)
assignedMonitors, err := db.GetMonitorPriority(ctx, serverID)
if err != nil {
    return false, fmt.Errorf("failed to get monitor priority: %w", err)
}

// Get available monitors (not assigned to this server)
availableMonitors, err := sl.findAvailableMonitors(ctx, db, serverID)
if err != nil {
    return false, fmt.Errorf("failed to find available monitors: %w", err)
}
```

#### Step 3: Build Account Limits
```go
// Build account limits from all monitors
accountLimits := sl.buildAccountLimitsFromMonitors(assignedMonitors)
```

#### Step 4: Evaluate All Monitors
```go
// Evaluate each monitor against constraints
evaluatedMonitors := make([]evaluatedMonitor, 0)

// Process assigned monitors
for _, row := range assignedMonitors {
    monitor := convertMonitorPriorityToCandidate(row)
    violation := sl.checkConstraints(&monitor, server, accountLimits, monitor.ServerStatus)

    if violation.Type != violationNone {
        violation.IsGrandfathered = sl.isGrandfathered(&monitor, server, violation)
    }

    state := sl.determineState(&monitor, violation)

    evaluatedMonitors = append(evaluatedMonitors, evaluatedMonitor{
        monitor:   monitor,
        violation: violation,
        state:     state,
    })
}

// Process available monitors
for _, monitor := range availableMonitors {
    // Check constraints for potential assignment
    targetState := ntpdb.ServerScoresStatusCandidate
    violation := sl.checkConstraints(&monitor, server, accountLimits, targetState)

    state := sl.determineState(&monitor, violation)

    evaluatedMonitors = append(evaluatedMonitors, evaluatedMonitor{
        monitor:   monitor,
        violation: violation,
        state:     state,
    })
}
```

#### Step 5: Apply Selection Rules
```go
// Categorize monitors by current status and recommended state
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
        if em.state != candidateBlock {
            availablePool = append(availablePool, em)
        }
    }
}

// Apply selection rules
changes := sl.applySelectionRules(
    activeMonitors,
    testingMonitors,
    candidateMonitors,
    availablePool,
)
```

#### Step 6: Execute Changes
```go
// Apply all status changes
for _, change := range changes {
    if err := sl.applyStatusChange(ctx, db, serverID, change); err != nil {
        sl.log.Error("failed to apply status change",
            "serverID", serverID,
            "monitorID", change.monitorID,
            "from", change.fromStatus,
            "to", change.toStatus,
            "error", err)
        // Continue with other changes
    }
}

// Track constraint violations
if err := sl.trackConstraintViolations(ctx, db, serverID, evaluatedMonitors); err != nil {
    sl.log.Error("failed to track constraint violations", "error", err)
}
```

### 3. Selection Rules Implementation

Create `applySelectionRules` method that implements the core logic:

```go
func (sl *selector) applySelectionRules(
    activeMonitors []evaluatedMonitor,
    testingMonitors []evaluatedMonitor,
    candidateMonitors []evaluatedMonitor,
    availablePool []evaluatedMonitor,
) []statusChange {
    changes := make([]statusChange, 0)

    // Count healthy monitors by state
    healthyActive := countHealthy(activeMonitors)
    healthyTesting := countHealthy(testingMonitors)

    // Rule 1: Remove monitors that should be blocked
    for _, em := range activeMonitors {
        if em.state == candidateBlock {
            changes = append(changes, statusChange{
                monitorID:  em.monitor.ID,
                fromStatus: ntpdb.ServerScoresStatusActive,
                toStatus:   ntpdb.ServerScoresStatusNew,
                reason:     "blocked by constraints",
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
            reason:     "gradual removal",
        })
    }

    // Rule 3: Promote from testing to active (respecting constraints)
    if healthyActive < targetActiveMonitors {
        toPromote := sl.selectMonitorsForPromotion(testingMonitors, targetActiveMonitors - healthyActive)
        for _, em := range toPromote {
            if em.monitor.GlobalStatus == ntpdb.MonitorsStatusActive {
                changes = append(changes, statusChange{
                    monitorID:  em.monitor.ID,
                    fromStatus: ntpdb.ServerScoresStatusTesting,
                    toStatus:   ntpdb.ServerScoresStatusActive,
                    reason:     "promotion to active",
                })
            }
        }
    }

    // Rule 4: Add new monitors to reach target capacity
    // First to candidate status
    neededCandidates := sl.calculateNeededCandidates(activeMonitors, testingMonitors, candidateMonitors)
    if neededCandidates > 0 {
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
    neededTesting := targetTestingMonitors - healthyTesting
    if neededTesting > 0 {
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

    // Rule 6: Handle out-of-order situations
    changes = sl.handleOutOfOrder(activeMonitors, testingMonitors, changes)

    return changes
}
```

### 4. Helper Methods

#### Finding Available Monitors
```go
func (sl *selector) findAvailableMonitors(
    ctx context.Context,
    db *ntpdb.Queries,
    serverID uint32,
) ([]monitorCandidate, error) {
    // Query for globally active/testing monitors not assigned to this server
    // This requires a new SQL query to be added
}
```

#### Loading Server Information
```go
func (sl *selector) loadServerInfo(
    ctx context.Context,
    db *ntpdb.Queries,
    serverID uint32,
) (*serverInfo, error) {
    // Load server details including IP and account
    // This might require extending an existing query or creating a new one
}
```

#### Selection Helpers
```go
func (sl *selector) selectMonitorsForRemoval(
    active []evaluatedMonitor,
    testing []evaluatedMonitor,
) []evaluatedMonitor {
    // Prioritize removal:
    // 1. candidateBlock monitors (immediate)
    // 2. candidateOut monitors (gradual)
    // 3. Oldest grandfathered violations first
}

func (sl *selector) selectMonitorsForPromotion(
    candidates []evaluatedMonitor,
    count int,
) []evaluatedMonitor {
    // Select best monitors for promotion:
    // 1. Must be candidateIn state
    // 2. Must be globally active
    // 3. Best health metrics
    // 4. Longest monitoring history
}

func (sl *selector) selectMonitorsToAdd(
    available []evaluatedMonitor,
    count int,
) []evaluatedMonitor {
    // Select from available pool:
    // 1. Must pass all constraints
    // 2. Prefer globally active over testing
    // 3. Prefer better network diversity
}
```

### 5. Status Change Execution

```go
func (sl *selector) applyStatusChange(
    ctx context.Context,
    db *ntpdb.Queries,
    serverID uint32,
    change statusChange,
) error {
    // Handle state transitions
    switch {
    case change.fromStatus == ntpdb.ServerScoresStatusNew &&
         change.toStatus == ntpdb.ServerScoresStatusCandidate:
        // Insert new server_score record
        return sl.insertNewCandidate(ctx, db, serverID, change.monitorID)

    case change.toStatus == ntpdb.ServerScoresStatusNew:
        // Remove from server_scores
        return db.DeleteServerScore(ctx, ntpdb.DeleteServerScoreParams{
            ServerID:  serverID,
            MonitorID: change.monitorID,
        })

    default:
        // Update existing status
        return db.UpdateServerScoreStatus(ctx, ntpdb.UpdateServerScoreStatusParams{
            ServerID:  serverID,
            MonitorID: change.monitorID,
            Status:    change.toStatus,
        })
    }
}
```

### 6. Logging and Observability

Add comprehensive logging throughout:

```go
// Log selection decisions
sl.log.Info("server selection complete",
    "serverID", serverID,
    "activeMonitors", len(activeMonitors),
    "testingMonitors", len(testingMonitors),
    "candidateMonitors", len(candidateMonitors),
    "changes", len(changes),
)

// Log each change
for _, change := range changes {
    sl.log.Debug("status change",
        "serverID", serverID,
        "monitorID", change.monitorID,
        "from", change.fromStatus,
        "to", change.toStatus,
        "reason", change.reason,
    )
}

// Log constraint violations
for _, em := range evaluatedMonitors {
    if em.violation.Type != violationNone {
        sl.log.Warn("constraint violation",
            "serverID", serverID,
            "monitorID", em.monitor.ID,
            "type", em.violation.Type,
            "grandfathered", em.violation.IsGrandfathered,
            "details", em.violation.Details,
        )
    }
}
```

## Testing Strategy

### Unit Tests
1. Test each helper method in isolation
2. Mock database queries
3. Test edge cases (no monitors, all blocked, etc.)
4. Test selection priority logic

### Integration Tests
1. Test full processServer flow
2. Test constraint enforcement
3. Test state transitions
4. Test grandfathering behavior

### Performance Tests
1. Benchmark with large monitor sets
2. Measure query performance
3. Test concurrent server processing

## SQL Query Requirements

We need to add or modify queries:

1. **GetAvailableMonitors** - Find globally active/testing monitors not assigned to server
2. **GetServerInfo** - Load server details including IP and account
3. **DeleteServerScore** - Remove monitor assignment
4. **Extend GetMonitorPriority** - Already done in Phase 5

## Migration Considerations

1. The new algorithm should produce similar results for healthy monitors
2. Gradual introduction via feature flags
3. Shadow mode to compare decisions
4. Ability to rollback quickly

## Success Criteria

1. All existing healthy monitors remain active
2. Constraint violations are properly detected
3. Grandfathering prevents sudden removals
4. Testing pool maintains 4+ globally active monitors
5. No performance regression
6. Clear audit trail of decisions

## Timeline

- Day 1: Implement core processServer structure and helper methods
- Day 2: Implement selection rules and state transitions
- Day 3: Add comprehensive logging and error handling
- Day 4: Write unit and integration tests

## Risks and Mitigations

1. **Risk**: Complex logic may have bugs
   **Mitigation**: Extensive testing, shadow mode deployment

2. **Risk**: Performance impact from additional queries
   **Mitigation**: Query optimization, caching where appropriate

3. **Risk**: Unexpected monitor removals
   **Mitigation**: Grandfathering system, gradual rollout

4. **Risk**: State transition errors
   **Mitigation**: Comprehensive state machine tests, transaction safety
