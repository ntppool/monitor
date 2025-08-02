# Architectural Improvements TODO

## Overview

This document consolidates major architectural improvements and simplifications planned for the NTP Pool monitoring system. These changes focus on reducing complexity, improving maintainability, and enhancing system reliability.

## Eliminate "New" Status Architecture Simplification ✅ **COMPLETED**
**Status**: Schema updated (commit: 64416d0)

### Current Problem
The selector system manages both "which monitors should be considered" (assignment policy) and "how to select among assigned monitors" (selection algorithm) in a single component, leading to:

- **Persistent constraint violation warnings** for monitors with `serverStatus=new`
- **Complex dual-state system** with global monitor status and server-monitor relationship
- **Constraint checking for unassigned monitors** that shouldn't have constraints
- **Convoluted selection logic** with "available pool" evaluation
- **False constraint violations** blocking valid monitor assignments

### Root Cause Analysis
The conceptual "new" status creates a hybrid state that doesn't actually exist in the database, requiring complex logic to simulate non-existent relationships. The selector tries to manage both "which monitors should be considered" (assignment policy) and "how to select among assigned monitors" (selection algorithm) in a single component.

### **IMPLEMENTATION READY**: Detailed elimination plan available in `eliminate-new-status.md` with:
- **4-week implementation timeline** (analysis → implementation → testing → deployment)
- **Production assignment code integration** using existing `InsertMonitorServerScores`
- **Comprehensive risk assessment** with mitigation strategies
- **~200+ lines of code reduction** by removing "available pool" logic
- **Zero constraint violation warnings** for unassigned monitors

### Proposed Simplified Architecture

#### Current State Model
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

#### Proposed State Model
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

### Implementation Strategy

#### Phase 1: Remove Available Monitor Logic
**Files to Modify**:
- `selector/selector.go` - Remove `findAvailableMonitors` call and evaluation
- `selector/process.go` - Remove "Rule 4: Add new monitors as candidates"
- `ntpdb/query.sql` - Remove `GetAvailableMonitors` query

**Benefits**:
- **Code Reduction**: ~200+ lines of complex "available pool" logic removed
- **Query Elimination**: One fewer database query per selector run
- **Complexity Reduction**: Simpler mental model and debugging

#### Phase 2: Update Constraint Checking Strategy
**New Approach**: Check constraints on EVERY run, not just for promotions
- Mark violations as `candidateOut` for gradual demotion (active→testing→candidate)
- Support cascading demotions when Monitor A demotion triggers Monitor B demotion
- Keep existing constraint types (network, account limits, network diversity)

**Implementation**:
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

#### Phase 3: External Assignment Integration
**Leverage Existing Production Code**: The API system already handles initial assignment via `InsertMonitorServerScores`:

```sql
INSERT IGNORE INTO server_scores
  (monitor_id, status, server_id, score_raw, created_on)
  SELECT m.id, 'candidate', s.id, s.score_raw, NOW()
  FROM servers s, monitors m
  WHERE s.ip_version = m.ip_version
    AND m.id = ?
    AND m.status IN ('active', 'testing')
    AND s.deletion_on IS NULL;
```

**Selector Responsibilities** (Simplified):
1. **Health-based promotions** - candidate→testing based on health
2. **Performance-based promotions** - testing→active based on metrics
3. **Constraint-based demotions** - active→testing→candidate when constraints violated
4. **Global status changes** - handle pending/paused/deleted monitors
5. **Bootstrap handling** - promote candidates when no testing monitors exist

### Expected Benefits

#### Bug Resolution
- **Zero constraint violation warnings** for "new" status monitors
- **Successful monitor promotions** from candidate pool
- **Proper constraint enforcement** for promotions only

#### Code Quality
- **Reduced lines of code** in selector package
- **Simplified test cases** without "new" status complexity
- **Clearer separation** between assignment and selection

#### Performance
- **Faster selector runs** due to less constraint checking
- **Fewer database queries** (no GetAvailableMonitors)
- **Better monitoring dashboard accuracy**

### Risk Assessment
- **Low Risk**: Removes complexity rather than adding it
- **Proven Components**: Existing assignment code already in production
- **Comprehensive Testing**: Plan covers all bootstrap and edge cases

## Conservative Promotion Logic Enhancement (Medium Priority)

### Current Issue
The selector can make aggressive promotion decisions that may need to be reversed quickly, leading to unnecessary churn.

