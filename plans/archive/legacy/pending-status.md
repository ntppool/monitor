# Pending Status Support for Monitor Selector

## Executive Summary

This document outlines a comprehensive plan to extend the monitor selector system (`scorer/cmd/selector.go`) to support "pending" state monitors. Currently, the selector operates on a binary active/testing model. This enhancement will enable a three-stage workflow: pending → testing → active, providing finer-grained control over monitor lifecycle management.

## Current System Analysis

### Architecture Overview

The monitor selector system is a critical component that determines which monitors actively monitor specific NTP servers. It operates through several interconnected components:

#### Database Schema

**monitors table** (`schema.sql:331-351`):
```sql
CREATE TABLE `monitors` (
  `id` int unsigned NOT NULL AUTO_INCREMENT,
  `status` enum('pending','testing','active','paused','deleted') NOT NULL,
  `type` enum('monitor','score') NOT NULL DEFAULT 'monitor',
  `hostname` varchar(255) NOT NULL DEFAULT '',
  `location` varchar(255) NOT NULL DEFAULT '',
  `ip` varchar(40) DEFAULT NULL,
  `ip_version` enum('v4','v6') DEFAULT NULL,
  -- ... other fields
)
```

**server_scores table** (`schema.sql:465-483`):
```sql
CREATE TABLE `server_scores` (
  `id` bigint unsigned NOT NULL AUTO_INCREMENT,
  `monitor_id` int unsigned NOT NULL,
  `server_id` int unsigned NOT NULL,
  `status` enum('new','testing','active') NOT NULL DEFAULT 'new',
  `score_ts` datetime DEFAULT NULL,
  `score_raw` double NOT NULL DEFAULT '0',
  -- ... other fields
  UNIQUE KEY `server_id` (`server_id`,`monitor_id`)
)
```

**servers_monitor_review table** (`schema.sql:612-622`):
```sql
CREATE TABLE `servers_monitor_review` (
  `server_id` int unsigned NOT NULL,
  `next_review` datetime DEFAULT NULL,
  `last_review` datetime DEFAULT NULL,
  `last_change` datetime DEFAULT NULL,
  PRIMARY KEY (`server_id`)
)
```

#### Current Selection Algorithm

The selector operates on the following principles:

1. **Target Capacity**: 5 active monitors per server (`selector.go:162`)
2. **Bootstrap Mode**: When ≤3 active monitors, relaxed requirements (`selector.go:165`)
3. **Health Requirements**: Only promotes monitors with `avg(step) >= 0` (`selector.go:206`)
4. **History Requirements**:
   - Normal mode: 6+ checks required (`selector.go:254`)
   - Bootstrap mode: 3+ checks sufficient (`selector.go:258`)
5. **Change Limits**: Maximum 1-2 status changes per run (`selector.go:305-314`)

#### Current State Machine

**Global Monitor States** (`ntpdb/models.go:56-64`):
- `pending`: Monitor registered but not yet approved
- `testing`: Monitor approved and under evaluation
- `active`: Monitor fully operational
- `paused`: Monitor temporarily disabled
- `deleted`: Monitor removed

**Per-Server Monitor States** (`ntpdb/models.go:143-149`):
- `new`: Initial state, no monitoring data
- `testing`: Collecting performance data
- `active`: Actively monitoring the server

**Internal Candidate States** (`selector.go:91-98`):
- `candidateUnknown`: Undefined state
- `candidateIn`: Should be promoted to active
- `candidateOut`: Should be demoted to testing
- `candidateBlock`: Should be blocked/removed

### Current Limitations

1. **Ignored Global Status**: The selector only considers `server_scores.status`, completely ignoring `monitors.status`
2. **Binary Transitions**: Only handles active ↔ testing transitions
3. **No Pending Support**: Cannot handle monitors waiting for approval
4. **Limited Workflow**: No support for pending → testing → active pipeline
5. **Inconsistent State**: Pending monitors can have server_scores entries but are ignored

### Key Query Analysis

**GetMonitorPriority** (`query.sql:200-217`):
```sql
select m.id, m.tls_name,
    avg(ls.rtt) / 1000 as avg_rtt,
    round((avg(ls.rtt)/1000) * (1+(2 * (1-avg(ls.step))))) as monitor_priority,
    avg(ls.step) as avg_step,
    if(avg(ls.step) < 0, false, true) as healthy,
    m.status as monitor_status, ss.status as status,
    count(*) as count
  from log_scores ls
  inner join monitors m
  left join server_scores ss on (ss.server_id = ls.server_id and ss.monitor_id = ls.monitor_id)
  where
    m.id = ls.monitor_id
  and ls.server_id = ?
  and m.type = 'monitor'
  and ls.ts > date_sub(now(), interval 12 hour)
  group by m.id, m.tls_name, m.status, ss.status
  order by healthy desc, monitor_priority, avg_step desc, avg_rtt;
```

