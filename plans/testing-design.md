# Testing Strategy and Quality Assurance Design

## Overview

This document outlines the comprehensive testing strategy for the NTP Pool monitoring system, with particular focus on the selector package's complex constraint validation and monitor selection algorithms.

## Testing Philosophy

### Layered Testing Approach

1. **Unit Tests**: Isolated component testing with mocked dependencies
2. **Integration Tests**: Cross-component testing with real database
3. **System Tests**: End-to-end workflow validation
4. **Performance Tests**: Load and benchmark testing
5. **Regression Tests**: Prevent re-introduction of known issues

### Test-Driven Quality Gates

- **Coverage Targets**: 80%+ overall, 95%+ for critical functions
- **Safety Logic**: 100% coverage requirement for all emergency/safety paths
- **Critical Functions**: Database integration, constraint checking, state transitions
- **Performance**: No regression > 10% for typical workloads

## Selector Package Testing Strategy

### Critical Test Areas

#### Constraint Validation Testing
**Priority**: High - Forms the core of monitor selection logic

**Test Scenarios**:
```go
func TestNetworkConstraints(t *testing.T) {
    tests := []struct {
        name       string
        monitorIP  string
        serverIP   string
        shouldPass bool
    }{
        {"same_ipv4_subnet", "192.168.1.10", "192.168.1.20", false},
        {"different_ipv4_subnet", "192.168.1.10", "192.168.2.20", true},
        {"same_ipv6_subnet", "2001:db8:1::10", "2001:db8:1::20", false},
        {"different_ipv6_subnet", "2001:db8:1::10", "2001:db8:2::20", true},
    }
}

func TestAccountConstraints(t *testing.T) {
    tests := []struct {
        name           string
        monitorAccount uint32
        serverAccount  uint32
        currentCount   int
        limit          int
        shouldPass     bool
    }{
        {"same_account", 1, 1, 0, 2, false},
        {"different_account_under_limit", 1, 2, 1, 2, true},
        {"different_account_at_limit", 1, 2, 2, 2, false},
    }
}
```

#### State Machine Testing
**Priority**: High - Validates monitor lifecycle transitions

**Test Categories**:
- **Valid Transitions**: All legal state changes
- **Invalid Transitions**: Blocked transitions with proper error handling
- **Edge Cases**: Bootstrap scenarios, emergency conditions
- **Consistency**: Global vs server state synchronization

```go
func TestStateMachineTransitions(t *testing.T) {
    tests := []struct {
        name         string
        globalStatus MonitorStatus
        serverStatus ServerScoreStatus
        constraints  []ConstraintViolation
        expected     CandidateState
    }{
        {"pending_active_monitor", StatusPending, StatusActive, nil, CandidateOut},
        {"paused_monitor", StatusPaused, StatusTesting, nil, CandidateBlock},
        {"healthy_testing", StatusActive, StatusTesting, nil, CandidateIn},
    }
}
```

#### Emergency Override Testing
**Priority**: Critical - Safety mechanism testing

**Test Scenarios**:
- Zero active monitors with constraint violations
- Emergency promotion despite constraints
- Emergency conditions vs capacity limits
- Bootstrap scenarios with multiple constraints

```go
func TestEmergencyOverride(t *testing.T) {
    // Setup: Server with 0 active monitors, all candidates have constraint violations
    server := createTestServer()
    candidates := createCandidatesWithConstraintViolations()

    // Expected: Emergency override allows promotion despite constraints
    result := selector.ProcessServer(server.ID)

    assert.Greater(t, result.ActiveMonitors, 0)
    assert.Contains(t, result.Changes, "emergency override")
}
```

### Test Data Management

#### Mathematical Test Validation
Ensure test conditions make expected outcomes possible:

