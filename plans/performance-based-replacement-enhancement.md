# Performance-Based Replacement Enhancement Plan

## Executive Summary

### Problem Statement
The current monitor selection system has an inconsistency in performance-based replacement logic:
- **Rule 5** (candidate→testing): Successfully implements both capacity-based promotion AND performance-based replacement
- **Rule 3** (testing→active): Only implements capacity-based promotion, missing performance-based replacement

This results in better-performing testing monitors being unable to replace worse-performing active monitors, even when all constraints are satisfied.

### Evidence
**Server 53979** logs show successful candidate-testing replacements:
```
mon-68 (candidate) → testing ("replacement promotion")
mon-64 (testing) → candidate ("replaced by better candidate")
```

However, clear performance inversions persist:
- `mon-3yj8077` (priority 18, testing) should replace `mon-2gcp0xq` (priority 136, active)
- Multiple candidates have better priorities than current active monitors

**Server 61266** shows zero planned changes despite obvious opportunities:
- `mon-1h0ee62` (priority 11, candidate) significantly outperforms `mon-31ehtkb` (priority 13, active)
- System logs: `plannedChanges=0 appliedChanges=0` every 20 minutes

### Goals
1. **Add active-testing swaps** to Rule 3 to match Rule 5's pattern
2. **Improve priority comparison** to use database-calculated monitor_priority field
3. **Maintain constraint compliance** and conservative change limits
4. **Preserve incremental approach** (20-60 minute intervals are appropriate)

### Expected Outcomes
- Better performing monitors will gradually replace worse performing ones
- Proper priority-based ordering across all monitor states
- Consistent replacement logic between Rule 3 and Rule 5
- No regression in existing candidate-testing replacement functionality

## Current State Analysis

### Rule Architecture Comparison

**Rule 3 (Testing → Active)**:
- ✅ Phase 1: Fill empty active slots with best testing monitors
- ❌ Phase 2: Performance-based replacement (MISSING)

**Rule 5 (Candidate → Testing)**:
- ✅ Phase 1: Fill empty testing slots (respecting dynamic testing target)
- ✅ Phase 2: Performance-based replacement via `attemptTestingReplacements()`

### Code Locations
- **Rule 3**: `selector/process.go:248-302` (`applyRule3TestingToActivePromotion`)
- **Rule 5**: `selector/process.go:304-380` (`applyRule5CandidateToTestingPromotion`)
- **Existing replacement logic**: `selector/process.go:959-1067` (`attemptTestingReplacements`)

### Performance Comparison Logic Issues
Current `candidateOutperformsTestingMonitor()` in `selector/helpers.go:153-168`:
- Only compares RTT as performance indicator
- Doesn't use database-calculated `monitor_priority` field
- Health comparison is basic but adequate

Database provides `monitor_priority` calculated as:
```sql
round((avg(ls.rtt)/1000) * (1+(2 * (1-avg(ls.step)))))
```
This incorporates both RTT and step accuracy, making it the definitive performance metric.

## Technical Requirements

### 1. Data Structure Enhancement
**File**: `selector/types.go`

Add Priority field to `monitorCandidate` struct:
```go
type monitorCandidate struct {
    // ... existing fields
    Priority float64 // From database: monitor_priority field
}
```

### 2. Database Field Extraction
**File**: `selector/tracking.go`

Modify `convertMonitorPriorityToCandidate()` to capture priority:
```go
// Add after RTT extraction
if priority, ok := row.MonitorPriority.(float64); ok {
    candidate.Priority = priority
} else {
    // If priority is NULL, mark monitor as unhealthy
    candidate.IsHealthy = false
    sl.log.WarnContext(ctx, "monitor has NULL priority, marking as unhealthy",
        slog.String("monitor_id", row.MonitorID),
        slog.String("server_id", serverID),
    )
}
```

### 3. Performance Comparison Enhancement
**File**: `selector/helpers.go`

Create unified `monitorOutperformsMonitor()` function with performance threshold:
```go
func (sl *Selector) monitorOutperformsMonitor(better, worse evaluatedMonitor) bool {
    // First compare by health - healthy monitors are always better
    if better.monitor.IsHealthy && !worse.monitor.IsHealthy {
        return true
    }
    if !better.monitor.IsHealthy && worse.monitor.IsHealthy {
        return false
    }

    // Calculate performance improvement
    priorityDiff := worse.monitor.Priority - better.monitor.Priority
    percentImprovement := (priorityDiff / worse.monitor.Priority) * 100

    // Require significant improvement: 5% AND at least 5 priority points
    return percentImprovement >= 5.0 && priorityDiff >= 5.0
}
```

