# Eliminate "New" Status Architecture Plan

## Executive Summary

This plan proposes a significant architectural simplification of the monitor selector system by eliminating the conceptual "new" status and relying entirely on existing production code that manages `server_scores` entries. This change will resolve the persistent constraint violation warnings and create a much cleaner, more maintainable architecture.

**Last Updated**: 2025-01-26 - Incorporated implementation clarifications

## Problem Statement

### Current Issues
1. **Persistent constraint violation warnings** for monitors with `serverStatus=new`
2. **Complex dual-state system** with global monitor status and server-monitor relationship
3. **Constraint checking for unassigned monitors** that shouldn't have constraints
4. **Convoluted selection logic** with "available pool" evaluation
5. **False constraint violations** blocking valid monitor assignments

### Root Cause
The selector tries to manage both "which monitors should be considered" (assignment policy) and "how to select among assigned monitors" (selection algorithm) in a single component, leading to complexity and bugs.

## Current Architecture Analysis

### Current State Model
```
Global Monitor Status (monitors.status):
├── pending   → Should gradually phase out
├── testing   → Can be assigned to servers
├── active    → Can be assigned to servers
├── paused    → Should stop work
└── deleted   → Should be removed

Server-Monitor Relationship:
├── new       → Conceptual only (not in server_scores)
├── candidate → In server_scores with candidate status
├── testing   → In server_scores with testing status
└── active    → In server_scores with active status
```

### Current Selector Flow
1. **Load assigned monitors** via `GetMonitorPriority(serverID)`
2. **Find available monitors** via `GetAvailableMonitors(serverID)`
3. **Evaluate ALL monitors** (assigned + available) for constraints
4. **Determine states** for all monitors based on constraints
5. **Apply selection rules** to make status changes

### Problems with Current Flow
- **Step 2-3**: Available monitors get constraint-checked unnecessarily
- **Step 4**: "New" monitors get blocked by false constraint violations
- **Step 5**: Blocked monitors can't become candidates

## Proposed Simplified Architecture

### New State Model
```
Global Monitor Status (monitors.status):
├── pending   → Should gradually phase out
├── testing   → Can be assigned to servers
├── active    → Can be assigned to servers
├── paused    → Should stop work
└── deleted   → Should be removed

Server-Monitor Relationship (only if in server_scores):
├── candidate → Monitor is being considered for this server
├── testing   → Monitor is actively monitoring this server
└── active    → Monitor is confirmed for this server
```

### New Selector Flow
1. **Load all server_scores entries** for the server
2. **Evaluate only assigned monitors** for constraints and health
3. **Apply selection rules** among existing assignments only

### Key Changes
- **No "available pool" logic** - assignment policy handled externally
- **No "new" status** - monitors either have server_scores entries or they don't
- **Constraint checking only for promotions** - candidate→testing→active
- **External assignment management** - other code handles which monitors get considered

## Implementation Plan

### Phase 1: Analysis and Preparation (1 day)
1. **Document existing assignment code** - The API system handles initial assignment via `InsertMonitorServerScores` query in `monitor_admin.sql`
2. **Verify GetAvailableMonitors is not used elsewhere** - Confirmed it's only used in selector
3. **Create comprehensive test cases** - cover bootstrap, constraint changes, and cascading scenarios
4. **Review constraint checking requirements** - Constraints must be checked on every run as rules may change

### Phase 2: Remove Available Monitors Logic (2 days)

#### Files to Modify:
- `selector/selector.go` - Remove findAvailableMonitors call and evaluation
- `selector/process.go` - Remove "Rule 4: Add new monitors as candidates"
- `ntpdb/query.sql` - Remove GetAvailableMonitors query (if not used elsewhere)

#### Specific Changes:
```go
// selector/selector.go - Remove these lines:
availableMonitors, err := sl.findAvailableMonitors(ctx, db, serverID)
// ... available monitor evaluation logic

// selector/process.go - Remove Rule 4:
// Rule 4: Add new monitors as candidates (respecting change limits)
// ... entire section
```

### Phase 3: Update Constraint Checking (1 day)