```go
// Example: Account limit testing
// MaxPerServer=2, ActiveCount=1, TestingCount=2
// Total limit = MaxPerServer + 1 = 3
// Current total = 1 + 2 = 3 (at limit)
// Promoting testing→active: would become 2 active + 1 testing = 3 (still valid)
accountLimits := map[uint32]*accountLimit{
    1: {AccountID: 1, MaxPerServer: 2, ActiveCount: 1, TestingCount: 2},
}
```

#### Test Helper Functions
Centralized test data creation for consistency:

```go
// Monitor creation helpers
func createHealthyMonitors(count int, status ServerScoreStatus) []evaluatedMonitor
func createMonitorsWithConstraintViolations(violationType ConstraintType) []evaluatedMonitor
func createAccountLimitsScenario(limits map[uint32]int) map[uint32]*accountLimit

// Validation helpers
func validateWorkingCounts(result RuleResult, expected workingCounts) error
func validateConstraintViolations(changes []statusChange, expected []violationType) error
func validateStateTransitions(changes []statusChange, expected []transition) error
```

### Integration Testing Framework

#### Database Integration Testing
**Scope**: Real database operations with proper isolation

**Infrastructure**:
- Use CI tools (`./scripts/test-db.sh start`) for database setup
- Port allocation (3308: all test databases - unified for consistency)
- Clean database state for each test run
- Transaction rollbacks for test isolation

**Test Scenarios**:
```go
func TestDatabaseIntegration(t *testing.T) {
    // Test actual database operations
    db := setupTestDatabase(t)
    defer cleanupTestDatabase(t, db)

    // Test constraint tracking persistence
    testConstraintViolationPersistence(t, db)

    // Test status change application
    testStatusChangeApplication(t, db)

    // Test concurrent operations
    testConcurrentSelectorRuns(t, db)
}
```

#### Multi-Server Testing
**Scope**: Behavior across multiple servers with shared monitor pool

**Test Cases**:
- Monitor reassignment between servers
- Constraint interactions across servers
- Account limit enforcement globally
- Network diversity across server assignments

### Performance Testing Strategy

#### Benchmark Testing
**Scope**: Algorithm performance under various load conditions

```go
func BenchmarkSelectorProcessing(b *testing.B) {
    scenarios := []struct {
        name         string
        monitorCount int
        serverCount  int
    }{
        {"small_scale", 50, 5},
        {"medium_scale", 500, 20},
        {"large_scale", 2000, 100},
    }

    for _, scenario := range scenarios {
        b.Run(scenario.name, func(b *testing.B) {
            testData := generateTestScenario(scenario.monitorCount, scenario.serverCount)
            b.ResetTimer()
            for i := 0; i < b.N; i++ {
                processAllServers(testData)
            }
        })
    }
}
```

#### Performance Regression Testing
- Baseline measurements for algorithm execution time
- Memory allocation tracking for large datasets
- Constraint checking overhead analysis
- Database query performance validation

### Test Coverage Analysis

#### Coverage Targets by Component

**Selector Core (`process.go`)**: 90%+
- Contains main selection algorithm
- All rule implementations
- Working count management

**Constraints (`constraints.go`)**: 95%+
- Network constraint validation
- Account constraint checking
- Constraint type definitions

**State Management (`state.go`)**: 95%+
- State determination logic
- Transition validation
- Emergency condition handling

**Grandfathering (`grandfathering.go`)**: 85%+
- Grandfathering detection
- Historical violation tracking

#### Coverage Measurement Commands
```bash
# Generate coverage profile
go test -coverprofile=coverage.out ./selector

# View coverage by function
go tool cover -func=coverage.out | grep -E "(applyRule|calculateSafety|determineState)"

# Generate HTML coverage report
go tool cover -html=coverage.out -o coverage.html

# Enforce coverage thresholds in CI
go tool cover -func=coverage.out | tail -1 | awk '{print $3}' | sed 's/%//' | awk '{if($1<80) exit 1}'
```

### Debugging and Troubleshooting Strategy

#### Test Failure Analysis Framework

**Step 1: Understand Data Dependencies**
- Trace through test setup to identify required data relationships
- Verify foreign key relationships are satisfied
- Check monitor types and server compatibility