Update existing `candidateOutperformsTestingMonitor()` to use new function:
```go
func (sl *Selector) candidateOutperformsTestingMonitor(candidate, testing evaluatedMonitor) bool {
    return sl.monitorOutperformsMonitor(candidate, testing)
}
```

### 4. Active Replacement Function
**File**: `selector/process.go`

Create `attemptActiveReplacements()` function following the same pattern as `attemptTestingReplacements()`.

## Detailed Implementation Plan

### Phase 1: Add Missing Phase 2 to Rule 3

**Why Phase 1 First?**
- Lower risk: Smaller, focused change
- Easier debugging: Can isolate issues in Rule 3 enhancement
- Better testing: Validate independently before refactoring
- Clearer git history: "add missing feature" separate from "refactor for consistency"

#### Step 1: Data Structure Changes

**Files to modify**:
- `selector/types.go`: Add Priority field
- `selector/tracking.go`: Extract priority from database

**Implementation Details**:
1. Add `Priority float64` field to `monitorCandidate` struct
2. Update `convertMonitorPriorityToCandidate()` to extract priority
3. Handle NULL priority by marking monitor as unhealthy
4. Add warning log for NULL priority cases

**Testing**:
- Unit test to verify priority extraction from database row
- Test NULL priority handling marks monitor as unhealthy
- Integration test with actual database query

#### Step 2: Performance Comparison Enhancement

**File**: `selector/helpers.go`

**Implementation Details**:
1. Create new `monitorOutperformsMonitor()` function:
   - Health comparison first (healthy always beats unhealthy)
   - Calculate both absolute and percentage improvement
   - Require BOTH 5% improvement AND 5+ priority points
2. Update existing `candidateOutperformsTestingMonitor()` to delegate
3. Add detailed logging for replacement evaluation

**Logging Requirements**:
```go
// Log when considering replacement
sl.log.DebugContext(ctx, "evaluating monitor replacement",
    slog.String("better_monitor", better.monitor.IDToken),
    slog.Float64("better_priority", better.monitor.Priority),
    slog.String("worse_monitor", worse.monitor.IDToken),
    slog.Float64("worse_priority", worse.monitor.Priority),
    slog.Float64("priority_diff", priorityDiff),
    slog.Float64("percent_improvement", percentImprovement),
    slog.Bool("meets_threshold", meetsThreshold),
)
```

**Testing**:
- Unit tests for various priority comparisons
- Test edge cases: identical priorities, small differences
- Test health vs priority precedence

#### Step 3: Active Replacement Function

**File**: `selector/process.go`

Create `attemptActiveReplacements()` function:

