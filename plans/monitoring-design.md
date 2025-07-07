# Monitor Lifecycle and Capacity Management Design

## Overview

This document describes the monitor lifecycle management and capacity management systems that govern how monitors are promoted, demoted, and maintained across the NTP Pool monitoring infrastructure.

## Monitor Lifecycle

### Lifecycle States

#### Global Monitor Lifecycle (monitors.status)
1. **Registration** → Monitor created with `pending` status
2. **Activation** → Manual approval changes status to `testing` or `active`
3. **Server Assignment** → API creates `server_scores` entries as `candidate`
4. **Promotion Pipeline** → `candidate` → `testing` → `active` per server
5. **Maintenance** → Ongoing health and constraint evaluation
6. **Deactivation** → Status changes to `paused` or `deleted`

#### Server-Specific Lifecycle (server_scores.status)
1. **Assignment** → Monitor assigned to server as `candidate`
2. **Testing Phase** → Promoted to `testing` to collect performance data
3. **Active Phase** → Promoted to `active` based on performance
4. **Demotion** → Demoted due to constraints, health, or capacity limits
5. **Removal** → Removed from server assignment

### State Transitions

#### Promotion Flow
```
Available Pool → Candidate → Testing → Active
     ↑              ↓         ↓       ↓
     └──────────────┴─────────┴───────┘
           (Demotion paths)
```

#### Promotion Criteria
- **Available → Candidate**: Assignment policy (handled by API)
- **Candidate → Testing**: Health check, constraint validation, capacity available
- **Testing → Active**: Performance metrics meet thresholds, globally active status

#### Demotion Triggers
- **Constraint violations**: Network, account, diversity violations
- **Health issues**: Poor performance metrics, connectivity problems
- **Global status changes**: Monitor becomes pending, paused, or deleted
- **Capacity management**: Excess monitors beyond targets

### Promotion Helper Architecture

Centralized promotion logic ensures consistent behavior across all rules:

```go
type promotionResult struct {
    promoted bool
    reason   string
}

func attemptPromotion(monitor *Monitor, targetStatus Status,
    workingCounts *workingCounts, emergency bool) promotionResult {
    // Unified promotion logic with:
    // - Emergency override handling
    // - Constraint checking
    // - Capacity validation
    // - Working count updates
}
```

## Capacity Management

### Target Definitions

#### Active Monitor Targets
- **Default**: 7 active monitors per server
- **Minimum**: Never reduce below 1 (safety mechanism)
- **Emergency override**: Zero active triggers emergency mode

#### Testing Monitor Targets (Dynamic)
- **Base target**: 5 testing monitors per server
- **Dynamic adjustment**: Increases when active monitors below target
- **Formula**: `targetTesting = 5 + max(0, targetActive - actualActive)`

**Example**: Server with 3 active monitors (target 7)
- Gap: 7 - 3 = 4 missing active monitors
- Testing target: 5 + 4 = 9 testing monitors
- Allows larger pipeline when rebuilding active pool

#### Globally Active Requirements
- **Minimum**: 4 globally active monitors in testing pool per server
- **Purpose**: Ensures promotion pipeline for server-active status
- **Bootstrap**: Promotes globally active candidates when requirement not met

### Capacity Enforcement Rules

#### Rule 1.5: Active Excess Demotion
Demotes excess healthy active monitors when count exceeds target:

**Safety Checks**:
- Never reduce to 0 active monitors
- Respects emergency override conditions
- Reserves demotion budget for constraint violations

**Implementation**:
```go
if workingActiveCount > targetActiveMonitors &&
   workingActiveCount > 1 &&
   !emergencyOverride &&
   limits.activeRemovals > demotionsSoFar {

    // Calculate available budget after reserving for constraints
    constraintDemotionsNeeded := countConstraintViolations()
    reservedDemotions := min(constraintDemotionsNeeded, remainingBudget)
    availableDemotions := remainingBudget - reservedDemotions

    // Demote worst performers
    demotionsNeeded := min(excessActive, availableDemotions)
}
```

