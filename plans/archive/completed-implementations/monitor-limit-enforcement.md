# Selector Monitor Limit Enforcement Implementation Plan

## Status: FULLY COMPLETED (July 2025)

**Implementation Summary:**
- ✅ Phase 1: Math fix + Rule 1.5 (commits: 8902a05, 6e5b4e8, 5d4403a)
- ✅ Fixed working count tracking and rule execution order
- ✅ Added testing capacity limits to Rule 5
- ✅ Phase 2: Testing pool investigation completed - Rule 5 over-promotion issue fixed
- ✅ Phase 3: Enhanced tracking implemented with helper function centralization (commit: 6c4ae72)
- ✅ Safety logic improvements completed (commits: e04e47a, b6515b8, 6035139)

**Additional Improvements Beyond Original Plan:**
- ✅ **Helper Function Centralization**: 47% code reduction through promotion pattern extraction
- ✅ **Safety Logic Refinement**: Fixed overly aggressive emergency conditions
- ✅ **Emergency Override Unification**: Consistent emergency handling across all promotion types
- ✅ **Response Selection Priority Fix**: Corrected preference for valid responses over timeouts

## Problem Summary

Server 1982 demonstrates critical selector bugs that allow monitor counts to exceed targets:
- **Active monitors**: 9 (target: 7) - excess of 2
- **Testing monitors**: 23 (target: 5) - excess of 18
- **Root cause**: Mathematical bugs in promotion logic + missing demotion rules

## Current Architecture Analysis

### Existing Rules
- **Rule 1**: Demote unhealthy active monitors ✓
- **Rule 2**: Demote unhealthy testing monitors ✓
- **Rule 2.5**: Demote excess testing monitors (dynamic pool sizing) ⚠️
- **Rule 3**: Promote testing→active (🐛 buggy math)
- **Rule 5**: Promote candidate→testing (different logic)

### Missing Rules
- **Rule 1.5**: Demote excess healthy active monitors ❌

## Critical Gaps Addressed (Based on Feedback)

### ❌ Identified Gaps in Original Plan
1. **Emergency Override Integration Risk**: Original plan didn't detail how Rule 1.5 interacts with emergency override
2. **Change Limits Interaction Complexity**: Risk of exhausting demotion budget on excess handling vs constraint violations
3. **Bootstrap Scenario Handling**: Missing logic for excess active monitors during bootstrap scenarios
4. **Cascading Effects**: Didn't address what happens when active→testing demotions overflow testing pool

### ✅ Resolutions in Updated Plan
1. **Emergency Override Safety**: Added explicit checks `!emergencyOverride && workingActiveCount > 1`
2. **Budget Prioritization**: Reserve demotion budget for constraint violations before excess handling
3. **Bootstrap Protection**: Check testing pool capacity and use cascading demotions when needed
4. **Cascading Demotion Logic**: Implement testing→candidate demotions when testing pool overflows

## Root Cause Analysis

### 1. Active Promotion Math Bug (`selector/process.go:281-288`)

**Current Buggy Code:**
```go
toAdd := targetNumber - currentActiveMonitors  // 7 - 9 = -2
for _, change := range changes {
    if change.fromStatus == ntpdb.ServerScoresStatusActive && change.toStatus == ntpdb.ServerScoresStatusTesting {
        toAdd++  // -2 + demotions = potentially positive
    }
}
```

**Problems:**
- Adds demotions to negative target difference (mathematically incorrect)
- Only handles active→testing, not other future status transitions
- Doesn't track working count during batch processing

### 2. Missing Active Excess Demotion
- No mechanism to reduce healthy active monitors when over target
- System accumulates active monitors beyond target with no reduction path

### 3. Testing Pool Excess Issue
- 23 testing monitors vs target 5 (18 over)
- Dynamic testing pool sizing may not be working correctly
- Possible similar math bugs in candidate→testing promotion logic

## Implementation Plan

### Phase 1: Combined Math Fix + Active Excess Demotion (Critical - Immediate)

**Location**: `selector/process.go:281-288` in `applySelectionRules`