### Proposed Improvements

#### Staged Promotion Requirements
```go
type promotionRequirement struct {
    MinimumTestingDuration time.Duration
    RequiredHealthChecks   int
    PerformanceThreshold   float64
    ConsistencyWindow      time.Duration
}

// Conservative promotion requires sustained good performance
func (sl *Selector) canPromoteConservatively(monitor *Monitor, req promotionRequirement) bool {
    // Require minimum time in current state
    if time.Since(monitor.LastStatusChange) < req.MinimumTestingDuration {
        return false
    }

    // Require sustained good health
    recentChecks := sl.getRecentHealthChecks(monitor.ID, req.ConsistencyWindow)
    if len(recentChecks) < req.RequiredHealthChecks {
        return false
    }

    // Require performance above threshold
    avgPerformance := sl.calculateAveragePerformance(recentChecks)
    if avgPerformance < req.PerformanceThreshold {
        return false
    }

    return true
}
```

#### Configurable Promotion Strategy
```go
type PromotionStrategy int

const (
    PromotionAggressive PromotionStrategy = iota // Current behavior
    PromotionBalanced                            // Moderate requirements
    PromotionConservative                        // High requirements
)

func (sl *Selector) getPromotionRequirements(strategy PromotionStrategy) promotionRequirement {
    switch strategy {
    case PromotionConservative:
        return promotionRequirement{
            MinimumTestingDuration: 4 * time.Hour,
            RequiredHealthChecks:   20,
            PerformanceThreshold:   0.95,
            ConsistencyWindow:      2 * time.Hour,
        }
    case PromotionBalanced:
        return promotionRequirement{
            MinimumTestingDuration: 1 * time.Hour,
            RequiredHealthChecks:   10,
            PerformanceThreshold:   0.90,
            ConsistencyWindow:      30 * time.Minute,
        }
    default: // Aggressive
        return promotionRequirement{
            MinimumTestingDuration: 15 * time.Minute,
            RequiredHealthChecks:   3,
            PerformanceThreshold:   0.80,
            ConsistencyWindow:      15 * time.Minute,
        }
    }
}
```

### Benefits
- **Reduced Churn**: Fewer promotion reversals
- **Improved Stability**: More reliable active monitor pool
- **Configurable Behavior**: Adapt to different operational needs

## Enhanced Account Constraint Management (Medium Priority)

### Current Limitations
Account constraints are hardcoded and inflexible, limiting operational adaptability.

### Proposed Enhancements

#### Dynamic Account Limits
```go
type AccountLimitConfig struct {
    MaxPerServer        int                          `json:"max_per_server"`
    MaxGlobal          int                          `json:"max_global"`
    PriorityMultiplier float64                      `json:"priority_multiplier"`
    OverrideRules      []AccountLimitOverride       `json:"override_rules"`
    ValidFrom          time.Time                    `json:"valid_from"`
    ValidUntil         *time.Time                   `json:"valid_until,omitempty"`
}

type AccountLimitOverride struct {
    ServerPattern string `json:"server_pattern"` // e.g., "*.pool.ntp.org"
    Limit         int    `json:"limit"`
    Reason        string `json:"reason"`
}
```

#### Time-Based Limit Adjustments
```go
func (sl *Selector) getEffectiveAccountLimit(accountID uint32, serverID uint32, timestamp time.Time) int {
    config := sl.getAccountLimitConfig(accountID)

    // Apply base limit
    limit := config.MaxPerServer

    // Apply server-specific overrides
    server := sl.getServerInfo(serverID)
    for _, override := range config.OverrideRules {
        if matchesPattern(server.Hostname, override.ServerPattern) {
            limit = override.Limit
            break
        }
    }

    // Apply time-based adjustments
    if config.ValidUntil != nil && timestamp.After(*config.ValidUntil) {
        // Revert to default when config expires
        limit = sl.defaultAccountLimit
    }

    return limit
}
```

### Implementation Strategy
1. **Extend accounts.flags JSON** with new limit configuration schema
2. **Update constraint checking** to use dynamic limit calculation
3. **Add API endpoints** for account limit management
4. **Implement limit history tracking** for audit and rollback

## Network Constraint Modernization (Low Priority)

### Current Implementation
Network constraints are hardcoded IPv4 /24 and IPv6 /48 subnets.

### Proposed Improvements

