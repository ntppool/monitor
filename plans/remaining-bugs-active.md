# Active Bug Tracking for Selector Package

## Overview

This document tracks the remaining unresolved bugs in the selector package after comprehensive analysis and major fixes completed in July 2025. Most originally identified issues were false positives or have been resolved.

**Last Updated**: July 2025 after monitor limit enforcement implementation

## Current Status Summary

**Major Fixes Completed ✅**:
- **Monitor Limit Enforcement** - Added Rule 1.5, fixed working count tracking, Rule 5 capacity limits
- **Network Diversity Constraint** - Fixed target state confusion (commit: 5d16aaa)
- **Safety Variable Scope Creep** - Fixed safety variables blocking legitimate constraint cleanup
- **Mathematical Consistency** - Fixed working count tracking across all rules

**Analysis Outcome**: After thorough code review, 9 of 13 originally identified "bugs" were false positives. The codebase demonstrates good engineering practices.

## Active Bugs Requiring Fixes

### 1. **Emergency Override Coverage Gap** (High Priority)
**Status**: 🔄 **UNRESOLVED** - Confirmed bug requiring fix

**Location**: `selector/constraints.go:611-629` in `canPromoteToTesting`

**Problem**: Emergency override only applies to testing→active promotions, not candidate→testing promotions
```go
// canPromoteToTesting lacks emergency override parameter
func (sl *Selector) canPromoteToTesting(
    monitor *monitorCandidate,
    server *serverInfo,
    accountLimits map[uint32]*accountLimit,
    existingMonitors []ntpdb.GetMonitorPriorityRow,
) bool {
    // No emergency override logic here - MISSING
}
```

**Impact**: System could get stuck with zero monitors if candidates can't be promoted during emergencies

**Fix Required**: Add `emergencyOverride bool` parameter to `canPromoteToTesting` function and bypass constraints when `emergencyOverride = true`

**Test Cases Needed**:
- Zero active monitors with constraint-violating candidates
- Emergency promotion despite account/network constraints
- Bootstrap scenarios with constraint violations

### 2. **Non-Functional Grandfathering Logic** (Medium Priority)
**Status**: 🔄 **UNRESOLVED** - Confirmed logic issue

**Location**: `selector/state.go:104-113`

**Problem**: Grandfathered and non-grandfathered violations have identical behavior
```go
if violation.Type != violationNone {
    if violation.IsGrandfathered {
        // Grandfathered violations get gradual removal
        return candidateOut
    }
    // All constraint violations trigger gradual removal
    return candidateOut  // SAME BEHAVIOR - No benefit from grandfathering
}
```

**Impact**: Grandfathering provides no actual operational benefit, misleading configuration

**Fix Options**:
1. **Different removal rate**: Grandfathered monitors get slower demotion
2. **Priority ordering**: Grandfathered monitors processed after non-grandfathered
3. **Grace period**: Grandfathered monitors get time-based delays before demotion
4. **Remove grandfathering**: If not needed operationally, simplify the logic

**Recommendation**: Investigate operational need for grandfathering before implementing solution

### 3. **Bootstrap Constraint Inconsistency** (Low Priority)
**Status**: 🔄 **UNRESOLVED** - Related to bug #1

**Location**: `selector/process.go:484, 504` in bootstrap scenarios

**Problem**: Bootstrap calls `canPromoteToTesting` without emergency override logic
```go
// Bootstrap scenario lacks emergency override consistency
if sl.canPromoteToTesting(&em.monitor, server, workingAccountLimits, assignedMonitors) {
    // May fail if all candidates have constraint violations
}
```

**Impact**: System bootstrap could fail if all candidates have constraint violations

**Fix**: Will be resolved automatically when bug #1 (Emergency Override Coverage Gap) is fixed

## Fixed/Resolved Issues ✅

### Target State Confusion in Network Diversity Check
**Status**: ✅ **RESOLVED** (commit: 5d16aaa)
- Fixed network diversity constraint to use target state instead of current state
- Prevents invalid promotions due to state confusion

### Safety Variable Scope Creep
**Status**: ✅ **RESOLVED** via monitor limit enforcement work
- Fixed `maxRemovals = 0` blocking ALL changes instead of just active demotions
- Safety variables now properly scoped to their intended operations

### Working Count Mathematical Consistency
**Status**: ✅ **RESOLVED** via monitor limit enforcement work
- Fixed missing `workingTestingCount++` in Rules 5 and 6
- All rules now properly update working counts after decisions