**Changes:**
1. Replace buggy `toAdd` calculation with working count logic
2. Make status change detection generic
3. Add Rule 1.5 - Active excess demotion with safety checks
4. Add cascading demotion logic for testing pool
5. Add emergency override integration

**Combined Implementation:**
```go
// Track working counts for both active and testing throughout process
workingActiveCount := currentActiveMonitors
workingTestingCount := len(testingMonitors)
emergencyOverride := (currentActiveMonitors == 0)

// Apply all existing demotions to working counts first
for _, change := range changes {
    if change.fromStatus == ntpdb.ServerScoresStatusActive && change.toStatus != ntpdb.ServerScoresStatusActive {
        workingActiveCount--
    }
    if change.toStatus == ntpdb.ServerScoresStatusTesting {
        workingTestingCount++
    }
    if change.fromStatus == ntpdb.ServerScoresStatusTesting && change.toStatus != ntpdb.ServerScoresStatusTesting {
        workingTestingCount--
    }
}

// Rule 1.5: Demote excess healthy active monitors when over target
// SAFETY CHECKS: Emergency override, minimum active count, bootstrap scenarios
if workingActiveCount > targetActiveMonitors &&
   workingActiveCount > 1 && // Never reduce to 0
   !emergencyOverride &&
   limits.demotions > demotionsSoFar {

    excessActive := workingActiveCount - targetActiveMonitors

    // Reserve demotion budget for constraint violations (Rule 1/2)
    reservedDemotions := min(2, limits.demotions - demotionsSoFar) // Reserve for unhealthy demotions
    availableDemotions := max(0, limits.demotions - demotionsSoFar - reservedDemotions)
    demotionsNeeded := min(excessActive, availableDemotions)

    // Check if testing pool can accommodate these demotions
    dynamicTestingTarget := baseTestingTarget + max(0, targetActiveMonitors - (workingActiveCount - demotionsNeeded))
    projectedTestingCount := workingTestingCount + demotionsNeeded

    if demotionsNeeded > 0 {
        // Use existing sort order - take worst performers from end of list
        startIndex := len(healthyActiveMonitors) - demotionsNeeded

        for i := startIndex; i < len(healthyActiveMonitors); i++ {
            changes = append(changes, statusChange{
                monitorID:  healthyActiveMonitors[i].monitor.ID,
                fromStatus: ntpdb.ServerScoresStatusActive,
                toStatus:   ntpdb.ServerScoresStatusTesting,
                reason:     "excess active demotion",
            })
            workingActiveCount--
            workingTestingCount++
            demotionsSoFar++
        }

        sl.log.InfoContext(ctx, "demoted excess active monitors",
            "count", demotionsNeeded,
            "newActiveCount", workingActiveCount,
            "newTestingCount", workingTestingCount)
    }

    // CASCADING DEMOTION: If testing pool now exceeds target, demote testing→candidate
    if workingTestingCount > dynamicTestingTarget && limits.testingRemovals > testingDemotionsSoFar {
        excessTesting := workingTestingCount - dynamicTestingTarget
        testingDemotionsNeeded := min(excessTesting, limits.testingRemovals - testingDemotionsSoFar)

        // Demote worst testing monitors to candidate
        testingStartIndex := len(testingMonitors) - testingDemotionsNeeded

        for i := testingStartIndex; i < len(testingMonitors); i++ {
            changes = append(changes, statusChange{
                monitorID:  testingMonitors[i].monitor.ID,
                fromStatus: ntpdb.ServerScoresStatusTesting,
                toStatus:   ntpdb.ServerScoresStatusCandidate,
                reason:     "cascading testing demotion from active excess",
            })
            workingTestingCount--
            testingDemotionsSoFar++
        }

        sl.log.InfoContext(ctx, "cascading testing demotions",
            "count", testingDemotionsNeeded,
            "newTestingCount", workingTestingCount)
    }
}

// Calculate active promotions needed based on working count
toAdd := max(0, targetNumber - workingActiveCount)
if toAdd > 0 && changesRemaining > 0 {
    promotionsNeeded := min(toAdd, changesRemaining)
    // ... existing promotion logic with working count updates

    // Update working count after each promotion
    for each promotion {
        workingActiveCount++
        workingTestingCount-- // if promoted from testing
    }
}
```