#### Rule 2.5: Testing Pool Management
Manages testing pool size based on dynamic targets:

**Dynamic Calculation**:
```go
baseTestingTarget := 5
activeGap := max(0, targetActiveMonitors - len(activeMonitors))
dynamicTestingTarget := baseTestingTarget + activeGap

if len(testingMonitors) > dynamicTestingTarget {
    // Demote excess testing monitors to candidate
}
```

#### Rule 5: Candidate Promotion with Performance-Based Replacement
Promotes candidates to testing using two complementary approaches:

**Phase 1: Capacity-Based Promotion** (existing logic):
```go
testingCapacity := max(0, dynamicTestingTarget - workingTestingCount)
promotionsNeeded := min(min(changesRemaining, 2), testingCapacity)
```

**Phase 2: Performance-Based Replacement** (new logic):
When testing pool is at capacity, compares candidate performance with existing testing monitors and replaces worse performers:

```go
// Only attempt replacement if we have budget remaining
remainingBudget := promotionLimit - capacityPromotions
if remainingBudget > 0 && len(candidates) > 0 && len(testingMonitors) > 0 {
    // Find better-performing candidates that can replace worse testing monitors
    // Respects all constraints and account limits
    replacementChanges := attemptTestingReplacements(...)
}
```

**Performance Comparison Logic**:
- Health status takes priority (healthy always beats unhealthy)
- Among monitors with equal health, RTT determines performance (lower is better)
- Only replaces when candidate significantly outperforms testing monitor

**Constraint Compliance**:
- Tests replacement scenarios with temporary account limits
- Validates both demotion and promotion against all constraints
- Maintains testing pool size (1 out, 1 in)

### Change Limits System

#### Per-Status-Group Limits
Independent limits for each transition type prevent competition:

```go
type changeLimits struct {
    activeRemovals  int // active → testing demotions
    testingRemovals int // testing → candidate demotions
    promotions      int // testing → active, candidate → testing
}
```

#### Limit Calculation
```go
func calculateChangeLimits(currentActiveMonitors, blockedMonitors int) changeLimits {
    base := 2 // Increased from 1 for better throughput
    if blockedMonitors > 1 {
        base = 3 // Expedite cleanup when many blocked
    }
    if currentActiveMonitors == 0 {
        base = 4 // Bootstrap mode
    }

    return changeLimits{
        activeRemovals:  base,
        testingRemovals: base,
        promotions:      base,
    }
}
```

#### Benefits
- Independent processing per status group
- Higher throughput (base 2 vs previous 1)
- Dynamic testing pool sizing works correctly
- Maintained safety with per-group limits

### Working Count Tracking

#### Mathematical Consistency
The system maintains accurate counts throughout the selection process:

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

// Update after each status change decision
func (wc *workingCounts) applyChange(change statusChange) {
    // Decrement from source status
    switch change.fromStatus {
    case ServerScoresStatusActive:
        wc.active--
    case ServerScoresStatusTesting:
        wc.testing--
    }

    // Increment to target status
    switch change.toStatus {
    case ServerScoresStatusActive:
        wc.active++
    case ServerScoresStatusTesting:
        wc.testing++
    }
}
```

#### Critical Pattern
Update working counts immediately after each change decision, not after database execution. This prevents counting errors that led to monitor target overruns.

## Rule Execution Order

### Optimized Sequence
The rules execute in a specific order to maximize efficiency:

1. **Rule 1** (Immediate Blocking): Remove monitors that should be blocked immediately
2. **Rule 2** (Gradual Constraint Removal): Gradual removal of candidateOut monitors
3. **Rule 1.5** (Active Excess Demotion): Demote excess healthy active monitors
4. **Rule 3** (Testing to Active Promotion): Promote from testing to active
5. **Rule 5** (Candidate to Testing Promotion): Promote candidates to testing and replace worse-performing testing monitors
6. **Rule 2.5** (Testing Pool Management): Demote excess testing monitors
7. **Rule 6** (Bootstrap Promotion): Bootstrap case promotions

### Order Rationale
- **Rule 2.5 after Rule 5**: Ensures Rule 2.5 sees all pending promotions from Rule 5
- **Rule 1.5 before Rule 3**: Ensures promotion counts are accurate after active demotions
- **Constraint rules first**: Clears violations before capacity optimizations

## Emergency Override System

### Emergency Conditions
Emergency override activates when `len(activeMonitors) == 0`, indicating system-critical state.

### Emergency Behaviors
- **Constraint bypassing**: Allows promotions despite constraint violations
- **Capacity respect**: Still respects capacity limits even in emergency
- **Unified handling**: All promotion functions accept `emergencyOverride` parameter

### Implementation Pattern
```go
emergencyOverride := len(activeMonitors) == 0