```go
func (sl *Selector) attemptActiveReplacements(
    ctx context.Context,
    selCtx *monitorSelectionContext,
    testingMonitors []evaluatedMonitor,
    activeMonitors []evaluatedMonitor,
    workingAccountLimits map[string]int,
    budget int,
) []monitorStatusChange {
    var changes []monitorStatusChange
    replacements := 0

    // Log entry
    sl.log.InfoContext(ctx, "attempting active-testing replacements",
        slog.String("server_id", selCtx.serverID),
        slog.Int("testing_count", len(testingMonitors)),
        slog.Int("active_count", len(activeMonitors)),
        slog.Int("budget", budget),
    )

    // Only consider globally-active testing monitors
    var eligibleTesting []evaluatedMonitor
    for _, tm := range testingMonitors {
        if tm.globalStatus == "active" || tm.globalStatus == "testing" {
            eligibleTesting = append(eligibleTesting, tm)
        }
    }

    // Sort testing monitors by priority (best first)
    sort.Slice(eligibleTesting, func(i, j int) bool {
        return eligibleTesting[i].monitor.Priority < eligibleTesting[j].monitor.Priority
    })

    // For each eligible testing monitor
    for _, testingMon := range eligibleTesting {
        if replacements*2 >= budget {
            sl.log.DebugContext(ctx, "active replacement budget exhausted",
                slog.Int("replacements", replacements),
                slog.Int("budget_used", replacements*2),
            )
            break
        }

        // Find worst active monitor that testing outperforms
        var targetActive *evaluatedMonitor
        var targetIdx int

        for idx, activeMon := range activeMonitors {
            // Skip if active has constraint violations (will be removed by other rules)
            if len(activeMon.violations) > 0 {
                continue
            }

            if sl.monitorOutperformsMonitor(testingMon, activeMon) {
                if targetActive == nil || activeMon.monitor.Priority > targetActive.monitor.Priority {
                    targetActive = &activeMon
                    targetIdx = idx
                }
            }
        }

        if targetActive == nil {
            continue
        }

        // Log consideration
        sl.log.InfoContext(ctx, "considering active-testing swap",
            slog.String("testing_monitor", testingMon.monitor.IDToken),
            slog.Float64("testing_priority", testingMon.monitor.Priority),
            slog.String("active_monitor", targetActive.monitor.IDToken),
            slog.Float64("active_priority", targetActive.monitor.Priority),
        )

        // Validate constraints for promotion (only promotion needs validation)
        tempLimits := maps.Clone(workingAccountLimits)

        // Simulate removing active monitor
        if targetActive.monitor.Account != "" {
            tempLimits[targetActive.monitor.Account]--
        }

        // Check if testing can be promoted
        violations := sl.checkPromotionConstraints(ctx, selCtx, testingMon, "active", tempLimits)
        if len(violations) > 0 {
            sl.log.DebugContext(ctx, "active-testing swap blocked by constraints",
                slog.String("testing_monitor", testingMon.monitor.IDToken),
                slog.Any("violations", violations),
            )
            continue
        }

        // Execute swap
        changes = append(changes, monitorStatusChange{
            Monitor:     *targetActive,
            FromStatus:  "active",
            ToStatus:    "testing",
            Reason:      "active-testing swap (demote)",
            ServerID:    selCtx.serverID,
        })

        changes = append(changes, monitorStatusChange{
            Monitor:     testingMon,
            FromStatus:  "testing",
            ToStatus:    "active",
            Reason:      "active-testing swap (promote)",
            ServerID:    selCtx.serverID,
        })

        // Update working limits
        if targetActive.monitor.Account != "" {
            workingAccountLimits[targetActive.monitor.Account]--
        }
        if testingMon.monitor.Account != "" {
            workingAccountLimits[testingMon.monitor.Account]++
        }

        // Remove from active list to prevent double-replacement
        activeMonitors = append(activeMonitors[:targetIdx], activeMonitors[targetIdx+1:]...)

        replacements++

        sl.log.InfoContext(ctx, "planned active-testing swap",
            slog.String("testing_monitor", testingMon.monitor.IDToken),
            slog.String("active_monitor", targetActive.monitor.IDToken),
            slog.Float64("priority_improvement", targetActive.monitor.Priority - testingMon.monitor.Priority),
        )
    }

    return changes
}
```

**Key Features**:
- Only validates promotion constraints (demotions always allowed)
- Uses unified performance comparison with thresholds
- Detailed logging at each decision point
- Updates working account limits atomically
- Removes replaced monitors from consideration

#### Step 4: Integration with Rule 3

**File**: `selector/process.go` in `applyRule3TestingToActivePromotion()`

Add Phase 2 after existing Phase 1 logic:
```go
// Phase 2: Performance-based replacement (new logic)
remainingBudget := selCtx.limits.promotions - promoted
if remainingBudget > 0 && len(testingMonitors) > 0 && len(activeMonitors) > 0 {
    replacementChanges := sl.attemptActiveReplacements(
        ctx, selCtx, testingMonitors, activeMonitors,
        workingAccountLimits, remainingBudget,
    )
    changes = append(changes, replacementChanges...)

    // Log Phase 2 results
    if len(replacementChanges) > 0 {
        sl.log.InfoContext(ctx, "rule 3 phase 2 complete",
            slog.String("server_id", selCtx.serverID),
            slog.Int("swaps", len(replacementChanges)/2),
            slog.Int("total_changes", len(changes)),
        )
    }
}
```

**Budget Management**:
- Active-testing swaps consume 2 units from the shared promotion budget
- Budget is shared across all rules in the selection process
- Remaining budget calculation prevents over-spending
- Conservative limits preserve system stability

### Phase 2: Future Refactoring (Optional)

After Phase 1 is tested and working, consider unified replacement function:

1. **Extract Common Logic**: Create `attemptPerformanceReplacements()`
2. **Parameterize State Transitions**: Handle both active-testing and testing-candidate
3. **Update Both Rules**: Rule 3 and Rule 5 Phase 2 call unified function
4. **Comprehensive Testing**: Ensure no regression

**Benefits**: More maintainable, consistent behavior
**Risks**: Higher complexity, harder debugging
**Decision**: Evaluate after Phase 1 success

## Edge Case Documentation

**To be added to `selector/README.md`**:

### Performance-Based Replacement Edge Cases

1. **Identical Priorities**
   - When monitors have identical priorities, no replacement occurs
   - Existing monitor retains its position (stability principle)

2. **NULL Priority Handling**
   - Monitors with NULL priority are marked as unhealthy
   - Unhealthy monitors cannot participate in replacements
   - Warning logged for debugging

3. **Threshold Edge Cases**
   - Monitor A (priority 100) vs B (priority 95): No replacement (only 5% improvement, needs >5 points)
   - Monitor A (priority 100) vs B (priority 94): No replacement (6% improvement but only 6 points)
   - Monitor A (priority 100) vs B (priority 94.9): Replacement occurs (5.1% and 5.1 points)

4. **Concurrent State Changes**
   - If a monitor's global status changes during selection, next run handles it
   - Selection operates on snapshot of monitor states

5. **Account Limit Edge Cases**
   - Swaps that would violate account limits are skipped
   - Account with 0 monitors can still receive promoted monitors

6. **Emergency Override Behavior**
   - Emergency overrides run in earlier rules (Rule 1)
   - By the time Rule 3 executes, budget may be exhausted
   - This is expected behavior - safety takes precedence

## Risk Assessment & Mitigation

### Technical Risks

**Database Field Extraction**:
- *Risk*: Priority field NULL or unexpected type
- *Mitigation*: NULL priorities mark monitor as unhealthy, log warning
- *Testing*: Integration tests with actual database

**Constraint Checking**:
- *Risk*: Promotion constraint validation complexity
- *Mitigation*: Only validate promotions (demotions always allowed)
- *Testing*: Unit tests for constraint edge cases

**Performance Impact**:
- *Risk*: Additional processing time
- *Mitigation*: Priority already in query, no extra DB hits
- *Testing*: Monitor `selector_process_duration_seconds` metric

### Operational Risks

**Change Frequency**:
- *Risk*: Too many replacements causing instability
- *Mitigation*: 5% + 5 point threshold prevents minor swaps
- *Testing*: Monitor replacement frequency metrics

**Replacement Loops**:
- *Risk*: A replaces B, then B replaces A
- *Mitigation*: Threshold requirement prevents oscillation
- *Testing*: Track monitor pairs over time

### Mitigation Strategies

1. **Code Rollback**: Simple deployment rollback if issues arise
2. **Conservative Thresholds**: 5% + 5 point requirement
3. **Comprehensive Logging**: Every decision point logged
4. **Metrics Collection**: Detailed operational metrics
5. **Integration Tests First**: Validate problem before implementing

## Testing Strategy

### Phase 0: Integration Tests First

**Create integration tests to validate the problem exists**:
```go
func TestRule3MissingPerformanceReplacement(t *testing.T) {
    // Setup: Create monitors with clear priority differences
    // - Active monitor with priority 100
    // - Testing monitor with priority 50 (50% better)
    // Run selector
    // Assert: No replacement occurs (demonstrating the bug)
}
```

This test should FAIL initially, proving the feature is missing.

### Unit Testing

