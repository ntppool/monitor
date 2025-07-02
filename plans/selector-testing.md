# Selector Test Coverage Improvement Plan

## Executive Summary

The recent refactoring of the selector package's `applySelectionRules` function introduced critical bugs that were caught by failing tests. Analysis revealed that the bugs stemmed from **safety variable scope creep** and **over-restrictive emergency conditions**. Current test coverage is 53.6% with several critical functions having inadequate coverage.

This plan provides a systematic approach to improve test coverage to 80%+ and ensure the refactored architecture is thoroughly validated.

## Background & Root Cause Analysis

### Bugs Introduced During Refactoring

1. **Safety Variable Repurposing**: `maxRemovals = 0` was originally intended to limit active monitor demotions only, but became a trigger for blocking ALL changes including necessary constraint cleanup.

2. **Emergency Condition Over-Restriction**: Emergency blocking logic changed from simple active monitor conditions to complex compound conditions requiring zero healthy monitors in both active AND testing pools, preventing legitimate constraint demotions.

3. **Lost Specificity**: Safety checks were abstracted into `calculateSafetyLimits()` but then reused for broader blocking than originally intended.

### Failed Tests
- `TestDynamicTestingPoolWithConstraints`: Expected 1 constraint demotion, got 0
- `TestIterativeAccountLimitEnforcement`: Expected 2 promotions, got 0

Both tests were getting 0 changes instead of expected changes due to overly aggressive early returns.

## Current Test Coverage Analysis

**Overall Coverage**: 53.6% of statements

### Critical Refactored Functions Coverage:
- `applySelectionRules`: 98.2% ✅ (good coverage)
- `calculateSafetyLimits`: 84.6% ⚠️ (needs improvement)
- `applyRule6BootstrapPromotion`: **13.6%** ❌ (very low coverage)
- `applyRule1ImmediateBlocking`: 75.0% ⚠️ (could be better)
- `applyRule2GradualConstraintRemoval`: 87.5% ⚠️ (good but could be complete)
- `applyRule3TestingToActivePromotion`: 100.0% ✅
- `applyRule5CandidateToTestingPromotion`: 100.0% ✅

### Completely Untested Critical Functions (0% coverage):
- `loadServerInfo`: Database loading logic
- `applyStatusChange`: Actual status change application
- `buildAccountLimitsFromMonitors`: Account limit calculation
- All metrics tracking functions
- `isGrandfathered`: Constraint grandfathering logic

## Implementation Plan

### Phase 1: Critical Safety Logic Testing (Priority: High, Week 1)

**Goal**: Achieve 100% coverage of safety logic and emergency conditions

#### 1.1 Test calculateSafetyLimits Edge Cases
**Current Coverage**: 84.6% → Target: 100%

**Test Scenarios**:
```go
// Test emergency safeguards
func TestCalculateSafetyLimits_EmergencyConditions(t *testing.T) {
    tests := []struct {
        name                string
        targetNumber        int
        totalMonitors       int
        healthyActive       int
        activeCount         int
        expectedMaxRemovals int
        expectEmergency     bool
    }{
        {
            name: "emergency_not_enough_monitors",
            targetNumber: 7, totalMonitors: 4,
            healthyActive: 2, activeCount: 3,
            expectedMaxRemovals: 0, expectEmergency: true,
        },
        {
            name: "safety_below_target_unhealthy",
            targetNumber: 7, totalMonitors: 10,
            healthyActive: 2, activeCount: 3,
            expectedMaxRemovals: 0, expectEmergency: false,
        },
        {
            name: "normal_operation_above_target",
            targetNumber: 7, totalMonitors: 10,
            healthyActive: 8, activeCount: 8,
            expectedMaxRemovals: 2, expectEmergency: false,
        },
    }
    // Test each scenario...
}
```

**Key Test Cases**:
- `targetNumber > len(evaluatedMonitors)` scenarios
- `healthyActive < activeCount` boundary conditions
- Combinations of zero/low healthy active vs testing monitors
- Safety limits with constraint violations present
- Verify `maxRemovals = 0` only blocks active demotions, not constraint cleanup

#### 1.2 Test Emergency Return Logic Integration
**Goal**: Ensure emergency conditions don't block legitimate operations

