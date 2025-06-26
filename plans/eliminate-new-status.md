# Eliminate "New" Status Architecture Plan

## Executive Summary

This plan proposes a significant architectural simplification of the monitor selector system by eliminating the conceptual "new" status and relying entirely on existing production code that manages `server_scores` entries. This change will resolve the persistent constraint violation warnings and create a much cleaner, more maintainable architecture.

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
1. **Document existing assignment code** - identify the production code that creates server_scores entries
2. **Analyze current selector dependencies** - understand what calls findAvailableMonitors
3. **Review GetAvailableMonitors usage** - ensure it's safe to remove
4. **Create comprehensive test cases** - cover all current selection scenarios

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

### Phase 3: Simplify Constraint Checking (1 day)

#### Remove Constraint Checks for Non-Promotions
- Only check constraints in `canPromoteToActive()` and `canPromoteToTesting()`
- Remove constraint checking for current state maintenance
- Update `determineState()` to not handle "new" status

#### Update State Machine
```go
// state.go - Simplified determineState function
func (sl *Selector) determineState(
    monitor *monitorCandidate,
    violation *constraintViolation,
) candidateState {
    // Only handle candidate, testing, active statuses
    // Remove "new" status handling completely
}
```

### Phase 4: Update Database Queries (1 day)

#### Modified Queries:
- **GetMonitorPriority**: Ensure it only returns monitors with server_scores entries
- **Remove GetAvailableMonitors**: No longer needed
- **Update metrics queries**: Remove "new" status from counts

#### Schema Considerations:
- No schema changes needed
- Existing server_scores entries work as-is
- May need to clean up orphaned "new" status tracking

### Phase 5: Testing and Validation (2 days)

#### Test Scenarios:
1. **Normal operation** - verify selection still works for assigned monitors
2. **Constraint violations** - ensure proper demotion behavior
3. **Bootstrap scenarios** - verify behavior when no candidates exist
4. **Edge cases** - empty servers, all blocked monitors, etc.

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
The existing production code that creates server_scores entries needs to handle:
1. **Initial candidate assignment** - which monitors should be considered for which servers
2. **Network diversity** - ensure geographic and network distribution
3. **Account balancing** - respect account limits across servers
4. **Capacity management** - don't overload servers with too many candidates

### Selector Responsibilities (Simplified)
The selector will only handle:
1. **Health-based promotions** - candidate→testing based on health
2. **Performance-based promotions** - testing→active based on metrics
3. **Constraint-based demotions** - active→testing→candidate when constraints violated
4. **Global status changes** - handle pending/paused/deleted monitors

### Interface Contract
```go
// What the selector expects:
// - server_scores entries exist for all monitors that should be considered
// - Initial status is 'candidate' for new assignments
// - External code handles constraint compliance for initial assignments

// What the selector provides:
// - Promotes healthy candidates to testing
// - Promotes good testing monitors to active
// - Demotes violating monitors appropriately
// - Respects global monitor status changes
```

## Risk Assessment

### High Risk
- **Incomplete understanding of assignment code** - need to verify it handles all cases
- **Bootstrap scenarios** - what happens when no server_scores entries exist?
- **Performance impact** - removing available pool might reduce discovery

### Medium Risk
- **Gradual transitions** - pending monitors may not be properly handled
- **Metrics compatibility** - dashboards may expect "new" status counts
- **Testing coverage** - complex selection scenarios may not be tested

### Low Risk
- **Database performance** - fewer queries should improve performance
- **Code maintainability** - simpler logic reduces bugs
- **Constraint violations** - should be eliminated entirely

## Mitigation Strategies

### For High Risks:
1. **Document assignment code thoroughly** before making changes
2. **Add bootstrap detection** - if no server_scores exist, log warning but don't crash
3. **Monitor assignment rate** - ensure external code keeps candidate pools full

### For Medium Risks:
1. **Update metrics queries** to handle missing "new" status
2. **Add comprehensive integration tests** for edge cases
3. **Implement gradual rollout** with ability to revert

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

## Conclusion

This architectural simplification will:
- **Eliminate the persistent constraint bug** by removing unnecessary constraint checks
- **Simplify the codebase** by removing complex "available pool" logic
- **Improve maintainability** by clarifying responsibility boundaries
- **Leverage existing production code** that already handles assignment policy

The change is low-risk because it removes complexity rather than adding it, and the existing assignment code is already proven in production.