**Critical Issue**: This query already returns `m.status as monitor_status` but the selector ignores it (`selector.go:240`).

## Technical Requirements

### Pending State Semantics

**Definition**: A monitor in "pending" state is:
- Registered in the system but not yet approved for monitoring
- May have historical monitoring data from previous active periods
- Should not be considered for active monitoring until moved to "testing" state
- Can be promoted directly to "testing" state by the selector under certain conditions

### Integration Requirements

1. **Respect Global Monitor Status**: The selector must consider `monitors.status` alongside `server_scores.status`
2. **Pending Promotion Logic**: Define conditions under which pending monitors are promoted to testing
3. **Workflow Consistency**: Ensure pending → testing → active progression is logical and reversible
4. **Backward Compatibility**: Existing active/testing logic must remain unchanged

### Business Logic Requirements

**Pending Monitor Promotion Conditions**:
1. Server has insufficient active monitors (below target)
2. Server has insufficient testing monitors (below minimum threshold)
3. Pending monitor has sufficient historical performance data
4. Pending monitor meets health requirements
5. System allows promotion changes in current cycle

**Priority Ordering**:
1. Healthy active monitors (keep active)
2. Healthy testing monitors ready for promotion
3. Healthy pending monitors with good history
4. Healthy pending monitors with minimal history
5. Unhealthy monitors (demote/block)

## Implementation Strategy

### Phase 1: Extend candidateState Enum

**File**: `scorer/cmd/selector.go`

**Changes**:
```go
type candidateState uint8

const (
    candidateUnknown candidateState = iota
    candidateIn
    candidateOut
    candidateBlock
    candidatePending  // NEW: Monitor should remain in pending state
)
```

**Regeneration**: Run `go generate` to update `candidatestate_enumer.go`

### Phase 2: Modify processServer Logic

**Current Flow** (`selector.go:194-283`):
```go
for _, candidate := range prilist {
    switch candidate.MonitorStatus {
    case ntpdb.MonitorsStatusActive:
        // Existing logic for active monitors
    case ntpdb.MonitorsStatusTesting:
        // Existing logic sets candidateOut
    default:
        // Existing logic sets candidateBlock
    }
}
```

**Enhanced Flow**:
```go
for _, candidate := range prilist {
    switch candidate.MonitorStatus {
    case ntpdb.MonitorsStatusActive:
        // Existing active monitor logic (unchanged)

    case ntpdb.MonitorsStatusTesting:
        // Existing testing monitor logic (unchanged)

    case ntpdb.MonitorsStatusPending:
        // NEW: Pending monitor evaluation logic
        var s candidateState

        switch {
        case healthy == 0:
            s = candidateBlock

        case currentActiveMonitors >= targetNumber && currentTestingMonitors >= bootstrapModeLimit:
            // System at capacity, keep pending
            s = candidatePending

        case candidate.Count <= 3 && currentActiveMonitors > bootstrapModeLimit:
            // Insufficient history unless in bootstrap mode
            s = candidatePending

        case candidate.Count <= 6 && currentActiveMonitors >= targetNumber:
            // Need more history when at capacity
            s = candidatePending

        default: // healthy == 1 and conditions met
            // Promote pending to testing
            s = candidateIn
        }

        newStatus.NewState = s
        nsl = append(nsl, newStatus)
        continue

    default:
        // Existing logic for paused/deleted (candidateBlock)
    }
}
```

### Phase 3: Database Operations Extension

**New Operations Required**:

1. **Promote Pending to Testing**:
```go
// Insert new server_score entry if none exists
err := db.InsertServerScore(sl.ctx, ntpdb.InsertServerScoreParams{
    MonitorID: candidate.ID,
    ServerID:  serverID,
    ScoreRaw:  0,
    CreatedOn: time.Now(),
})

// Set status to testing
err := db.UpdateServerScoreStatus(sl.ctx, ntpdb.UpdateServerScoreStatusParams{
    MonitorID: candidate.ID,
    ServerID:  serverID,
    Status:    ntpdb.ServerScoresStatusTesting,
})
```

2. **Track Pending Promotions**:
```go
type pendingPromotion struct {
    MonitorID uint32
    ServerID  uint32
    Promoted  bool
}
```

### Phase 4: Counter and Metrics Updates