**Test Scenarios**:
```go
func TestEmergencyConditions_ConstraintProcessing(t *testing.T) {
    // Test: Constraint demotions should proceed even when emergency conditions trigger
    // Test: Normal promotions should work when below target but have healthy monitors
    // Test: True emergency scenarios block all changes appropriately
    // Test: Constraint violations + emergency conditions combinations
}
```

**Files to Modify**: `process_test.go`

### Phase 2: Bootstrap and Edge Case Coverage (Priority: High, Week 2)

#### 2.1 Test applyRule6BootstrapPromotion
**Current Coverage**: 13.6% → Target: 90%+

**Critical Test Scenarios**:
```go
func TestApplyRule6BootstrapPromotion(t *testing.T) {
    tests := []struct {
        name                string
        testingMonitors     []evaluatedMonitor
        candidateMonitors   []evaluatedMonitor
        expectedPromotions  int
        expectedReason      string
    }{
        {
            name: "zero_testing_promotes_candidates",
            testingMonitors: []evaluatedMonitor{},
            candidateMonitors: createHealthyCandidates(3),
            expectedPromotions: 3,
        },
        {
            name: "bootstrap_with_account_constraints",
            testingMonitors: []evaluatedMonitor{},
            candidateMonitors: createCandidatesWithAccountLimits(),
            expectedPromotions: 2, // limited by account
        },
        {
            name: "bootstrap_with_network_constraints",
            testingMonitors: []evaluatedMonitor{},
            candidateMonitors: createCandidatesWithNetworkConflicts(),
            expectedPromotions: 1, // limited by network diversity
        },
    }
}
```

#### 2.2 Test applyRule1ImmediateBlocking
**Current Coverage**: 75% → Target: 95%+

**Test Cases**:
- Monitors marked `candidateBlock` from active status
- Monitors marked `candidateBlock` from testing status
- Empty monitor lists and edge cases
- Limit interactions with immediate blocking

**Files to Create**: `bootstrap_test.go`

### Phase 3: Integration and State Management Testing (Priority: Medium, Week 3)

#### 3.1 Test Working Count State Management
**Goal**: Verify state consistency across rule execution

**Test Framework**:
```go
func TestWorkingCountConsistency(t *testing.T) {
    // Create test harness that tracks working counts after each rule
    initialState := createInitialState()

    // Apply each rule and verify counts
    state1 := applyRule1AndValidateCounts(initialState)
    state2 := applyRule2AndValidateCounts(state1)
    // ... continue for all rules

    // Verify final counts match expected outcomes
    validateFinalState(finalState, expectedCounts)
}
```

#### 3.2 Test Rule Execution Order Dependencies
**Goal**: Verify rule order doesn't break functionality

**Test Approach**:
```go
func TestRuleOrderSensitivity(t *testing.T) {
    // Test critical paths:
    // - Rule 2 → Rule 3 interactions (constraint demotions → promotions)
    // - Rule 1.5 → Rule 3 interactions (active demotions → promotions)

    scenario := createComplexScenario()

    // Test normal order
    normalResult := executeRulesInOrder(scenario, normalOrder)

    // Test that changing order doesn't break essential functionality
    // (Some order dependencies are intentional and should be documented)
}
```

**Files to Create**: `rule_order_test.go`, expand `integration_test.go`

### Phase 4: Untested Infrastructure (Priority: Medium, Week 4)

#### 4.1 Test Database Integration Functions
**Current Coverage**: 0% → Target: 80%+

**Critical Functions**:
```go
func TestLoadServerInfo(t *testing.T) {
    // Test with valid/invalid server IDs
    // Test database connection failures
    // Test malformed server data
}

func TestApplyStatusChange(t *testing.T) {
    // Test actual database status updates
    // Test transaction rollbacks on failure
    // Test concurrent update scenarios
}

func TestBuildAccountLimitsFromMonitors(t *testing.T) {
    // Test account limit calculation from monitor data
    // Test edge cases with missing account data
    // Test performance with large monitor sets
}
```

#### 4.2 Test Metrics and Tracking
**Current Coverage**: 0% → Target: 80%+