// Level 1: Emergency safety (allows constraint violations)
if !emergencyOverride && hasConstraintViolation() {
    return false  // Block only if not emergency
}

// Level 2: Capacity limits (respected even in emergencies)
if workingCount >= targetCount {
    return false  // Always respect capacity
}
```

## Bootstrap Logic

### Bootstrap Conditions
- Zero active monitors on server
- All monitors in candidate status
- Need to establish initial monitoring

### Bootstrap Behavior
- Promotes multiple candidates to testing simultaneously
- Respects constraint checking during promotion
- Uses emergency override for faster recovery
- Targets minimum viable monitoring coverage

### Implementation
```go
if len(activeMonitors) == 0 && len(testingMonitors) == 0 {
    // Bootstrap mode: promote up to target testing count
    promoteCount := min(targetTestingCount, len(healthyCandidates))
    // Apply emergency override for constraint bypassing
    emergencyOverride := true
}
```

## Health and Performance Integration

### Performance Metrics
- **RTT (Round Trip Time)**: Network latency to server
- **Step**: Time accuracy measurement
- **Health Score**: Composite health calculation
- **Availability**: Successful monitoring percentage

### Health-Based Decisions
- **Poor performance**: Triggers candidateOut (gradual demotion)
- **Good performance**: Enables testing → active promotion
- **No performance data**: Prevents promotion until data available

### Performance Thresholds
Configurable thresholds determine promotion/demotion decisions:
- Testing monitors need consistent performance for active promotion
- Active monitors with degraded performance face demotion
- Candidates need basic health check for testing promotion

## Safety Mechanisms

### Never-Zero Active Rule
The system never reduces active monitors to zero unless in controlled scenarios:
- Emergency override allows bypass only for recovery
- Rule 1.5 checks `workingActiveCount > 1` before demoting
- Bootstrap logic ensures rapid recovery from zero state

### Constraint Budget Reservation
When multiple changes compete for limited budget:
1. **Count constraint violations** requiring immediate attention
2. **Reserve budget** for constraint-based demotions
3. **Use remaining budget** for capacity optimizations

### Cascading Demotion Prevention
- Capacity checks prevent over-promotion requiring immediate cleanup
- Dynamic testing targets adjust based on active monitor availability
- Working counts prevent mathematical inconsistencies

## Monitoring and Metrics

### Capacity Metrics
- `selector_monitor_pool_size{status, server_id}`: Current pool sizes
- `selector_globally_active_monitors{server_id}`: Globally active in testing
- Active/testing count ratios and gap measurements

### Performance Metrics
- `selector_process_duration_seconds{server_id}`: Selection algorithm timing
- `selector_changes_applied_total{server_id}`: Successful changes per server
- Working count accuracy validation

### Health Metrics
- Promotion/demotion rates by reason
- Constraint violation frequencies
- Emergency override activation tracking

## Integration Points

### API Integration
- Monitor activation triggers `InsertMonitorServerScores`
- Creates candidate entries for all compatible servers
- Selector promotes candidates through testing to active

### Database Schema
- `server_scores.status` tracks per-server monitor state
- `monitors.status` provides global monitor state
- `accounts.flags` configures per-account limits

### Configuration Management
- Dynamic testing target calculation
- Configurable active monitor targets
- Per-account limit customization via flags JSON

This capacity management design ensures efficient monitor utilization while maintaining system stability and operational safety.