**Enhanced Counters** (`selector.go:180-192`):
```go
currentActiveMonitors := 0
currentTestingMonitors := 0
currentPendingMonitors := 0  // NEW

for _, candidate := range prilist {
    switch candidate.Status.ServerScoresStatus {
    case ntpdb.ServerScoresStatusActive:
        currentActiveMonitors++
    case ntpdb.ServerScoresStatusTesting:
        currentTestingMonitors++
    case ntpdb.ServerScoresStatusNew:
        // Check global monitor status
        if candidate.MonitorStatus == ntpdb.MonitorsStatusPending {
            currentPendingMonitors++
        }
    }
}
```

**Enhanced Candidate Counting** (`selector.go:285-301`):
```go
healthyMonitors := 0
okMonitors := 0
blockedMonitors := 0
pendingMonitors := 0  // NEW

for _, ns := range nsl {
    switch ns.NewState {
    case candidateIn:
        healthyMonitors++
        okMonitors++
    case candidateOut:
        okMonitors++
    case candidateBlock:
        blockedMonitors++
    case candidatePending:  // NEW
        pendingMonitors++
        okMonitors++  // Pending counts as "available"
    }
}
```

### Phase 5: State Transition Logic

**Enhanced Change Processing** (`selector.go:344-366`):

Current logic removes candidates marked for `candidateBlock` and `candidateOut`. Need to add promotion logic for `candidateIn` where current status is not already active.

```go
// Enhanced promotion logic (after removal processing)
for _, ns := range nsl {
    if ns.NewState != candidateIn {
        continue
    }

    // Handle different current states
    switch ns.CurrentStatus {
    case ntpdb.ServerScoresStatusActive:
        // Already active, skip
        continue

    case ntpdb.ServerScoresStatusTesting:
        // Existing promotion logic (unchanged)
        if allowedChanges <= 0 || toAdd <= 0 {
            break
        }
        // Promote testing to active

    case ntpdb.ServerScoresStatusNew:
        // NEW: Handle pending monitor promotion
        if ns.MonitorStatus == ntpdb.MonitorsStatusPending {
            if allowedChanges <= 0 || toAdd <= 0 {
                break
            }
            log.Info("promoting pending monitor", "monitorID", ns.MonitorID)

            // Insert server_score if needed
            db.InsertServerScore(sl.ctx, ntpdb.InsertServerScoreParams{
                MonitorID: ns.MonitorID,
                ServerID:  serverID,
                ScoreRaw:  0,
                CreatedOn: time.Now(),
            })

            // Set to testing (not directly to active)
            db.UpdateServerScoreStatus(sl.ctx, ntpdb.UpdateServerScoreStatusParams{
                MonitorID: ns.MonitorID,
                ServerID:  serverID,
                Status:    ntpdb.ServerScoresStatusTesting,
            })

            changed = true
            ns.CurrentStatus = ntpdb.ServerScoresStatusTesting
            currentTestingMonitors++
            allowedChanges--
            // Note: Don't decrement toAdd yet, let normal promotion handle it
        }
    }
}
```

### Phase 6: Logging and Observability

**Enhanced Logging** (`selector.go:303`):
```go
log.Info("monitor status",
    "ok", okMonitors,
    "healthy", healthyMonitors,
    "active", currentActiveMonitors,
    "testing", currentTestingMonitors,
    "pending", currentPendingMonitors,  // NEW
    "blocked", blockedMonitors)
```

**Debug Logging Enhancement** (`selector.go:321-328`):
```go
for _, ns := range nsl {
    log.Debug("candidate analysis",
        "monitorID", ns.MonitorID,
        "globalStatus", ns.MonitorStatus,     // NEW
        "serverStatus", ns.CurrentStatus,
        "newState", ns.NewState,
        "rtt", ns.RTT,
        "healthy", ns.Healthy,               // NEW (if available)
    )
}
```

## Edge Cases and Considerations

### Edge Case 1: All Monitors Pending

**Scenario**: A new server with only pending monitors.

**Current Behavior**: No monitors would be selected (system ignores pending).

**New Behavior**:
- Bootstrap mode activated (currentActiveMonitors = 0)
- Up to `bootstrapModeLimit` pending monitors promoted to testing
- Relaxed history requirements (3+ checks instead of 6+)

**Implementation**: Ensure bootstrap mode logic properly handles pending monitors.

### Edge Case 2: Pending Monitor with Stale Data

**Scenario**: Monitor was previously active, moved to pending, has old performance data.

**Considerations**:
- Should stale data be used for promotion decisions?
- How to handle monitors with gaps in monitoring history?
- Should there be a "freshness" requirement for promotion?

**Recommendation**: Use existing 12-hour window logic. If no recent data, treat as insufficient history.

