# Plan: Per-Status-Group Change Limits

## Problem Statement

The global `allowedChanges` limit was restricting the selector's efficiency by applying a single limit to all status changes within a server processing run. This caused bottlenecks where different status transition types competed for the same limited change budget.

### Example Issues:
- Dynamic testing pool sizing could only demote 1 monitor per run instead of 2
- Active demotions would consume the entire change budget, preventing testing promotions
- Conservative limits (base=1) were necessary because all changes shared the same pool

## Solution: Per-Status-Group Limits

Replace the global `allowedChanges` with separate limits for each status transition type:

```go
type changeLimits struct {
    activeRemovals  int // active → testing demotions
    testingRemovals int // testing → candidate demotions
    promotions      int // testing → active, candidate → testing
}
```

## Benefits

1. **Independent Processing**: Each status group can process changes up to its limit without competing
2. **Higher Throughput**: Can be more generous with limits since they don't compete (base=2 vs 1)
3. **Better Efficiency**: Dynamic testing pool sizing works as intended
4. **Maintained Safety**: Still prevents too many changes at once per status group

## Implementation Details

### Location: `selector/process.go`

### Key Changes:

1. **Add changeLimits struct** with separate limits per transition type
2. **Replace allowedChanges calculation** with `calculateChangeLimits()`
3. **Update all selection rules** to use appropriate per-status limits
4. **Increase base limits** from 1→2 since status groups no longer compete

### Limit Calculation:
```go
func calculateChangeLimits(currentActiveMonitors, blockedMonitors int) changeLimits {
    base := 2 // Increased from 1 for better throughput
    if blockedMonitors > 1 {
        base = 3 // Increased from 2
    }
    if currentActiveMonitors == 0 {
        base = 4 // bootstrap mode (unchanged)
    }

    return changeLimits{
        activeRemovals:  base,
        testingRemovals: base,
        promotions:      base,
    }
}
```

### Rule Updates:
- **Rule 2**: Use `limits.activeRemovals` for active→testing demotions
- **Rule 2 (testing)**: Use `limits.testingRemovals` for testing→candidate demotions
- **Rule 2.5**: Use `limits.testingRemovals` for dynamic testing pool demotions
- **Rule 3**: Use `limits.promotions` for testing→active promotions
- **Rule 5**: Use `limits.promotions` for candidate→testing promotions

## Implementation Status ✅ COMPLETED

**Implemented**: June 27, 2025 in commit `eb00c99`

### Key Changes Made:

1. **Added changeLimits struct** and `calculateChangeLimits()` function
2. **Updated all selection rules** to use appropriate per-status limits
3. **Increased base limits** to 2 for better efficiency
4. **Fixed dynamic testing demotions** to track previous testing removals
5. **Updated tests** to reflect new behavior (2 promotions vs 1)

### Test Results:
- ✅ `TestDynamicTestingPoolSizing`: Now properly demotes 2 testing monitors
- ✅ `TestIterativeAccountLimitEnforcement`: Updated to expect 2 promotions
- ✅ All other selector tests continue to pass
- ✅ Per-status limits working independently without competition

### Backward Compatibility:
- Maintained `allowedChanges` variable for logging compatibility
- Emergency override logic continues to work correctly
- All existing constraint checking patterns preserved

### Performance Improvements:
- Dynamic testing pool sizing now works as designed
- Better monitor status management throughput
- More efficient use of change budgets per processing cycle