**Priority Comparison Tests**:
```go
func TestMonitorOutperformsMonitor(t *testing.T) {
    tests := []struct {
        name     string
        better   evaluatedMonitor
        worse    evaluatedMonitor
        expected bool
    }{
        {
            name:     "significant improvement",
            better:   evaluatedMonitor{monitor: monitorCandidate{Priority: 50, IsHealthy: true}},
            worse:    evaluatedMonitor{monitor: monitorCandidate{Priority: 100, IsHealthy: true}},
            expected: true, // 50% improvement and 50 points
        },
        {
            name:     "below percentage threshold",
            better:   evaluatedMonitor{monitor: monitorCandidate{Priority: 96, IsHealthy: true}},
            worse:    evaluatedMonitor{monitor: monitorCandidate{Priority: 100, IsHealthy: true}},
            expected: false, // Only 4% improvement
        },
        {
            name:     "below point threshold",
            better:   evaluatedMonitor{monitor: monitorCandidate{Priority: 95, IsHealthy: true}},
            worse:    evaluatedMonitor{monitor: monitorCandidate{Priority: 99, IsHealthy: true}},
            expected: false, // Only 4 points difference
        },
        {
            name:     "healthy beats unhealthy",
            better:   evaluatedMonitor{monitor: monitorCandidate{Priority: 200, IsHealthy: true}},
            worse:    evaluatedMonitor{monitor: monitorCandidate{Priority: 10, IsHealthy: false}},
            expected: true, // Health overrides priority
        },
    }

    for _, tt := range tests {
        t.Run(tt.name, func(t *testing.T) {
            result := sl.monitorOutperformsMonitor(tt.better, tt.worse)
            if result != tt.expected {
                t.Errorf("expected %v, got %v", tt.expected, result)
            }
        })
    }
}
```

**Active Replacement Tests**:
- Test constraint validation blocks invalid swaps
- Test account limit updates correctly
- Test budget consumption (2 units per swap)
- Test replacement of worst active monitor first
- Test NULL priority handling

**Integration Tests**:
- Verify problem is fixed after implementation
- Test with real database priority calculations
- Verify no regression in existing Rule 5 replacements
- Test interaction with other rules

### Production Validation

**Log Analysis**:
Look for new log messages:
```
"attempting active-testing replacements" server_id=X testing_count=Y active_count=Z budget=B
"considering active-testing swap" testing_monitor=X testing_priority=Y active_monitor=Z active_priority=W
"planned active-testing swap" testing_monitor=X active_monitor=Y priority_improvement=Z
"rule 3 phase 2 complete" server_id=X swaps=Y total_changes=Z
```

**Metrics Collection**:

Add new Prometheus metrics to track replacement behavior:

```go
// In metrics.go or similar
var (
    // Track replacement evaluations
    selectorReplacementEvaluations = prometheus.NewCounterVec(
        prometheus.CounterOpts{
            Name: "selector_replacement_evaluations_total",
            Help: "Total number of replacement evaluations by type",
        },
        []string{"server_id", "replacement_type", "result"},
    )

    // Track successful replacements
    selectorReplacementsApplied = prometheus.NewCounterVec(
        prometheus.CounterOpts{
            Name: "selector_replacements_applied_total",
            Help: "Total number of successful replacements by type",
        },
        []string{"server_id", "replacement_type"},
    )

    // Track priority improvements
    selectorPriorityImprovement = prometheus.NewHistogramVec(
        prometheus.HistogramOpts{
            Name: "selector_priority_improvement",
            Help: "Priority improvement from replacements",
            Buckets: []float64{5, 10, 20, 50, 100, 200},
        },
        []string{"server_id", "replacement_type"},
    )
)
```

**Example Queries**:
```promql
# Replacement success rate
rate(selector_replacements_applied_total[5m]) / rate(selector_replacement_evaluations_total[5m])

# Average priority improvement
histogram_quantile(0.5, selector_priority_improvement)

# Active-testing swap frequency
rate(selector_replacements_applied_total{replacement_type="active_testing"}[1h])
```

**Server-Specific Validation**:
- **Server 53979**: Should see priority 18 testing monitor promoted over priority 136 active
- **Server 61266**: Should see candidates replace active monitors over multiple runs
- **General**: Gradual improvement in priority ordering

### Note on Batch Processing

The initial review mentioned "batch processing" as a potential optimization. This referred to evaluating all possible replacements first, then selecting the best ones within budget, rather than the current greedy approach. However, the current implementation pattern (process replacements one at a time in priority order) is simpler and sufficient for the conservative change limits in use. The greedy approach also matches the existing Rule 5 implementation, maintaining consistency.

## Success Criteria

### Immediate Success (First 24 Hours)
1. **No errors**: New code deploys without runtime errors
2. **Basic functionality**: Active-testing swaps appear in logs
3. **Constraint compliance**: All swaps respect account limits and network constraints
4. **No regression**: Existing candidate-testing replacements continue working

### Short-term Success (1 Week)
1. **Performance improvements**: Better monitors replace worse ones on target servers
2. **Proper ordering**: Priority-based ordering improves across multiple servers
3. **Budget management**: Change limits are respected, no over-spending
4. **Stable operation**: No emergency rollbacks required