### Edge Case 3: Rapid Status Changes

**Scenario**: Monitor status changes between selector runs.

**Race Condition**: Monitor promoted to testing by selector, but globally set to pending between database reads.

**Mitigation**:
- Use database transactions properly
- Add status validation before final status updates
- Log inconsistencies for debugging

### Edge Case 4: Database Consistency

**Scenario**: Monitor has `monitors.status = pending` but `server_scores.status = active`.

**Current State**: System would consider it active (ignores global status).

**New Behavior**: Global status takes precedence. Log inconsistency and demote to testing.

```go
// Add consistency check
if candidate.MonitorStatus == ntpdb.MonitorsStatusPending &&
   currentStatus == ntpdb.ServerScoresStatusActive {
    log.Warn("inconsistent monitor state detected",
        "monitorID", candidate.ID,
        "globalStatus", candidate.MonitorStatus,
        "serverStatus", currentStatus)
    // Force demotion
    newStatus.NewState = candidateOut
}
```

### Edge Case 5: Performance Impact

**Consideration**: Additional logic complexity may impact selector performance.

**Mitigation**:
- Keep decision logic simple and fast
- Avoid additional database queries where possible
- Monitor execution time and optimize if needed

**Measurement**: Add timing metrics around `processServer()` calls.

## Testing Strategy

### Unit Test Requirements

**Test File**: `scorer/cmd/selector_test.go` (to be created)

**Core Test Cases**:

1. **Pending Monitor Promotion**:
```go
func TestPendingMonitorPromotion(t *testing.T) {
    // Setup: Server with 2 active, 1 testing, 2 pending monitors
    // Expected: 1 pending promoted to testing (reaching target 5)
}
```

2. **Bootstrap Mode with Pending**:
```go
func TestBootstrapModeWithPending(t *testing.T) {
    // Setup: New server, only pending monitors
    // Expected: Up to 3 pending monitors promoted
}
```

3. **Capacity Management**:
```go
func TestPendingIgnoredAtCapacity(t *testing.T) {
    // Setup: Server at target capacity (5 active)
    // Expected: Pending monitors remain pending
}
```

4. **Health Requirements**:
```go
func TestUnhealthyPendingBlocked(t *testing.T) {
    // Setup: Pending monitor with avg(step) < 0
    // Expected: Monitor marked candidateBlock
}
```

5. **State Consistency**:
```go
func TestInconsistentStateHandling(t *testing.T) {
    // Setup: Monitor pending globally but active per-server
    // Expected: Demoted to testing, inconsistency logged
}
```

### Integration Test Requirements

**Database Integration**:
- Test with real database schema
- Verify transaction handling
- Test concurrent selector runs

**End-to-End Workflow**:
- Monitor registration → pending state
- Selector promotion → testing state
- Performance evaluation → active state
- Demotion scenarios

### Performance Test Requirements

**Benchmark Tests**:
```go
func BenchmarkSelectorWithPending(b *testing.B) {
    // Measure performance impact of pending logic
    // Compare with current implementation
}
```

**Load Testing**:
- 1000+ servers with mixed monitor states
- Measure selector execution time
- Verify memory usage patterns

### Migration Test Requirements

**Backward Compatibility**:
- Existing deployments with no pending monitors
- Database schema migration validation
- Configuration compatibility

**Forward Compatibility**:
- Future state additions
- API evolution considerations

## Migration and Deployment Strategy

### Database Migration

**Migration Script** (example):
```sql
-- No schema changes required - enum already supports 'pending'
-- But may need to clean up inconsistent data

-- Find and log inconsistent states
SELECT m.id, m.status as global_status, ss.status as server_status, s.id as server_id
FROM monitors m
JOIN server_scores ss ON m.id = ss.monitor_id
JOIN servers s ON s.id = ss.server_id
WHERE m.status = 'pending' AND ss.status IN ('testing', 'active');

-- Optional: Force consistency (backup first!)
-- UPDATE server_scores ss
-- JOIN monitors m ON m.id = ss.monitor_id
-- SET ss.status = 'new'
-- WHERE m.status = 'pending' AND ss.status IN ('testing', 'active');
```

### Feature Flag Strategy

**Implementation**: Use environment variable or database setting.

```go
// In selector.go
func (sl *selector) supportsPendingState() bool {
    setting, err := sl.db.GetSystemSetting(sl.ctx, "selector_pending_support")
    if err != nil {
        return false // Default to disabled
    }
    return setting == "enabled"
}

// In processServer logic
if sl.supportsPendingState() && candidate.MonitorStatus == ntpdb.MonitorsStatusPending {
    // New pending logic
} else {
    // Legacy logic (treat as candidateBlock)
}
```

