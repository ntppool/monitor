# Process.go Refactoring Plan

## Overview

The `selector/process.go` file contains significant code duplication in promotion logic that can be refactored to improve maintainability, consistency, and testability.

## Current Issues

### 1. **Promotion Logic Duplication (Lines 376-437)**
Rule 5 has nearly identical code blocks for promoting globally active vs globally testing monitors with shared patterns:
- `canPromoteToTesting()` call with emergency override logic
- Emergency reason assignment
- Status change creation
- Account limits update
- Working counts update

### 2. **Bootstrap Promotion Duplication (Lines 510-563)**
Rule 6 repeats the same promotion pattern for healthy vs other candidates.

### 3. **Emergency Reason Logic Duplication**
The emergency override reason assignment pattern appears 6 times throughout the file:
```go
reason := "some base reason"
if emergencyOverride && canPromote {
    reason = "emergency promotion/bootstrap emergency promotion to testing/active: zero active monitors"
}
```

**Note**: Emergency reason for testing promotions should be "zero active monitors" not "zero testing monitors" - the emergency condition is triggered when there are zero active monitors.

## Proposed Refactoring

### 1. **Extract Promotion Helper Function**

Create a unified promotion attempt function:

```go
type promotionRequest struct {
    monitor           *monitorCandidate
    server            *serverInfo
    workingLimits     map[uint32]*accountLimit
    assignedMonitors  []ntpdb.GetMonitorPriorityRow
    emergencyOverride bool
    fromStatus        ntpdb.ServerScoresStatus
    toStatus          ntpdb.ServerScoresStatus
    baseReason        string
    emergencyReason   string
}

func (sl *Selector) attemptPromotion(req promotionRequest) (*statusChange, bool) {
    var canPromote bool

    switch req.toStatus {
    case ntpdb.ServerScoresStatusActive:
        canPromote = sl.canPromoteToActive(req.monitor, req.server, req.workingLimits, req.assignedMonitors, req.emergencyOverride)
    case ntpdb.ServerScoresStatusTesting:
        canPromote = sl.canPromoteToTesting(req.monitor, req.server, req.workingLimits, req.assignedMonitors, req.emergencyOverride)
    default:
        return nil, false
    }

    if !canPromote {
        return nil, false
    }

    reason := req.baseReason
    if req.emergencyOverride {
        reason = req.emergencyReason
    }

    change := statusChange{
        monitorID:  req.monitor.ID,
        fromStatus: req.fromStatus,
        toStatus:   req.toStatus,
        reason:     reason,
    }

    // Update working account limits
    sl.updateAccountLimitsForPromotion(req.workingLimits, req.monitor, req.fromStatus, req.toStatus)

    return &change, true
}
```

### 2. **Extract Emergency Reason Helper**

```go
func getEmergencyReason(baseReason string, toStatus ntpdb.ServerScoresStatus, isBootstrap bool) string {
    prefix := "emergency promotion"
    if isBootstrap {
        prefix = "bootstrap emergency promotion"
    }

    switch toStatus {
    case ntpdb.ServerScoresStatusActive:
        return prefix + ": zero active monitors"
    case ntpdb.ServerScoresStatusTesting:
        return prefix + " to testing: zero active monitors"
    default:
        return baseReason
    }
}
```

### 3. **Extract Monitor Filtering Logic**

```go
func filterMonitorsByGlobalStatus(monitors []evaluatedMonitor, status ntpdb.MonitorsStatus) []evaluatedMonitor {
    var filtered []evaluatedMonitor
    for _, em := range monitors {
        if em.monitor.GlobalStatus == status {
            filtered = append(filtered, em)
        }
    }
    return filtered
}

func filterBootstrapCandidates(monitors []evaluatedMonitor) (healthy, other []evaluatedMonitor) {
    for _, em := range monitors {
        isEligible := em.monitor.GlobalStatus == ntpdb.MonitorsStatusActive ||
                    em.monitor.GlobalStatus == ntpdb.MonitorsStatusTesting

        if !isEligible {
            continue
        }

        if em.monitor.IsHealthy {
            healthy = append(healthy, em)
        } else {
            other = append(other, em)
        }
    }
    return
}
```

