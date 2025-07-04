# Performance Optimizations TODO

**Last Updated**: 2025-01-26

## Overview

This document consolidates outstanding performance optimization tasks identified across the NTP Pool monitoring system, with focus on the selector package and related components.

## Helper Function Extraction ✅ [COMPLETED - 2025-01]

### Current Status: Completed ✅
**Implementation**: Commit 6c4ae72 - Helper function centralization
**Result**: 47% code reduction in promotion logic
**Completion Date**: January 2025

### Completed Work
- ✅ Extracted `attemptPromotion()` - Unified promotion logic with count tracking
- ✅ Extracted `getEmergencyReason()` - Consistent emergency reason generation
- ✅ Extracted `filterMonitorsByGlobalStatus()` - Monitor filtering by global status
- ✅ Extracted `filterBootstrapCandidates()` - Bootstrap candidate separation
- ✅ Extracted `createCandidateGroups()` - Ordered candidate group creation

### Benefits Achieved
- **Reduced Code Duplication**: ~200 lines reduced to ~100 lines in promotion logic
- **Consistent Logic**: Emergency handling unified across all promotion types
- **Easier Testing**: Helper functions can be unit tested independently
- **Better Maintainability**: Changes to promotion logic only needed in one place
- **Improved Readability**: Business logic clearer without repetitive boilerplate

## Constraint Evaluation Optimization ✅ [COMPLETED - 2024]

### Previous Issue: Unnecessary Constraint Evaluation
**Problem**: ~80% performance overhead from evaluating all 4 possible transitions for every monitor
**Solution**: Lazy evaluation - only check constraints when needed for decisions

### Implementation Results
- **Performance Gain**: 80% reduction in constraint checks (60 → 12 per server)
- **Memory Optimization**: Eliminated allocation for unused transition evaluations
- **Code Clarity**: Cleaner, more maintainable constraint checking logic

### Implementation Pattern
```go
// OLD: Evaluate all transitions
for _, transition := range allPossibleTransitions {
    violation := checkConstraints(monitor, transition.targetState)
}

// NEW: Lazy evaluation
if isPromoting(monitor) {
    violation := checkConstraints(monitor, targetState)
}
```

## Emergency Override Consistency ✅ [COMPLETED - commit: b6515b8]

### Previous Issue
**Problem**: Emergency override only applied to testing→active promotions, not candidate→testing
**Impact**: System could get stuck with zero monitors if candidates couldn't be promoted

### Completed Implementation
- ✅ Added `emergencyOverride` parameter to `canPromoteToTesting()`
- ✅ Updated all call sites to pass emergency override status
- ✅ Applied consistent emergency logic across all promotion paths
- ✅ Added comprehensive test coverage for emergency scenarios

### Performance Impact
- **Positive**: Reduced system recovery time during emergencies
- **Neutral**: No additional computational overhead
- **Critical**: Prevented system deadlock scenarios

## Grandfathering Logic Enhancement 📋 [TODO]

### Current Issue: Non-Functional Grandfathering
**Problem**: Grandfathered and non-grandfathered violations have identical behavior
```go
if violation.Type != violationNone {
    if violation.IsGrandfathered {
        return candidateOut  // Same as non-grandfathered
    }
    return candidateOut      // Same behavior
}
```

### Performance Optimization Opportunities
1. **Tiered Removal Rates**: Grandfathered violations removed more slowly
2. **Priority Queuing**: Non-grandfathered violations get higher removal priority
3. **Batch Processing**: Group similar violations for more efficient processing

### Proposed Implementation
```go
func (sl *Selector) determineRemovalPriority(violation *constraintViolation) removalPriority {
    if violation.IsGrandfathered {
        // Slower removal rate for grandfathered violations
        return removalPriorityLow
    }
    return removalPriorityNormal
}

// Apply different change limits based on priority
func (sl *Selector) calculateRemovalLimits(violations []violation) map[removalPriority]int {
    return map[removalPriority]int{
        removalPriorityLow:    1, // Slower removal of grandfathered
        removalPriorityNormal: 2, // Normal removal rate
        removalPriorityHigh:   3, // Fast removal for urgent cases
    }
}
```