### Phase 2: Investigate Testing Pool Excess (Diagnostic)

**Analysis Required:**
1. **Check dynamic testing pool calculation:**
   ```go
   activeGap := max(0, targetActiveMonitors - len(activeMonitors))
   dynamicTestingTarget := baseTestingTarget + activeGap
   ```
   With 9 active vs 7 target: `activeGap = 0`, so `dynamicTestingTarget = 5`

2. **Review Rule 2.5 implementation** - why isn't it reducing 23→5?

3. **Check candidate→testing promotion logic** for similar math bugs

4. **Historical data analysis** - when did testing pool grow to 23?

**Potential Issues:**
- Rule 2.5 not executing (missing logic branch?)
- Change limits preventing sufficient testing demotions
- Different math bug in candidate promotion logic
- Dynamic testing calculation not being used properly

### Phase 3: Add Enhanced Working Count Tracking (Enhancement)

## Implementation Notes (July 2025)

### Actual Implementation Differed from Plan

**Simplified Approach:**
1. **No cascading demotion needed** - Fixed rule execution order instead
2. **Simpler budget calculation** - Count actual constraints, not hardcoded reserve
3. **Rule 5 capacity limit** - Prevents over-promotion, eliminating churn

**Key Discoveries:**
1. **Rule execution order was critical**: Rule 2.5 ran before Rule 5, missing promotions
2. **Working count tracking bugs**: Rules weren't updating `workingTestingCount`
3. **Rule 5 over-promotion**: Promoted 2 candidates without checking testing capacity

**Actual Fixes Applied:**
1. **Rule 1.5**: Implemented with safety checks (no cascading needed)
   ```go
   // Count actual monitors needing constraint-based demotions
   constraintDemotionsNeeded := 0
   for _, em := range activeMonitors {
       if em.recommendedState == candidateOut {
           constraintDemotionsNeeded++
       }
   }
   reservedDemotions := min(constraintDemotionsNeeded, limits.activeRemovals-demotionsSoFar)
   ```

2. **Rule Execution Order**: Moved Rule 2.5 after Rule 5
   - Before: Rule 1 → 2 → 1.5 → 2.5 → 3 → 5 → 6
   - After: Rule 1 → 2 → 1.5 → 3 → 5 → 2.5 → 6

3. **Rule 5 Capacity Check**:
   ```go
   activeGap := max(0, targetActiveMonitors-workingActiveCount)
   dynamicTestingTarget := baseTestingTarget + activeGap
   testingCapacity := max(0, dynamicTestingTarget-workingTestingCount)
   promotionsNeeded := min(min(changesRemaining, 2), testingCapacity)
   ```

**Results:**
- Server 1982: 9→7 active monitors ✅
- Server 1486: Proper testing limits maintained ✅
- Server 1065: No more testing pool overflow ✅

**Implementation:**
```go
type workingCounts struct {
    active  int
    testing int
}

// Initialize with current counts
working := workingCounts{
    active:  len(activeMonitors),
    testing: len(testingMonitors),
}

// Update after each status change
func (wc *workingCounts) applyChange(change statusChange) {
    switch change.fromStatus {
    case ntpdb.ServerScoresStatusActive:
        wc.active--
    case ntpdb.ServerScoresStatusTesting:
        wc.testing--
    }

    switch change.toStatus {
    case ntpdb.ServerScoresStatusActive:
        wc.active++
    case ntpdb.ServerScoresStatusTesting:
        wc.testing++
    }
}
```

### Phase 4: Add Safety Validation (Monitoring)

**Implementation:**
```go
// Final validation after all changes
finalActiveCount := workingActiveCount
finalTestingCount := workingTestingCount

if finalActiveCount > targetActiveMonitors {
    sl.log.ErrorContext(ctx, "CRITICAL: active monitor count exceeds target",
        "finalCount", finalActiveCount,
        "target", targetActiveMonitors,
        "serverID", server.ID)
    // Optionally: trigger alert/monitoring
}

if finalTestingCount > dynamicTestingTarget {
    sl.log.ErrorContext(ctx, "CRITICAL: testing monitor count exceeds target",
        "finalCount", finalTestingCount,
        "target", dynamicTestingTarget,
        "serverID", server.ID)
}
```