### Long-term Success (1 Month)
1. **System-wide ordering**: Most servers achieve proper priority-based monitor ordering
2. **Consistent behavior**: Rule 3 and Rule 5 exhibit similar replacement patterns
3. **Operational stability**: System continues to operate within change budgets
4. **Performance metrics**: Overall monitor performance improves measurably

## Implementation Timeline

### Phase 1 Development
**Week 1**: Steps 1-2 (Data structure and performance comparison)
**Week 2**: Step 3 (Active replacement function)
**Week 3**: Step 4 (Integration and testing)
**Week 4**: Production deployment and monitoring

### Phase 1 Validation
**Month 1**: Monitor system behavior, collect metrics
**Month 2**: Evaluate success criteria, plan Phase 2 if beneficial

### Phase 2 Consideration (Optional)
**Month 3+**: Evaluate need for unified replacement function
- If Phase 1 successful and maintainability would benefit from unification
- If additional state transitions need replacement logic
- If development team capacity allows for refactoring project

## Implementation Guide

### Phase 1 Step-by-Step Implementation

#### Step 0: Write Integration Test (1 hour)
1. Create `selector/process_test.go` test demonstrating the missing feature
2. Test should fail, proving Rule 3 lacks performance replacement
3. Keep test for validation after implementation

#### Step 1: Data Structure Changes (2 hours)
1. Add `Priority float64` to `monitorCandidate` in `selector/types.go`
2. Update `convertMonitorPriorityToCandidate()` in `selector/tracking.go`:
   - Extract priority from `row.MonitorPriority`
   - Handle NULL by marking monitor unhealthy
   - Add warning log for NULL cases
3. Run unit tests to verify priority extraction

#### Step 2: Performance Comparison (3 hours)
1. Add `monitorOutperformsMonitor()` to `selector/helpers.go`:
   - Implement health comparison logic
   - Calculate percentage and absolute improvement
   - Enforce 5% AND 5+ point threshold
   - Add debug logging
2. Update `candidateOutperformsTestingMonitor()` to delegate
3. Write comprehensive unit tests for edge cases
4. Verify all tests pass

#### Step 3: Active Replacement Function (4 hours)
1. Add `attemptActiveReplacements()` to `selector/process.go`:
   - Copy structure from `attemptTestingReplacements()`
   - Filter for eligible monitors
   - Implement replacement logic with logging
   - Handle constraint validation (promotions only)
   - Update working account limits
2. Add comprehensive logging at each decision point
3. Write unit tests for the function
4. Test constraint validation edge cases

#### Step 4: Integration (2 hours)
1. Modify `applyRule3TestingToActivePromotion()`:
   - Calculate remaining budget after Phase 1
   - Add Phase 2 call to `attemptActiveReplacements()`
   - Add completion logging
2. Run integration test from Step 0 - should now pass
3. Run full test suite to check for regressions

#### Step 5: Metrics Implementation (2 hours)
1. Add metrics to `selector/metrics.go`:
   - `selector_replacement_evaluations_total`
   - `selector_replacements_applied_total`
   - `selector_priority_improvement`
2. Instrument `attemptActiveReplacements()` with metrics
3. Update Grafana dashboards to visualize new metrics

#### Step 6: Documentation (1 hour)
1. Update `selector/README.md` with edge cases section
2. Add metrics documentation
3. Update any relevant API documentation

### Validation Checklist

Before deployment:
- [ ] Integration test passes (Step 0 test now succeeds)
- [ ] All unit tests pass
- [ ] No regression in existing tests
- [ ] Logging appears at appropriate levels
- [ ] Metrics are properly registered
- [ ] Code formatted with `gofumpt`
- [ ] No race conditions detected

### Production Deployment

1. Deploy to test environment first
2. Monitor logs for new replacement messages
3. Verify metrics are being collected
4. Check specific servers (53979, 61266) for expected behavior
5. Monitor for 24 hours before production deployment

## Conclusion

This enhancement addresses a clear inconsistency in the monitor selection system by adding the missing performance-based replacement capability to Rule 3. The phased approach minimizes risk while achieving the goal of proper priority-based monitor ordering.

The conservative change limits and incremental execution model (every 20-60 minutes) are preserved, ensuring system stability while allowing gradual optimization over time.

Expected outcome: Better-performing monitors will systematically replace worse-performing ones, resulting in improved overall NTP monitoring quality for the pool.