## Testing Performance Optimization (Outstanding)

### Current Test Coverage: 53.6%
**Target**: 80%+ overall coverage with 100% safety logic coverage

### Performance Optimization Opportunities

#### Test Execution Speed
```go
// Parallel test execution where possible
func TestConstraintValidation(t *testing.T) {
    t.Parallel() // Enable concurrent test execution

    tests := []struct {
        name string
        // test cases
    }{
        // test definitions
    }

    for _, tt := range tests {
        tt := tt // Capture range variable
        t.Run(tt.name, func(t *testing.T) {
            t.Parallel() // Parallel sub-tests
            // test implementation
        })
    }
}
```

#### Test Data Generation Optimization
```go
// Optimize test data generation for large scenario tests
func generateOptimizedTestData(monitorCount int) *testScenario {
    // Use object pools for frequent allocations
    // Pre-generate common test patterns
    // Cache expensive test setup operations
}
```

#### Benchmark Integration
```go
func BenchmarkSelectorPerformance(b *testing.B) {
    scenarios := []struct {
        name         string
        monitorCount int
        complexity   string
    }{
        {"small_simple", 10, "basic"},
        {"medium_complex", 100, "constraints"},
        {"large_realistic", 1000, "production"},
    }

    for _, scenario := range scenarios {
        b.Run(scenario.name, func(b *testing.B) {
            data := generateBenchmarkData(scenario)
            b.ResetTimer()
            for i := 0; i < b.N; i++ {
                processSelector(data)
            }
        })
    }
}
```

## Database Query Optimization (Future)

### Potential Optimizations

#### Query Consolidation
- **GetMonitorPriority**: Already optimized with single query
- **GetAvailableMonitors**: Planned for removal in "eliminate new status" architecture
- **Batch Operations**: Group multiple status updates into transactions

#### Index Optimization
```sql
-- Ensure optimal indexes exist for constraint checking
CREATE INDEX idx_server_scores_constraint_lookup
ON server_scores (server_id, status, constraint_violation_type);

CREATE INDEX idx_monitors_account_status
ON monitors (account_id, status, ip_version);
```

#### Connection Pool Tuning
```go
// Optimize database connection pool settings
type DBConfig struct {
    MaxOpenConns    int           // Default: 25
    MaxIdleConns    int           // Default: 25
    ConnMaxLifetime time.Duration // Default: 5 minutes
    ConnMaxIdleTime time.Duration // Default: 5 minutes
}
```

## Memory Optimization (Future)

### Object Pool Pattern
```go
// Reuse expensive objects to reduce GC pressure
var (
    candidatePool = sync.Pool{
        New: func() interface{} {
            return make([]evaluatedMonitor, 0, 100)
        },
    }

    statusChangePool = sync.Pool{
        New: func() interface{} {
            return make([]statusChange, 0, 10)
        },
    }
)

func (sl *Selector) processServerOptimized(serverID uint32) error {
    // Get pooled slices
    candidates := candidatePool.Get().([]evaluatedMonitor)
    changes := statusChangePool.Get().([]statusChange)

    defer func() {
        // Reset and return to pool
        candidates = candidates[:0]
        changes = changes[:0]
        candidatePool.Put(candidates)
        statusChangePool.Put(changes)
    }()

    // Use pooled objects for processing
}
```

### String Interning for Metrics
```go
// Intern commonly used metric labels to reduce memory
var labelIntern = map[string]string{
    "active":    "active",
    "testing":   "testing",
    "candidate": "candidate",
    // Pre-allocate common label values
}
```

## Algorithmic Optimizations (Future)