## Testing Strategy

### Unit Tests

1. **Test Active Promotion Math Fix:**
   ```go
   func TestActivePromotionMathFix(t *testing.T) {
       // Scenario: 9 active, target 7, 3 demotions
       // Should calculate: working = 9-3 = 6, toAdd = 7-6 = 1
   }
   ```

2. **Test Active Excess Demotion:**
   ```go
   func TestActiveExcessDemotion(t *testing.T) {
       // Scenario: 9 healthy active, target 7
       // Should demote 2 worst performers
   }
   ```

3. **Test Generic Status Change Detection:**
   ```go
   func TestGenericStatusChangeDetection(t *testing.T) {
       // Test active→testing, active→candidate, active→paused
   }
   ```

4. **Test Emergency Override Safety:**
   ```go
   func TestEmergencyOverrideSafety(t *testing.T) {
       // Scenario: 0 active monitors, excess demotions should be blocked
       // Expected: emergencyOverride=true blocks Rule 1.5
   }
   ```

5. **Test Change Limit Budget Management:**
   ```go
   func TestChangeLimitBudgetManagement(t *testing.T) {
       // Scenario: Limited demotion budget, constraint violations + excess
       // Expected: Constraint violations prioritized over excess handling
   }
   ```

6. **Test Cascading Demotion Logic:**
   ```go
   func TestCascadingDemotionLogic(t *testing.T) {
       // Scenario: Active excess causes testing overflow
       // Expected: active→testing, then testing→candidate demotions
   }
   ```

### Integration Tests

1. **Server 1982 Scenario Test:**
   - Input: 9 active, 23 testing, 10 candidate
   - Expected: 7 active, 5 testing, candidates promoted as needed
   - Validate: no over-promotion, proper demotion, cascading effects

2. **Change Limits Respect:**
   - Ensure demotions don't exceed `limits.demotions`
   - Ensure promotions don't exceed `limits.promotions`
   - Validate budget prioritization (constraint violations before excess)

3. **Emergency Override Integration:**
   - Test with zero active monitors
   - Ensure override doesn't violate fundamental limits
   - Validate Rule 1.5 is blocked during emergency

4. **Bootstrap Scenario Handling:**
   - Test excess active monitors during bootstrap
   - Ensure testing pool capacity is checked
   - Validate cascading demotions work during bootstrap

5. **Complex Interaction Test:**
   - Test all rules working together (1, 1.5, 2, 2.5, 3, 5)
   - Multiple constraint types and limit scenarios
   - Validate working count consistency throughout

### Production Validation

1. **Metrics Monitoring:**
   - Track active/testing counts before/after selection runs
   - Alert on limit violations
   - Monitor demotion/promotion rates

2. **Logging Enhancement:**
   - Log all limit-related decisions
   - Include working count calculations
   - Log reasons for demotions/promotions

## Implementation Order

### Immediate (Critical Path - Combined Implementation)
1. **Phase 1: Combined Math Fix + Rule 1.5** - single atomic change
   - Fix active promotion math bug
   - Add Rule 1.5 with emergency override safety
   - Add cascading demotion logic
   - Implement change limit budget management
   - Add working count tracking for active and testing

### Short Term (1-2 weeks)
2. **Phase 2: Testing pool investigation** - understand why 23 vs 5
3. **Comprehensive testing** - validate all safety scenarios
4. **Production deployment** with monitoring

### Medium Term (1 month)
5. **Phase 3: Enhanced working count tracking** - systematic optimization
6. **Phase 4: Safety validation** - monitoring and alerting
7. **Performance optimization** - if needed

## Expected Outcomes

### Immediate Benefits
- **Server 1982**: 9 active → 7 active monitors
- **Prevent new over-promotion** beyond targets
- **Mathematical correctness** in promotion logic

### Short Term Benefits
- **Server 1982**: 23 testing → 5 testing monitors
- **Self-healing system** that maintains proper limits
- **Architectural completeness** with balanced promotion/demotion

