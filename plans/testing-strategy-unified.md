# Unified Testing Strategy for NTP Monitor

## Executive Summary

This comprehensive testing strategy consolidates findings from multiple testing analyses to address critical infrastructure gaps and recent refactoring bugs. Current test coverage is inadequate at **6% overall** with **53.6% in the selector package**. Critical infrastructure components have **0% coverage**.

**Primary Goals:**
- Increase overall test coverage from 6% to 40-50%
- Achieve 80%+ coverage in critical packages (selector, client/config)
- Address specific bugs and architectural issues identified in recent refactoring
- Establish sustainable testing practices

## Current State Analysis

### Critical Issues Identified

#### Recent Refactoring Bugs
1. **Safety Variable Scope Creep**: `maxRemovals = 0` blocking ALL changes instead of just active demotions
2. **Emergency Condition Over-Restriction**: Complex compound conditions preventing legitimate constraint demotions
3. **Lost Specificity**: Safety checks abstracted but then reused for broader blocking than intended

#### Infrastructure Coverage Gaps
- **Client Configuration Management**: ~0% coverage with race conditions in fsnotify hot reloading
- **Database Operations**: 0% coverage for connection management, transactions, status updates
- **NTP Monitoring Core**: ~0% coverage for core business functionality
- **JWT Authentication**: 0% coverage for security-critical components

### Risk-Based Priority Matrix

## Implementation Plan

### Phase 1: Critical Safety Logic (Week 1-2)
**Priority**: CRITICAL - Production stability risks

#### 1.1 Selector Safety Logic Validation
**Current**: 53.6% → **Target**: 95%+

**Critical Functions to Test**:
- `calculateSafetyLimits`: 84.6% → 100% (edge cases, emergency conditions)
- `applyRule6BootstrapPromotion`: 13.6% → 90%+ (bootstrap scenarios)
- `applyRule1ImmediateBlocking`: 75% → 95%+ (blocking logic)

**Key Test Scenarios**:
```go
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
            name: "normal_operation_above_target",
            targetNumber: 7, totalMonitors: 10,
            healthyActive: 8, activeCount: 8,
            expectedMaxRemovals: 2, expectEmergency: false,
        },
    }
}

func TestEmergencyConditions_ConstraintProcessing(t *testing.T) {
    // Verify constraint demotions proceed even during emergency conditions
    // Verify safety variables don't block legitimate constraint cleanup
}
```

#### 1.2 Client Configuration Management
**Current**: ~0% → **Target**: 85%+

**Critical Race Conditions to Test**:
- fsnotify event handling and debounce logic
- Atomic file operations and concurrent access
- Certificate loading/renewal workflows
- Memory leaks in notification system

**Test Files to Create**:
- `client/config/appconfig_test.go`
- `client/config/appconfig_manager_test.go`
- `client/config/config_persist_test.go`
- `client/config/appconfig_certs_test.go`

### Phase 2: Core Business Logic (Week 3-4)
**Priority**: HIGH - Core functionality validation

#### 2.1 Database Operations
**Current**: 0% → **Target**: 80%+

**Critical Functions**:
```go
func TestLoadServerInfo(t *testing.T) {
    // Test valid/invalid server IDs, connection failures, malformed data
}

func TestApplyStatusChange(t *testing.T) {
    // Test database status updates, transaction rollbacks, concurrent updates
}

func TestBuildAccountLimitsFromMonitors(t *testing.T) {
    // Test account limit calculation, missing data, performance with large sets
}
```

#### 2.2 NTP Monitoring Core Logic
**Current**: ~0% → **Target**: 85%+

**Files Needing Tests**:
- `client/monitor/monitor.go` - NTP monitoring with beevik/ntp
- `client/monitor/status.go` - Status transitions
- `client/monitor/capturebuffer.go` - Data capture and buffering

### Phase 3: Integration and State Management (Week 5-6)
**Priority**: MEDIUM-HIGH - System-wide validation

#### 3.1 Working Count State Management
**Goal**: Verify state consistency across rule execution

```go
func TestWorkingCountConsistency(t *testing.T) {
    // Track working counts after each rule application
    // Verify counts match expected outcomes
    // Test rule execution order dependencies
}
```

#### 3.2 JWT Authentication and API
**Current**: 0% → **Target**: 85%+

- Token generation and validation
- Permission mapping for environments
- MQTT topic permission generation
- API endpoints and certificate-based authentication

### Phase 4: Performance and Complex Scenarios (Week 7-8)
**Priority**: MEDIUM - System validation and optimization

#### 4.1 Multi-Constraint Scenarios
```go
func TestComplexProductionScenarios(t *testing.T) {
    scenarios := []struct {
        name     string
        setup    func() testScenario
        validate func([]statusChange) error
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
    }
}
```

#### 4.2 Performance Benchmarks
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
    // Performance validation for refactored architecture
}
```

## Testing Framework and Utilities

### Configuration Testing Framework
```go
type testEnv struct {
    ctx    context.Context
    cfg    *AppConfig
    tmpDir string
}

func setupTestConfig(t *testing.T) (*testEnv, func()) {
    tmpDir, err := os.MkdirTemp("", "config-test-*")
    require.NoError(t, err)

    ctx := context.Background()
    log := logger.Setup()
    ctx = logger.NewContext(ctx, log)

    cfg, err := NewAppConfig(ctx, depenv.DeployDevel, tmpDir, false)
    require.NoError(t, err)

    return &testEnv{
        ctx:    ctx,
        cfg:    cfg,
        tmpDir: tmpDir,
    }, func() {
        os.RemoveAll(tmpDir)
    }
}
```

### Selector Testing Framework
```go
// Test data builders for selector scenarios
func createTestScenario(name string) *selectorTestScenario {
    return &selectorTestScenario{
        name:               name,
        monitors:           make(map[ntpdb.ServerScoresStatus][]evaluatedMonitor),
        accountLimits:      make(map[uint32]*accountLimit),
        expectedChanges:    make([]statusChange, 0),
        expectedViolations: make([]constraintViolation, 0),
    }
}