### 4. **Refactored Rule 5 Example**

```go
// Rule 5: Promote candidates to testing
if changesRemaining > 0 && len(candidateMonitors) > 0 {
    // Calculate capacity and promotion needs
    activeGap := max(0, targetActiveMonitors-workingActiveCount)
    dynamicTestingTarget := baseTestingTarget + activeGap
    testingCapacity := max(0, dynamicTestingTarget-workingTestingCount)
    promotionsNeeded := min(min(changesRemaining, 2), testingCapacity)

    promoted := 0

    // Try globally active monitors first, then globally testing
    candidateGroups := []struct {
        monitors []evaluatedMonitor
        name     string
    }{
        {filterMonitorsByGlobalStatus(candidateMonitors, ntpdb.MonitorsStatusActive), "active"},
        {filterMonitorsByGlobalStatus(candidateMonitors, ntpdb.MonitorsStatusTesting), "testing"},
    }

    for _, group := range candidateGroups {
        for _, em := range group.monitors {
            if promoted >= promotionsNeeded {
                break
            }

            req := promotionRequest{
                monitor:           &em.monitor,
                server:            server,
                workingLimits:     workingAccountLimits,
                assignedMonitors:  assignedMonitors,
                emergencyOverride: emergencyOverride,
                fromStatus:        ntpdb.ServerScoresStatusCandidate,
                toStatus:          ntpdb.ServerScoresStatusTesting,
                baseReason:        "candidate to testing",
                emergencyReason:   getEmergencyReason("candidate to testing", ntpdb.ServerScoresStatusTesting, false),
            }

            if change, success := sl.attemptPromotion(req); success {
                changes = append(changes, *change)
                workingTestingCount++
                promoted++
            }
        }
    }
}
```

## Benefits

1. **Reduced Code Duplication**: ~200 lines could be reduced to ~100 lines
2. **Consistent Logic**: Emergency handling becomes uniform across all promotion types
3. **Easier Testing**: Helper functions can be unit tested independently
4. **Better Maintainability**: Changes to promotion logic only need to be made in one place
5. **Improved Readability**: Business logic becomes clearer without repetitive boilerplate

## Implementation Priority

1. **High Priority**: Extract promotion helper function (biggest impact)
2. **Medium Priority**: Extract emergency reason logic (consistency improvement)
3. **Low Priority**: Extract filtering helpers (minor cleanup)

## Emergency Reason Text Correction

Current emergency reasons should be:
- Testing promotions: "emergency promotion to testing: zero active monitors"
- Bootstrap testing promotions: "bootstrap emergency promotion to testing: zero active monitors"
- Active promotions: "emergency promotion: zero active monitors"

The emergency condition is always triggered by zero active monitors, not zero testing monitors.

## Implementation Strategy

### Phase 1: Extract Helper Functions
1. Create `promotionRequest` struct and `attemptPromotion` method
2. Create `getEmergencyReason` helper function
3. Add unit tests for helpers

### Phase 2: Refactor Rule 5
1. Replace duplicated promotion logic with helper calls
2. Verify integration tests still pass

### Phase 3: Refactor Rule 6 (Bootstrap)
1. Apply same pattern to bootstrap promotion logic
2. Extract bootstrap filtering logic

### Phase 4: Refactor Rule 3
1. Apply promotion helper to testing→active promotions
2. Ensure consistency across all promotion rules

## Testing Strategy

- Unit tests for all helper functions
- Integration tests to verify behavior is unchanged
- Specific tests for emergency override scenarios
- Performance tests to ensure no regression

## Future Considerations

After this refactoring, consider:
- Further extraction of common status change patterns
- Simplification of working count management
- Consolidation of change limit tracking