### Deployment Phases

**Phase 1: Code Deployment**
- Deploy selector with pending support disabled by default
- Monitor for any regressions in existing functionality
- Validate logging and metrics

**Phase 2: Gradual Enablement**
- Enable pending support in test environment
- Monitor selector behavior and performance
- Validate promotion/demotion logic

**Phase 3: Production Rollout**
- Enable pending support in production
- Monitor system health and performance
- Be prepared to disable if issues arise

**Phase 4: Cleanup**
- Remove feature flag after stable operation
- Update documentation and procedures

### Rollback Procedures

**Immediate Rollback**: Disable feature flag
```sql
UPDATE system_settings
SET value = 'disabled'
WHERE `key` = 'selector_pending_support';
```

**Code Rollback**: Deploy previous version
- Previous version ignores pending monitors (safe)
- No data corruption risk
- May need to clean up promoted monitors manually

### Monitoring and Alerting

**New Metrics**:
- `selector_pending_monitors_total`
- `selector_pending_promotions_total`
- `selector_state_inconsistencies_total`
- `selector_execution_time_seconds` (enhanced)

**Alerts**:
- High number of state inconsistencies
- Selector execution time increase > 50%
- Pending promotion failures

## Performance Implications

### Complexity Analysis

**Current Algorithm**: O(n log n) where n = number of candidate monitors
- Sort by priority: O(n log n)
- Process candidates: O(n)
- State updates: O(changes)

**Enhanced Algorithm**: Same complexity, additional constant factors
- Additional state checks per candidate: O(1)
- Enhanced logging: O(n)
- State consistency validation: O(1)

**Expected Impact**: < 10% performance degradation

### Memory Impact

**Additional State**:
- One extra candidateState value: minimal
- Enhanced logging strings: small increase
- Counter variables: negligible

**Expected Impact**: < 1% memory increase

### Database Impact

**Additional Queries**: None (uses existing GetMonitorPriority)

**Modified Operations**:
- InsertServerScore: May increase for pending promotions
- UpdateServerScoreStatus: Same frequency, additional cases

**Expected Impact**: Minimal database load increase

## Risk Assessment

### High Risk Issues

1. **State Inconsistency**: Existing inconsistent data could cause unexpected behavior
   - **Mitigation**: Add validation and logging, provide cleanup scripts

2. **Performance Regression**: Additional logic could slow selector
   - **Mitigation**: Performance testing, monitoring, rollback capability

3. **Logic Errors**: Complex state transitions could have bugs
   - **Mitigation**: Comprehensive testing, gradual rollout

### Medium Risk Issues

1. **Race Conditions**: Concurrent status changes during selection
   - **Mitigation**: Proper transaction handling, status validation

2. **Configuration Drift**: Feature flag management complexity
   - **Mitigation**: Clear procedures, monitoring

### Low Risk Issues

1. **Logging Volume**: Enhanced logging could increase log volume
   - **Mitigation**: Appropriate log levels, retention policies

## Success Criteria

### Functional Requirements

✅ **Primary Goals**:
- Pending monitors are ignored when sufficient capacity exists
- Pending monitors are promoted when capacity is needed
- Existing active/testing logic remains unchanged
- State inconsistencies are detected and resolved

✅ **Secondary Goals**:
- Performance impact < 10%
- Memory impact < 1%
- Comprehensive logging and metrics
- Clean rollback capability

### Verification Methods

1. **Unit Tests**: All test cases pass
2. **Performance Tests**: Benchmarks within acceptable limits
3. **Integration Tests**: End-to-end workflow validation
4. **Production Monitoring**: Metrics show expected behavior

## Future Considerations

### Potential Extensions

1. **Priority-Based Pending**: Different priority levels for pending monitors
2. **Pending Timeout**: Auto-promote pending monitors after time period
3. **Capacity Planning**: Predictive promotion based on trends
4. **Multi-Server Optimization**: Global pending monitor allocation

### API Enhancements

Future APIs could expose pending monitor information:
- Count of pending monitors per server
- Pending promotion recommendations
- State transition history

## Conclusion

This implementation plan provides a comprehensive approach to adding pending state support to the monitor selector while maintaining backward compatibility and system stability. The phased approach allows for careful validation at each step, and the extensive testing strategy ensures robust functionality.

The key insight is that the existing query already provides monitor status information - the implementation primarily involves respecting that status in the selection logic and adding appropriate state transitions for pending monitors.

The feature flag approach enables safe deployment and rollback, while comprehensive monitoring ensures any issues can be quickly detected and resolved.