### Early Termination Patterns
```go
// Stop processing when change limits reached
func (sl *Selector) processMonitorsWithEarlyExit(monitors []evaluatedMonitor, maxChanges int) []statusChange {
    changes := make([]statusChange, 0, maxChanges)

    for _, monitor := range monitors {
        if len(changes) >= maxChanges {
            break // Early termination
        }

        if change := sl.evaluateMonitor(monitor); change != nil {
            changes = append(changes, *change)
        }
    }

    return changes
}
```

### Batch Constraint Checking
```go
// Check constraints for multiple monitors simultaneously
func (sl *Selector) batchCheckConstraints(monitors []monitorCandidate, server *serverInfo) map[uint32]*constraintViolation {
    violations := make(map[uint32]*constraintViolation)

    // Group by constraint type for batch processing
    byAccount := groupByAccount(monitors)
    byNetwork := groupByNetwork(monitors)

    // Process each group efficiently
    for accountID, accountMonitors := range byAccount {
        sl.checkAccountConstraintsBatch(accountMonitors, violations)
    }

    for network, networkMonitors := range byNetwork {
        sl.checkNetworkConstraintsBatch(networkMonitors, violations)
    }

    return violations
}
```

## Monitoring and Measurement

### Performance Metrics Collection
```go
// Track optimization effectiveness
type PerformanceMetrics struct {
    SelectorDuration      time.Duration
    ConstraintCheckCount  int
    PromotionAttempts     int
    CacheHitRatio        float64
    MemoryAllocations    int64
}

func (sl *Selector) recordPerformanceMetrics(metrics PerformanceMetrics) {
    sl.metrics.SelectorDurationHistogram.Observe(metrics.SelectorDuration.Seconds())
    sl.metrics.ConstraintChecksCounter.Add(float64(metrics.ConstraintCheckCount))
    sl.metrics.CacheHitRatioGauge.Set(metrics.CacheHitRatio)
}
```

### Optimization Validation
```go
// A/B test performance improvements
func (sl *Selector) benchmarkOptimization(serverID uint32) {
    // Measure baseline performance
    start := time.Now()
    resultOld := sl.processServerOld(serverID)
    durationOld := time.Since(start)

    // Measure optimized performance
    start = time.Now()
    resultNew := sl.processServerOptimized(serverID)
    durationNew := time.Since(start)

    // Validate results are equivalent
    if !resultsEqual(resultOld, resultNew) {
        log.Error("optimization changed results", "server", serverID)
    }

    // Track performance improvement
    improvement := float64(durationOld-durationNew) / float64(durationOld) * 100
    log.Info("optimization performance", "improvement", improvement, "server", serverID)
}
```

## Implementation Priorities

### Immediate (Next Sprint)
1. **Emergency Override Consistency** - Fix candidate→testing promotion gap
2. **Grandfathering Logic Enhancement** - Implement functional grandfathering

### Short Term (Next Quarter)
1. **Test Performance Optimization** - Parallel execution, data generation
2. **Database Query Review** - Index optimization, connection tuning

### Long Term (Next 6 Months)
1. **Memory Optimization** - Object pools, string interning
2. **Algorithmic Optimizations** - Batch processing, early termination
3. **Comprehensive Performance Monitoring** - Metrics collection, A/B testing

## Success Metrics

### Performance Targets
- **Selector Execution Time**: < 100ms per server (current: varies)
- **Memory Usage**: < 50MB peak during processing (target)
- **Test Execution Time**: < 30s for full test suite (current: varies)
- **Database Query Count**: < 5 queries per server selection (current: ~3-4)

### Quality Targets
- **Test Coverage**: 80%+ overall, 100% safety logic
- **Emergency Recovery**: < 5s from zero monitors to restored service
- **Code Maintainability**: < 100 lines of code per promotion rule

This optimization roadmap provides a clear path for improving system performance while maintaining reliability and correctness.
