# Plan: Dynamic Testing Pool Size Based on Active Monitor Gap

## Problem Statement

The testing monitor pool size should dynamically adjust based on how many active monitors we're missing:
- **Base testing target**: 5
- **Dynamic adjustment**: Add the gap between target and actual active monitors
- **Formula**: `targetTesting = 5 + (targetActive - actualActive)`

### Example:
- Target active: 7, Actual active: 3
- Testing target: 5 + (7 - 3) = 9
- Current testing: 11 (should demote 2)

## Current Issue

The code uses a fixed `targetTestingMonitors = 5` constant, which doesn't account for the need to maintain a larger testing pool when we're below the active target.

## Proposed Solution

Replace the constant with dynamic calculation:

```go
// In applySelectionRules() after counting monitors:
baseTestingTarget := 5
activeGap := max(0, targetActiveMonitors - len(activeMonitors))
dynamicTestingTarget := baseTestingTarget + activeGap

// Then check if we have excess testing monitors:
if len(testingMonitors) > dynamicTestingTarget {
    // Demote excess testing monitors...
}
```

## Benefits

1. **Maintains promotion pipeline**: When active count is low, keeps more testing monitors ready
2. **Automatic adjustment**: As active monitors increase, testing pool naturally shrinks
3. **Prevents thrashing**: Avoids demoting monitors that might soon be needed for promotion

## Implementation Details

### Location: `selector/process.go`

1. Remove or rename constant `targetTestingMonitors = 5`
2. Calculate dynamic target in `applySelectionRules()`
3. Add demotion logic for excess testing monitors
4. Use performance-based selection (demote worst performers first)

### Full Logic:
```go
// After counting monitors and processing constraint-based demotions
baseTestingTarget := 5
activeGap := max(0, targetActiveMonitors - len(activeMonitors))
dynamicTestingTarget := baseTestingTarget + activeGap

testingCount := len(testingMonitors)
if testingCount > dynamicTestingTarget {
    excessTesting := testingCount - dynamicTestingTarget
    testingRemovalsRemaining := allowedChanges - len(changes)
    demotionsNeeded := min(excessTesting, testingRemovalsRemaining)

    // Demote worst-performing testing monitors
    demoted := 0
    for i := len(testingMonitors) - 1; i >= 0 && demoted < demotionsNeeded; i-- {
        em := testingMonitors[i]
        // Skip if already marked for demotion or has violations
        if em.recommendedState != candidateOut && em.currentViolation.Type == violationNone {
            changes = append(changes, statusChange{
                monitorID:  em.monitor.ID,
                fromStatus: ntpdb.ServerScoresStatusTesting,
                toStatus:   ntpdb.ServerScoresStatusCandidate,
                reason:     fmt.Sprintf("excess testing monitors (%d > target %d)",
                           testingCount, dynamicTestingTarget),
            })
            demoted++
            testingCount--
        }
    }
}
```

## Expected Results

For the example server with 3 active monitors:
- Dynamic testing target: 5 + (7-3) = 9
- Current testing: 11
- Would demote 2 worst-performing testing monitors to candidate

As active monitors increase toward 7:
- 4 active → testing target 8
- 5 active → testing target 7
- 6 active → testing target 6
- 7 active → testing target 5 (base level)

## Edge Cases

- Never demote testing monitors with constraint violations (they're already marked)
- Respect change limits per run
- Don't demote below base target even if active count exceeds target
- Account for pending promotions in the same run

## Implementation Status ✅ COMPLETED

**Implemented**: June 27, 2025 in commit `9f43eeb`

### Key Changes Made:
1. **Renamed constant**: `targetTestingMonitors` → `baseTestingTarget = 5`
2. **Added dynamic calculation**: `dynamicTestingTarget = baseTestingTarget + max(0, targetActive - actualActive)`
3. **Implemented Rule 2.5**: Excess testing monitor demotion logic
4. **Added comprehensive tests**: `TestDynamicTestingPoolSizing` with 5 test cases
5. **Per-status limits**: Enhanced with separate `testingRemovals` limit for better throughput

### Final Implementation:
```go
// Rule 2.5: Dynamic testing pool sizing
baseTestingTarget := 5
activeGap := max(0, targetActiveMonitors - len(activeMonitors))
dynamicTestingTarget := baseTestingTarget + activeGap

testingCount := len(testingMonitors)
if testingCount > dynamicTestingTarget {
    excessTesting := testingCount - dynamicTestingTarget
    // Use separate testing removal limits for independent processing
    dynamicTestingRemovalsRemaining := max(0, limits.testingRemovals - testingDemotionsSoFar)
    demotionsNeeded := min(excessTesting, dynamicTestingRemovalsRemaining)
    // ... demotion logic
}
```

### Test Results:
- ✅ All dynamic testing pool scenarios pass
- ✅ Works with per-status-group change limits
- ✅ Properly handles bootstrap mode (0 active monitors)
- ✅ Demotes worst performers first based on performance metrics