**Test Framework**:
```go
func TestMetricsTracking(t *testing.T) {
    // Mock metrics collector
    mockMetrics := &MockMetrics{}
    selector := NewSelector(mockMetrics)

    // Execute selector operations
    selector.ProcessServer(testScenario)

    // Verify expected metrics were recorded
    assert.Equal(t, expectedStatusChanges, mockMetrics.StatusChanges)
    assert.Equal(t, expectedConstraintViolations, mockMetrics.Violations)
}
```

**Files to Create**: `metrics_test.go`

### Phase 5: Complex Scenario Integration Testing (Priority: Low, Week 5)

#### 5.1 Multi-Constraint Scenarios
**Goal**: Test realistic production scenarios

**Test Scenarios**:
```go
func TestComplexProductionScenarios(t *testing.T) {
    scenarios := []struct {
        name        string
        setup       func() testScenario
        validate    func([]statusChange) error
    }{
        {
            name: "account_limits_plus_network_diversity_plus_safety",
            setup: createMultiConstraintScenario,
            validate: validateConstraintInteractions,
        },
        {
            name: "constraint_violations_during_emergency_override",
            setup: createEmergencyWithViolations,
            validate: validateEmergencyConstraintHandling,
        },
        {
            name: "gradual_removal_with_simultaneous_promotions",
            setup: createGradualRemovalScenario,
            validate: validateGradualRemovalFlow,
        },
    }
}
```

#### 5.2 Performance and Stress Testing
**Goal**: Verify refactoring didn't introduce performance regressions

**Benchmarks**:
```go
func BenchmarkApplySelectionRules(b *testing.B) {
    scenarios := []struct {
        name         string
        monitorCount int
    }{
        {"small_10_monitors", 10},
        {"medium_50_monitors", 50},
        {"large_100_monitors", 100},
        {"xlarge_500_monitors", 500},
    }

    for _, scenario := range scenarios {
        b.Run(scenario.name, func(b *testing.B) {
            testData := generateMonitors(scenario.monitorCount)
            b.ResetTimer()
            for i := 0; i < b.N; i++ {
                applySelectionRules(testData)
            }
        })
    }
}
```

**Files to Create**: `benchmark_test.go`

## Success Criteria

### Coverage Targets
- **Overall selector package coverage: 80%+** (from 53.6%)
- **Critical refactored functions: 95%+ coverage**
- **All emergency/safety logic paths: 100% coverage**
- **Database integration functions: 80%+ coverage**

### Quality Gates
- All existing tests continue to pass
- No performance regression > 10% for typical workloads
- All edge cases from bug analysis are covered with tests
- Integration tests cover multi-rule interactions
- Documentation of intentional vs accidental rule order dependencies

## Implementation Guidelines

### Test Development Best Practices

#### 1. Use Table-Driven Tests
```go
func TestSafetyConditions(t *testing.T) {
    tests := []struct {
        name           string
        input          testInput
        expectedResult expectedResult
        expectError    bool
    }{
        // Test cases...
    }

    for _, tt := range tests {
        t.Run(tt.name, func(t *testing.T) {
            // Test implementation
        })
    }
}
```

#### 2. Create Comprehensive Test Helpers
```go
// Helper functions for creating test data
func createHealthyMonitors(count int, status ntpdb.ServerScoresStatus) []evaluatedMonitor
func createMonitorsWithViolations(violationType constraintViolationType) []evaluatedMonitor
func createAccountLimitsScenario(limits map[uint32]int) map[uint32]*accountLimit

// Helper functions for validation
func validateWorkingCounts(result ruleResult, expected workingCounts) error
func validateConstraintViolations(changes []statusChange, expected []violationType) error
```

#### 3. Mock External Dependencies
```go
// Mock database for testing database integration
type MockDatabase struct {
    servers   map[uint32]*serverInfo
    changes   []statusChange
    failNext  bool
}

// Mock metrics for testing observability
type MockMetrics struct {
    StatusChanges       []statusChange
    ConstraintViolations []constraintViolation
    PoolSizes          []poolSizeMetric
}
```

### Coverage Measurement