**Step 2: Verify Query Requirements**
- Use `Grep` to find query definitions
- Read actual SQL to understand JOIN conditions and WHERE clauses
- Ensure test data satisfies all query conditions

**Step 3: Use CI Tools First**
- `./scripts/test-ci-local.sh` - Full CI environment emulation
- `./scripts/test-scorer-integration.sh` - Component-specific tests
- `./scripts/diagnose-ci.sh` - Comprehensive failure diagnostics

#### Common Bug Patterns and Tests

**Safety Variable Scope Creep**:
```go
func TestSafetyVariableScopeCreep(t *testing.T) {
    // Test that maxRemovals=0 only affects intended operations
    // Verify constraint cleanup can proceed when safety limits active
}
```

**Mathematical Consistency**:
```go
func TestWorkingCountConsistency(t *testing.T) {
    // Verify working counts are updated after each change
    // Test that promotion/demotion math remains consistent
}
```

**Self-Reference Bugs**:
```go
func TestSelfExclusionInConstraints(t *testing.T) {
    // Verify entities are excluded from conflict detection with themselves
}
```

### Test Automation and CI Integration

#### Continuous Integration Requirements

**Pre-commit Validation**:
```yaml
# CI Pipeline Integration
- name: Test Coverage Check
  run: |
    go test -coverprofile=coverage.out ./selector
    COVERAGE=$(go tool cover -func=coverage.out | tail -1 | awk '{print $3}' | sed 's/%//')
    if [ $COVERAGE -lt 80 ]; then
      echo "Coverage $COVERAGE% is below 80% threshold"
      exit 1
    fi

- name: Safety Logic Coverage
  run: |
    # Verify 100% coverage of emergency and safety functions
    go test -coverprofile=safety.out -run="Emergency|Safety" ./selector
```

**Integration Test Matrix**:
- Multiple Go versions (1.19, 1.20, 1.21)
- Multiple database versions (MySQL 8.0, 8.1)
- Concurrent test execution with race detection

#### Test Environment Management

**Database Port Allocation**:
- 3308: All test databases (unified port for consistency)

**Test Data Isolation**:
- Use separate databases per test suite
- Implement test-specific prefixes for data separation
- Clean shutdown procedures to prevent test interference

### Quality Metrics and Monitoring

#### Test Quality Indicators

**Coverage Metrics**:
- Line coverage percentage by package
- Function coverage for critical paths
- Branch coverage for conditional logic

**Test Effectiveness**:
- Bug detection rate in testing vs production
- Test execution time trends
- Flaky test identification and remediation

**Regression Prevention**:
- Tests per bug report ratio
- Post-incident test additions
- Test coverage delta on code changes

#### Operational Test Monitoring

**Test Execution Tracking**:
- CI test success/failure rates
- Test execution duration trends
- Resource usage during test runs

**Test Quality Trends**:
- Coverage improvement over time
- Test addition rate vs code growth
- Critical path test stability

### Test Maintenance Strategy

#### Regular Review Process

**Monthly Coverage Review**:
- Identify coverage gaps in new code
- Review test effectiveness for recent bug fixes
- Update test scenarios for changed requirements

**Quarterly Test Architecture Review**:
- Assess test execution performance
- Identify opportunities for test consolidation
- Review mock usage and integration boundaries

**Post-Incident Test Enhancement**:
- Add tests for every production issue discovered
- Validate that new tests would have caught the issue
- Document test scenarios in incident retrospectives

#### Test Documentation Requirements

**Test Scenario Documentation**:
- Complex test cases include mathematical explanations
- Edge case documentation with business justification
- Integration test workflow documentation

**Test Data Documentation**:
- Explain relationships between test entities
- Document constraint calculations and expected outcomes
- Provide examples of valid and invalid test scenarios

This comprehensive testing strategy ensures robust validation of the complex selector algorithms while maintaining development velocity and preventing regressions.