#### Configurable Network Constraints
```go
type NetworkConstraintConfig struct {
    IPv4SubnetMask int      `yaml:"ipv4_subnet_mask"` // Default: 24
    IPv6SubnetMask int      `yaml:"ipv6_subnet_mask"` // Default: 48
    ExemptNetworks []string `yaml:"exempt_networks"`  // Networks exempt from constraints
    StrictMode     bool     `yaml:"strict_mode"`      // Enforce constraints strictly
}

func (sl *Selector) loadNetworkConstraintConfig() (*NetworkConstraintConfig, error) {
    config := &NetworkConstraintConfig{
        IPv4SubnetMask: 24, // Default values
        IPv6SubnetMask: 48,
        StrictMode:     true,
    }

    // Load from configuration file or environment
    if configFile := os.Getenv("NETWORK_CONSTRAINT_CONFIG"); configFile != "" {
        if err := yaml.UnmarshalFile(configFile, config); err != nil {
            return nil, fmt.Errorf("loading network constraint config: %w", err)
        }
    }

    return config, nil
}
```

#### Geographic Diversity Constraints
```go
type GeographicConstraint struct {
    MaxPerCountry    int `yaml:"max_per_country"`
    MaxPerRegion     int `yaml:"max_per_region"`
    RequiredDistance int `yaml:"required_distance_km"`
}

func (sl *Selector) checkGeographicConstraint(
    monitor *Monitor,
    server *Server,
    existingMonitors []Monitor,
) error {
    monitorLocation := sl.geoIP.Lookup(monitor.IP)
    serverLocation := sl.geoIP.Lookup(server.IP)

    // Check distance requirement
    distance := calculateDistance(monitorLocation, serverLocation)
    if distance < sl.config.RequiredDistance {
        return fmt.Errorf("monitor too close to server: %dkm < %dkm",
            distance, sl.config.RequiredDistance)
    }

    // Check country/region limits
    sameCountryCount := sl.countSameCountry(existingMonitors, monitorLocation.Country)
    if sameCountryCount >= sl.config.MaxPerCountry {
        return fmt.Errorf("too many monitors from country %s: %d >= %d",
            monitorLocation.Country, sameCountryCount, sl.config.MaxPerCountry)
    }

    return nil
}
```

## Implementation Roadmap

### Phase 1: Foundation (Next Quarter)
1. ✅ **Eliminate "New" Status** - Schema updated (commit: 64416d0)
2. ✅ **Emergency Override Consistency** - Fixed (commit: b6515b8)
3. **Grandfathering Logic** - Implement functional grandfathering behavior

### Phase 2: Enhancement (Following Quarter)
1. **Conservative Promotion Logic** - Reduce unnecessary churn
2. **Dynamic Account Limits** - Configurable constraint management
3. **Enhanced Testing Framework** - Support new architectural patterns

### Phase 3: Advanced Features (Future)
1. **Network Constraint Modernization** - Geographic diversity, configurable subnets
2. **Real-time Configuration Updates** - Hot-reload constraint configurations
3. **Machine Learning Integration** - Predictive monitor performance scoring

## Success Metrics

### Complexity Reduction
- **Lines of Code**: 25% reduction in selector package complexity
- **Cyclomatic Complexity**: Reduce average function complexity by 30%
- **Test Maintenance**: 50% reduction in test setup complexity

### Operational Improvements
- **Configuration Flexibility**: Support for 5+ constraint configuration scenarios
- **Deployment Safety**: Zero-downtime constraint configuration changes
- **Recovery Time**: < 5 seconds from system issues to full recovery

### Development Velocity
- **New Feature Development**: 40% faster implementation of new constraints
- **Bug Fix Time**: 60% reduction in time to identify and fix constraint bugs
- **Code Review Efficiency**: 50% reduction in review time for constraint changes

## Migration Strategy

### Backward Compatibility
- **Configuration Migration**: Automatic conversion of existing constraint configurations
- **API Compatibility**: Maintain existing API contracts during transition
- **Gradual Rollout**: Feature flags for progressive enablement

### Risk Mitigation
- **Comprehensive Testing**: Integration tests for all architectural changes
- **Production Validation**: A/B testing of new vs old behavior
- **Rollback Procedures**: Quick revert capability for each architectural change
- **Monitoring Integration**: Real-time validation of architectural improvements

This architectural improvement roadmap provides a clear path toward a simpler, more maintainable, and more flexible monitoring system while preserving operational stability and reliability.