#### Run Coverage Analysis
```bash
# Generate coverage profile
go test -coverprofile=coverage.out ./selector

# View coverage by function
go tool cover -func=coverage.out | grep -E "(applyRule|calculateSafety|applySelection)"

# Generate HTML coverage report
go tool cover -html=coverage.out -o coverage.html

# Set coverage targets in CI
go tool cover -func=coverage.out | tail -1 | awk '{print $3}' | sed 's/%//' | awk '{if($1<80) exit 1}'
```

#### Target Coverage by File
- `process.go`: 90%+ (contains core refactored logic)
- `constraints.go`: 85%+ (constraint checking logic)
- `selector.go`: 70%+ (main entry points, some are integration-only)
- `state.go`: 95%+ (state management is critical)

## Risk Mitigation

### Preventing Regressions During Test Development
1. **Run existing test suite after each phase** to catch any accidental breaking changes
2. **Use feature flags for new test scenarios** to allow incremental rollout
3. **Maintain backwards compatibility** in test helpers and utilities
4. **Document any discovered architectural issues** for future refactoring consideration

### Managing Test Complexity
1. **Start with simple unit tests** before complex integration scenarios
2. **Use dependency injection** to make functions more testable
3. **Create focused test files** rather than adding everything to existing files
4. **Maintain clear separation** between unit tests and integration tests

### Performance Considerations
1. **Use parallel test execution** where possible (`t.Parallel()`)
2. **Optimize test data generation** for large scenario tests
3. **Profile test execution time** to identify slow tests
4. **Use build tags** to separate performance tests from unit tests

## Timeline & Milestones

### Week 1: Critical Safety Logic (Phase 1)
- **Deliverable**: 100% coverage of `calculateSafetyLimits` and emergency conditions
- **Milestone**: All safety logic edge cases tested and validated
- **Risk**: If not completed, production deployment confidence remains low

### Week 2: Bootstrap and Edge Cases (Phase 2)
- **Deliverable**: 90%+ coverage of `applyRule6BootstrapPromotion` and Rule 1
- **Milestone**: All low-coverage critical functions brought to acceptable levels
- **Risk**: Bootstrap scenarios are complex; may need additional time

### Week 3: Integration and State Management (Phase 3)
- **Deliverable**: Working count consistency tests and rule order validation
- **Milestone**: Refactored architecture proven sound through integration tests
- **Risk**: May discover architectural issues requiring design changes

### Week 4: Infrastructure Testing (Phase 4)
- **Deliverable**: Database integration and metrics testing at 80%+ coverage
- **Milestone**: All 0% coverage critical functions tested
- **Risk**: Database test infrastructure may require significant setup

### Week 5: Complex Scenarios and Performance (Phase 5)
- **Deliverable**: Production scenario tests and performance benchmarks
- **Milestone**: 80%+ overall package coverage achieved
- **Risk**: Performance regression discovery may require optimization work

## Maintenance and Continuous Improvement

### Integration with CI/CD
```yaml
# Add to CI pipeline
- name: Test Coverage Check
  run: |
    go test -coverprofile=coverage.out ./selector
    COVERAGE=$(go tool cover -func=coverage.out | tail -1 | awk '{print $3}' | sed 's/%//')
    if [ $COVERAGE -lt 80 ]; then
      echo "Coverage $COVERAGE% is below 80% threshold"
      exit 1
    fi
```

### Regular Review Process
- **Monthly coverage review**: Track coverage trends and identify gaps
- **Quarterly test effectiveness review**: Analyze if tests catch real bugs
- **Post-incident test addition**: Add tests for any production issues discovered

### Documentation Updates
- Update `CLAUDE.md` with new testing patterns and requirements
- Document any architectural decisions made during testing
- Maintain test case documentation for complex scenarios

---

## Conclusion

This comprehensive testing plan addresses the specific architectural issues that caused the refactoring bugs while providing a systematic approach to achieving production-ready test coverage. The phased approach ensures that the most critical safety logic is tested first, while the complete plan provides confidence in the refactored selector architecture.

The focus on safety logic, emergency conditions, and constraint handling directly addresses the root causes of the original bugs, while the broader coverage improvements ensure that future refactoring efforts will be better supported by comprehensive test coverage.