#### Constraint Checking Strategy
- **Check constraints on EVERY run** - Not just for promotions, as constraint rules may change
- **Mark violations as `candidateOut`** - Triggers gradual demotion (active→testing→candidate)
- **Support cascading demotions** - Monitor A demotion may trigger Monitor B demotion
- **Keep existing constraint types** - Network, account limits, network diversity

#### Update State Machine
```go
// state.go - Updated determineState function
func (sl *Selector) determineState(
    monitor *monitorCandidate,
    violation *constraintViolation,
) candidateState {
    // Check global status first (pending, paused, deleted)
    // Then check constraints for all states
    // Mark violations as candidateOut for gradual demotion
    // Remove "new" status handling completely
}
```

### Phase 4: Update Database Queries and Bootstrap Logic (1 day)

#### Modified Queries:
- **GetMonitorPriority**: Already only returns monitors with server_scores entries
- **Remove GetAvailableMonitors**: Delete from query.sql (not used elsewhere)
- **No metrics queries use "new" status** - Confirmed no updates needed

#### Bootstrap Strategy:
- **API ensures candidates exist** via `InsertMonitorServerScores` when monitor becomes active/testing
- **Selector promotes candidates to testing** when all monitors are candidates
- **Promote multiple at once during bootstrap** to reach configured testing count
- **Empty server_scores is handled naturally** - Servers with no monitors drop in priority

#### Schema Considerations:
- No schema changes needed
- Existing server_scores entries work as-is

### Phase 5: Update Change Limits and Testing (2 days)

#### Change Limit Logic:
```go
// Dynamic change limits based on blocked monitors
maxChanges := 1 // Normal case: optimizing best monitors
if hasBlockedMonitors { // deleted, paused monitors exist
    maxChanges = 2 // Expedite recalibration
}
```

#### Test Scenarios:
1. **Normal operation** - verify selection still works for assigned monitors
2. **Constraint violations** - ensure proper demotion behavior (active→testing→candidate)
3. **Bootstrap scenarios** - all candidates get promoted to testing as needed
4. **Cascading demotions** - Monitor A demotion triggers Monitor B demotion
5. **Constraint rule changes** - Existing monitors violate new constraints
6. **Edge cases** - empty servers, all blocked monitors, etc.

#### Test Implementation:
- **Create server_scores entries as needed** in test setup
- **Test gradual demotion paths** for constraint violations
- **Verify bootstrap promotes correctly** when all monitors are candidates

#### Integration Testing:
- Use existing CI tools (`./scripts/test-ci-local.sh`)
- Run selector against production-like data
- Verify no warnings appear for "new" status monitors

### Phase 6: Production Deployment (1 week)

#### Deployment Strategy:
1. **Shadow mode** - run new selector alongside old one, compare decisions
2. **Gradual rollout** - enable for subset of servers first
3. **Monitor metrics** - watch for unexpected behavior
4. **Rollback plan** - feature flag to revert to old logic

## External Assignment Code Integration

### Current Production Assignment Code
The API system creates server_scores entries via `InsertMonitorServerScores`:
```sql
-- From monitor_admin.sql
INSERT IGNORE INTO server_scores
  (monitor_id, status, server_id, score_raw, created_on)
  SELECT m.id, 'candidate', s.id, s.score_raw, NOW()
  FROM servers s, monitors m
  WHERE
    s.ip_version = m.ip_version
    AND m.id = ?
    AND m.status IN ('active', 'testing')
    AND s.deletion_on IS NULL;
```

This query:
1. **Creates candidate entries** for all servers matching the monitor's IP version
2. **Triggers on monitor activation** - when monitor status becomes active/testing
3. **Handles IP version matching** - IPv4 monitors only assigned to IPv4 servers
4. **Respects server deletion** - skips servers marked for deletion

### Selector Responsibilities (Simplified)
The selector will only handle:
1. **Health-based promotions** - candidate→testing based on health
2. **Performance-based promotions** - testing→active based on metrics
3. **Constraint-based demotions** - active→testing→candidate when constraints violated
4. **Constraint validation** - check all monitors on every run for rule changes
5. **Global status changes** - handle pending/paused/deleted monitors
6. **Bootstrap handling** - promote candidates when no testing monitors exist