// Validation helpers for complex scenarios
func validateWorkingCounts(result ruleResult, expected workingCounts) error {
    if result.workingActive != expected.active {
        return fmt.Errorf("expected %d active, got %d", expected.active, result.workingActive)
    }
    return nil
}
```

### Database Testing Framework
```go
type MockDatabase struct {
    servers     map[uint32]*serverInfo
    changes     []statusChange
    failNext    bool
    transaction *sql.Tx
}

func (m *MockDatabase) LoadServerInfo(serverID uint32) (*serverInfo, error) {
    if m.failNext {
        m.failNext = false
        return nil, errors.New("mock database error")
    }
    return m.servers[serverID], nil
}
```

## Coverage Targets and Quality Gates

### Coverage Targets by Package

| Package | Current | Target | Priority |
|---------|---------|--------|----------|
| client/config | ~0% | 85% | Critical |
| selector | 53.6% | 95% | Critical |
| ntpdb | 0% | 80% | High |
| client/monitor | ~0% | 85% | High |
| server/jwt | 0% | 85% | Medium-High |
| server | ~0% | 80% | Medium-High |
| **Overall** | **6%** | **40-50%** | **Project Goal** |

### Quality Gates
- [ ] All existing tests continue to pass throughout development
- [ ] No performance regression > 10% for typical workloads
- [ ] All identified bugs from recent refactoring covered by tests
- [ ] Integration tests cover multi-component interactions
- [ ] CI/CD pipeline enforces coverage thresholds

## Testing Standards and Requirements

### Code Quality Requirements
1. **Race Condition Testing**: All tests must pass with `go test -race`
2. **Coverage Thresholds**:
   - Critical packages: 85%+ coverage
   - Standard packages: 70%+ coverage
   - Integration tests: 60%+ coverage
3. **Performance**: No test should run longer than 5 seconds
4. **Resource Management**: All tests must clean up resources (temp files, goroutines)

### Test Development Guidelines
1. **Use table-driven tests** for multiple scenarios
2. **Avoid `testify/assert`** - use standard library testing patterns
3. **Create focused test files** - don't add everything to existing files
4. **Use dependency injection** to improve testability
5. **Mock external dependencies** but prefer integration tests for critical paths

## CI/CD Integration and Maintenance

### Coverage Enforcement
```yaml
# CI Pipeline Integration
- name: Test Coverage Check
  run: |
    go test -coverprofile=coverage.out ./...
    COVERAGE=$(go tool cover -func=coverage.out | tail -1 | awk '{print $3}' | sed 's/%//')
    if [ $COVERAGE -lt 40 ]; then
      echo "Coverage $COVERAGE% is below 40% threshold"
      exit 1
    fi

- name: Safety Logic Coverage
  run: |
    # Verify 100% coverage of emergency and safety functions
    go test -coverprofile=safety.out -run="Emergency|Safety" ./selector
```

### Pre-commit Testing Requirements
```bash
# Required before any commit
gofumpt -w $(find . -name "*.go")
go test ./...
go test -race ./...
go test -cover ./... | grep -E "(total:|FAIL)"
```

### Ongoing Improvement
1. **Monthly Reviews**: Track coverage trends and identify new gaps
2. **Post-Incident Testing**: Add tests for any production issues
3. **Refactoring Support**: Maintain high coverage during architectural changes
4. **Documentation Updates**: Keep testing guidelines current

## Success Metrics and Timeline

### Week 1-2: Critical Safety Logic
- **Deliverable**: Configuration package 85%+ coverage, selector safety logic 95%+ coverage
- **Milestone**: All fsnotify race conditions and safety logic edge cases tested
- **Risk**: If not completed, production deployment confidence remains low

### Week 3-4: Core Business Logic
- **Deliverable**: Database package 80%+ coverage, monitor package 85%+ coverage
- **Milestone**: Core business functionality validated through comprehensive tests
- **Risk**: Database test infrastructure may require significant setup

### Week 5-6: Integration and Authentication
- **Deliverable**: JWT package 85%+ coverage, working count consistency validation
- **Milestone**: Security-critical components and state management proven sound
- **Risk**: May discover architectural issues requiring design changes

### Week 7-8: Performance and Complex Scenarios
- **Deliverable**: 40-50% overall coverage achieved, performance benchmarks established
- **Milestone**: Production-ready test coverage with performance validation
- **Risk**: Performance regression discovery may require optimization work

## Risk Mitigation

### Preventing Regressions
1. **Incremental Development**: Add tests in small batches, validate after each
2. **Backwards Compatibility**: Maintain existing test interfaces
3. **Feature Flags**: Use build tags for experimental tests
4. **Continuous Validation**: Run full test suite after each phase

### Managing Complexity
1. **Start Simple**: Unit tests before integration tests
2. **Clear Separation**: Separate unit, integration, and performance tests
3. **Documentation**: Document complex test scenarios and their purpose
4. **Review Process**: Code review all test additions

---

## Immediate Next Steps

1. **Week 1**: Begin with selector safety logic validation - highest risk from recent refactoring
2. **Week 1**: Parallel start on `client/config` package testing - critical infrastructure risk
3. **Week 2**: Complete safety logic and begin database operations testing
4. **Set up CI coverage enforcement** to prevent regression

This unified strategy provides a clear, prioritized path from the current 6% coverage to a production-ready 40-50% coverage while addressing both the most critical stability risks and the specific bugs introduced during recent refactoring.