### Long Term Benefits
- **Robust limit enforcement** across all servers
- **Predictable monitor distribution** according to targets
- **System reliability** through proper constraint management

## Risk Analysis

### ✅ Mitigated High Risk Areas (Addressed by Updated Plan)

#### Emergency Override Integration (RESOLVED)
- **Risk**: Rule 1.5 could demote active monitors when emergency override is active
- **Mitigation**: Added explicit emergency override check: `!emergencyOverride`
- **Safety**: Never reduce active count to 0: `workingActiveCount > 1`

#### Change Limits Budget Management (RESOLVED)
- **Risk**: Rule 1.5 could exhaust demotion budget needed for constraint violations
- **Mitigation**: Reserve demotion budget for unhealthy demotions (Rule 1/2)
- **Implementation**: `availableDemotions = limits.demotions - demotionsSoFar - reservedDemotions`

#### Bootstrap Scenario Handling (RESOLVED)
- **Risk**: Active excess demotion when testing pool is full or during bootstrap
- **Mitigation**: Cascading demotion logic - demote testing→candidate if needed
- **Check**: Validate testing pool capacity before active→testing demotions

### Low Risk Changes
- Math bug fix (isolated logic change)
- Generic status change detection (broader compatibility)
- Working count tracking (systematic state management)

### Medium Risk Changes
- Cascading demotion logic (multiple state transitions)
- Testing pool capacity calculations
- Change limit budget management

### Remaining High Risk Areas
- Testing pool investigation (may uncover deeper issues)
- Performance impact of cascading demotions
- Complex interaction testing between all rules

## Success Criteria

### Critical Success Criteria
1. **Math Verification**: Active promotion logic calculates correct `toAdd` values
2. **Limit Enforcement**: No server exceeds active/testing targets after selection
3. **Server 1982 Fix**: Specific validation that problematic server reaches targets (9→7 active, 23→5 testing)
4. **Emergency Override Safety**: Never reduces active monitors to 0 during override
5. **Budget Management**: Constraint violation demotions always take priority over excess demotions

### Additional Success Criteria
6. **Cascading Logic**: Testing pool properly manages overflow from active demotions
7. **Bootstrap Compatibility**: System can bootstrap from zero monitors without interference
8. **Regression Prevention**: All existing functionality preserved
9. **Performance Impact**: Selection runtime remains acceptable
10. **Safety Validation**: All edge cases covered by tests

## Summary of Plan Updates (Addressing Critical Feedback)

### Key Improvements Made
1. **✅ Combined Phase 1 & 2**: Single atomic implementation prevents intermediate inconsistent states
2. **✅ Emergency Override Safety**: Explicit checks prevent interference with emergency scenarios
3. **✅ Budget Prioritization**: Reserve demotion budget for constraint violations before excess handling
4. **✅ Cascading Demotion Logic**: Handle testing pool overflow from active demotions
5. **✅ Bootstrap Protection**: Validate testing pool capacity and prevent bootstrap interference
6. **✅ Generic Status Changes**: Support future status transitions beyond active→testing

### Critical Safety Mechanisms
- **Never reduce to 0 active monitors**: `workingActiveCount > 1`
- **Emergency override protection**: `!emergencyOverride` check in Rule 1.5
- **Budget reservation**: Reserve demotions for unhealthy monitors before excess handling
- **Capacity validation**: Check testing pool capacity before cascading demotions
- **Working count consistency**: Track both active and testing counts throughout process

### Validation Strategy
- **Unit tests** for all safety mechanisms and edge cases
- **Integration tests** including Server 1982 specific scenario
- **Production monitoring** with comprehensive logging and alerting
- **Regression testing** to ensure existing functionality preserved

---

**Implementation Priority**: **Critical** - Production servers are operating beyond design limits, potentially affecting monitoring coverage and system reliability.

**Estimated Timeline**:
- **Phase 1 (Combined)**: 1 week for implementation + testing
- **Full Validation**: 2-3 weeks including production testing
- **Complete Implementation**: 3-4 weeks with monitoring and optimization