### Interface Contract
```go
// What the selector expects:
// - server_scores entries exist for all monitors that should be considered
// - Initial status is 'candidate' for new assignments
// - API creates entries when monitors become active/testing

// What the selector provides:
// - Promotes healthy candidates to testing (including bootstrap)
// - Promotes good testing monitors to active
// - Demotes violating monitors gradually (candidateOut)
// - Checks constraints on every run
// - Respects global monitor status changes
```

## Risk Assessment

### High Risk
- ~~**Incomplete understanding of assignment code**~~ - ✓ Verified: API uses `InsertMonitorServerScores`
- ~~**Bootstrap scenarios**~~ - ✓ Resolved: Selector promotes candidates to testing as needed
- ~~**Performance impact**~~ - ✓ Resolved: Actually improves performance (fewer queries)

### Medium Risk
- **Cascading demotions** - Multiple monitors may be demoted in one run
- **Constraint rule changes** - May trigger many demotions if rules tighten
- **Testing coverage** - Need comprehensive tests for new scenarios

### Low Risk
- **Database performance** - Fewer queries should improve performance
- **Code maintainability** - Simpler logic reduces bugs
- **Constraint violations** - Will be eliminated entirely
- ~~**Empty server_scores**~~ - ✓ Resolved: Natural priority drop, no special handling

## Mitigation Strategies

### For Medium Risks:
1. **Test cascading scenarios thoroughly** - Ensure demotions work correctly
2. **Monitor constraint changes in production** - Watch for mass demotions
3. **Add comprehensive integration tests** for all edge cases
4. **Log all state transitions** for debugging

### Change Limit Protection:
- Normal operations: 1 change per run
- Blocked monitors present: 2 changes per run
- Prevents mass changes while allowing recovery

## Success Metrics

### Bug Resolution:
- **Zero constraint violation warnings** for "new" status monitors
- **Successful monitor promotions** from candidate pool
- **Proper constraint enforcement** for promotions only

### Code Quality:
- **Reduced lines of code** in selector package
- **Simplified test cases** without "new" status complexity
- **Clearer separation** between assignment and selection

### Performance:
- **Faster selector runs** due to less constraint checking
- **Fewer database queries** (no GetAvailableMonitors)
- **Better monitoring dashboard accuracy**

## Timeline

- **Week 1**: Analysis and preparation (Phase 1)
- **Week 2**: Implementation (Phases 2-4)
- **Week 3**: Testing and validation (Phase 5)
- **Week 4**: Production deployment (Phase 6)

## Rollback Plan

If issues arise during deployment:
1. **Feature flag** to revert to old available monitors logic
2. **Keep GetAvailableMonitors query** until rollback period expires
3. **Maintain old constraint checking** as fallback option
4. **Monitor assignment code** should be unaffected by selector changes

## Key Implementation Details

### Constraint Checking Changes
```go
// OLD: Only check constraints for promotions
if isPromoting {
    checkConstraints()
}

// NEW: Check constraints on every run
for _, monitor := range allMonitors {
    violation := checkConstraints(monitor)
    if violation != nil && monitor.isActive() {
        monitor.state = candidateOut // Gradual demotion
    }
}
```

### Bootstrap Handling
```go
// When all monitors are candidates
if allCandidates && needMoreTesting {
    // Promote up to targetTestingCount at once
    promoteCount := min(targetTestingCount - currentTestingCount,
                       len(healthyCandidates))
    // Respect constraints during promotion
}
```

### Demotion Cascade
```go
// Active monitor violates constraint
monitorA.state = candidateOut // Will become testing

// This may cause testing monitor to violate
if monitorB.wouldViolate() {
    monitorB.state = candidateOut // Will become candidate
}
```

## Conclusion

This architectural simplification will:
- **Eliminate the persistent constraint bug** by removing unnecessary constraint checks for unassigned monitors
- **Simplify the codebase** by removing complex "available pool" logic (~200+ lines of code)
- **Improve maintainability** by clarifying responsibility boundaries
- **Support dynamic constraint updates** with proper gradual demotion
- **Handle all edge cases properly** including bootstrap and cascading scenarios
- **Leverage existing production code** that already handles assignment policy

The change is low-risk because:
1. It removes complexity rather than adding it
2. The existing assignment code (API) is already proven in production
3. All clarifications have been addressed with specific solutions
4. Comprehensive testing plan covers all scenarios