### Rule Execution Order
**Status**: ✅ **RESOLVED** via monitor limit enforcement work
- Fixed Rule 2.5 to run after Rule 5 to see pending promotions
- Optimized rule sequence for mathematical consistency

## Verified Non-Issues (False Positives) ❌

**Account Limit Logic**: Correctly handles self-exclusion and candidate counting
**Inefficient Account Limit Building**: Proper caching implemented with existence checks
**Race Condition in Account Limit Updates**: Uses proper deep copying patterns
**Missing Null Checks**: Defensive programming patterns confirmed
**Change Limit Calculation**: Bootstrap cases correctly handled with higher limits
**Redundant Constraint Checks**: Intentional for logging/metrics purposes
**Metrics Recording**: Sequential processing with Prometheus internal synchronization
**State Access Patterns**: No evidence of actual race conditions

## Implementation Priority

### Phase 1: Emergency Override Fix (High Priority)
**Timeline**: 1-2 days
1. Add `emergencyOverride` parameter to `canPromoteToTesting`
2. Implement constraint bypass logic when emergency conditions detected
3. Add comprehensive tests for emergency scenarios
4. Verify bootstrap logic works with emergency override

### Phase 2: Grandfathering Logic (Medium Priority)
**Timeline**: 2-3 days
1. **Investigation**: Determine operational need for grandfathering behavior
2. **Decision**: Choose approach (enhanced grandfathering vs removal)
3. **Implementation**: Implement chosen solution
4. **Validation**: Test with production-like constraint scenarios

### Phase 3: Integration Testing (Low Priority)
**Timeline**: 1 day
1. End-to-end testing of all fixes together
2. Performance validation of changes
3. Production deployment preparation

## Testing Requirements

### Critical Test Scenarios
1. **Zero Active Monitor Recovery**: Emergency override enables candidate promotion despite constraints
2. **Grandfathering Behavior**: Validate different treatment of grandfathered vs new violations
3. **Bootstrap with Constraints**: System recovery when all candidates have constraint violations
4. **Emergency + Grandfathering**: Interaction between emergency override and grandfathered violations

### Performance Tests
1. **No Regression**: Verify fixes don't slow down normal operations
2. **Emergency Performance**: Ensure emergency scenarios complete quickly
3. **Constraint Checking**: Validate constraint logic performance unchanged

## Development Notes

### Emergency Override Pattern
```go
// Recommended implementation pattern
func (sl *Selector) canPromoteToTesting(
    monitor *monitorCandidate,
    server *serverInfo,
    accountLimits map[uint32]*accountLimit,
    existingMonitors []ntpdb.GetMonitorPriorityRow,
    emergencyOverride bool, // ADD THIS PARAMETER
) bool {
    // Check constraints normally
    if violation := sl.checkConstraints(monitor, server, accountLimits, existingMonitors); violation != nil {
        if emergencyOverride {
            // Log emergency bypass
            sl.logger.WarnContext(ctx, "emergency override: promoting despite constraint violation",
                "monitor", monitor.ID, "violation", violation.Type)
            return true
        }
        return false
    }
    return true
}
```

### Grandfathering Enhancement Options
```go
// Option 1: Time-based grace period
if violation.IsGrandfathered {
    gracePeriod := time.Since(violation.Since)
    if gracePeriod < time.Hour * 24 {
        return candidatePending // Delay demotion
    }
    return candidateOut // Gradual removal after grace period
}

// Option 2: Priority-based processing
if violation.IsGrandfathered {
    return candidateOut // Lower priority for removal
} else {
    return candidateBlock // Higher priority for removal
}
```

## Success Criteria

### Bug Resolution Validation
- [ ] Emergency scenarios with zero monitors recover successfully
- [ ] Grandfathering provides measurable operational benefit
- [ ] Bootstrap scenarios work regardless of constraint violations
- [ ] No performance regression in normal operations

### Code Quality Improvements
- [ ] Emergency override logic consistent across all promotion paths
- [ ] Grandfathering behavior clearly documented and tested
- [ ] All edge cases covered by comprehensive tests
- [ ] Simplified logic where grandfathering not needed

---

**Next Review**: After Phase 1 completion - validate emergency override fix effectiveness
**Responsibility**: Engineering team with selector expertise
**Priority**: Focus on bug #1 (Emergency Override) first as highest impact
