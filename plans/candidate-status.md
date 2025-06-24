# Candidate Status Implementation Plan

## Executive Summary

This document outlines a comprehensive plan to enhance the monitor selector system with a sophisticated constraint validation framework. The enhancement introduces a four-stage workflow (available → candidate → testing → active) with grandfathering support for existing assignments that violate new constraints. Additionally, the plan includes restructuring the monolithic `selector.go` file into focused, maintainable components.

### Current Status (as of latest commit 0325b15)
- **Phase 1**: File Restructuring ✅ COMPLETED
- **Phase 2**: Database Schema Updates ✅ COMPLETED
- **Phase 3**: Constraint System Core ✅ COMPLETED
- **Phase 4**: Grandfathering System ✅ COMPLETED
- **Phase 5**: Enhanced State Machine ✅ COMPLETED
- **Phase 6**: Selection Algorithm Rewrite - TODO
- **Phase 7-10**: Testing, Monitoring, Deployment - TODO

### Key Goals
1. Implement multi-stage monitor selection with "candidate" status ✅
2. Enforce network and account-based constraints with grandfathering ✅
3. Ensure gradual transitions when constraints change ✅
4. Restructure code for better maintainability ✅
5. Maintain backward compatibility and system stability

## File Restructuring Strategy

### Current State
- `scorer/cmd/selector.go` - 465 lines containing all selector logic

### Target Structure
```
scorer/cmd/
├── selector.go                    # Main selector struct, Run method (~100 lines)
├── selector_constraints.go        # Constraint validation logic (~200 lines)
├── selector_state.go             # candidateState enum and transitions (~150 lines)
├── selector_process.go           # processServer logic (~250 lines)
├── selector_grandfathering.go    # Grandfathering detection (~150 lines)
├── selector_types.go             # Type definitions (~100 lines)
└── selector_test.go              # Tests (new)
```

### File Responsibilities

**selector.go** - Core orchestration
- `selector` struct definition
- `Run()` method
- `newSelector()` constructor
- Main loop and transaction management

**selector_constraints.go** - Validation logic
- Network constraint checking (subnet validation)
- Account constraint checking (same account, limits)
- Constraint configuration management
- `net/netip` based IP operations

**selector_state.go** - State management
- `candidateState` enum definition
- State transition logic
- `newStatusList` type and methods
- `IsOutOfOrder()` logic

**selector_process.go** - Server processing
- `processServer()` main logic
- Monitor categorization
- Selection algorithm implementation
- Database operations coordination

**selector_grandfathering.go** - Legacy support
- Grandfathering detection
- Constraint change tracking
- Gradual removal prioritization
- Historical violation tracking

**selector_types.go** - Data structures
- `newStatus` struct
- `monitorCandidate` struct
- `constraintViolation` enum
- `serverInfo` struct
- Configuration types

## Terminology and Problem Analysis

### Why "Pending" is Problematic

The term "pending" is already used in the global monitor status (`monitors.status = 'pending'`) to indicate a monitor that is not yet approved for any monitoring work. Using the same term for the server-monitor relationship would create confusion. Instead, we use:

- **"Available"**: Monitors that are globally active/testing but not assigned to this specific server
- **"Candidate"**: Monitors selected for potential assignment to this server
- **"Testing"**: Monitors actively monitoring and being evaluated for this server
- **"Active"**: Monitors confirmed for long-term monitoring of this server

### Global Status is Primary

The system ALWAYS respects global monitor status (`monitors.status`) before any server-specific logic:
- Only monitors with global status `active` or `testing` can be assigned to servers
- Monitors with global status `pending` are gradually phased out to allow clean transitions
- Monitors with global status `paused` or `deleted` are immediately blocked from selection
- The existing `GetMonitorPriority` query already provides `monitor_status` which we use as the primary filter
- State determination checks global status FIRST (Step 1) before any constraint validation

### The Performance Data Challenge

A critical issue with the original "pending" concept is that monitors not actively monitoring a server won't have performance data (`log_scores` entries) for evaluation. The `GetMonitorPriority` query requires:
- Recent monitoring data: `ls.ts > date_sub(now(), interval 12 hour)`
- Performance metrics: `avg(ls.rtt)`, `avg(ls.step)`, health calculations

Our solution uses an "available pool" approach where:
1. Monitors are selected into the "candidate" state based on non-performance criteria (constraints, load balancing)
2. Candidates are promoted to "testing" to begin collecting performance data
3. Testing monitors with good performance are promoted to "active"

## System Architecture

### Database Schema

#### Existing Tables (Referenced)
```sql
-- monitors table
CREATE TABLE `monitors` (
  `id` int unsigned NOT NULL AUTO_INCREMENT,
  `account_id` int unsigned DEFAULT NULL,
  `status` enum('pending','testing','active','paused','deleted') NOT NULL,
  `ip` varchar(40) DEFAULT NULL,
  `ip_version` enum('v4','v6') DEFAULT NULL,
  -- other fields...
);

-- server_scores table (to be modified)
CREATE TABLE `server_scores` (
  `id` bigint unsigned NOT NULL AUTO_INCREMENT,
  `monitor_id` int unsigned NOT NULL,
  `server_id` int unsigned NOT NULL,
  `status` enum('new','testing','active') NOT NULL DEFAULT 'new',
  -- other fields...
);

-- servers table
CREATE TABLE `servers` (
  `id` int unsigned NOT NULL AUTO_INCREMENT,
  `account_id` int unsigned DEFAULT NULL,
  `ip` varchar(40) NOT NULL,
  `ip_version` enum('v4','v6') NOT NULL,
  -- other fields...
);
```

#### Schema Modifications

**1. Extend server_scores.status enum**
```sql
ALTER TABLE server_scores
MODIFY COLUMN status enum('new','candidate','testing','active') NOT NULL DEFAULT 'new';
```

**2. Add constraint tracking**
```sql
ALTER TABLE server_scores
ADD COLUMN constraint_violation_type varchar(50) DEFAULT NULL,
ADD COLUMN constraint_violation_since datetime DEFAULT NULL,
ADD INDEX idx_constraint_violation (constraint_violation_type, constraint_violation_since);
```

**Note**: Account monitor limits are stored in `accounts.flags` JSON column with structure:
```json
{
  "monitor_limit": 5,           // Total monitors for account
  "monitor_per_server_limit": 2, // Base limit per server (default: 2)
  "monitor_enabled": true
}
```

The `monitor_per_server_limit` (X) is enforced per state:
- Active monitors: max X per account per server
- Testing monitors: max X+1 per account per server
- Active + Testing combined: max X+1 per account per server
- Candidate monitors: no limit

**Note**: Network constraints are hardcoded in the application:
- IPv4: /24 subnet
- IPv6: /48 subnet

### Constraint System Design

#### Hard vs Soft Constraints (candidateBlock vs candidateOut)

The system implements constraint enforcement through two removal mechanisms:

**Hard Constraints (candidateBlock)** - Immediate removal:
- New violations on unassigned monitors
- Globally paused or deleted monitors
- Non-grandfathered violations on new assignments

**Soft Constraints (candidateOut)** - Gradual removal:
- Globally pending monitors (allow clean transitions)
- Grandfathered violations (existing assignments that violate new constraints)
- Performance/health issues
- State inconsistencies
- Account limit violations on existing assignments

This approach ensures operational stability by preventing sudden capacity drops when constraints change.

#### Constraint Types
```go
// selector_types.go
type constraintViolationType string

const (
    violationNone        constraintViolationType = ""
    violationNetwork     constraintViolationType = "network"      // Same subnet
    violationAccount     constraintViolationType = "account"      // Same account
    violationLimit       constraintViolationType = "limit"        // Account limit exceeded
)

type constraintViolation struct {
    Type           constraintViolationType
    Since          time.Time
    IsGrandfathered bool
    Details        string
}
```

#### Constraint Configuration
```go
// Hardcoded in selector_constraints.go
const (
    defaultSubnetV4 = 24  // IPv4 subnet constraint
    defaultSubnetV6 = 48  // IPv6 subnet constraint
    defaultAccountLimitPerServer = 2  // Default max monitors per server per account
)
```

### State Machine

#### Monitor States (Global)
- `pending` - Not approved for any monitoring
- `testing` - Approved, being evaluated globally
- `active` - Fully approved for monitoring
- `paused` - Temporarily disabled
- `deleted` - Removed from system

#### Server-Monitor States (Per-server assignment)
- `new` - No relationship exists
- `candidate` - Selected for potential assignment
- `testing` - Actively monitoring, being evaluated
- `active` - Confirmed for long-term monitoring

#### Internal Candidate States
```go
// selector_state.go
type candidateState uint8

const (
    candidateUnknown candidateState = iota
    candidateIn                    // Should be promoted/kept active
    candidateOut                   // Should be demoted (gradual)
    candidateBlock                 // Should be removed immediately
    candidatePending               // Should remain as candidate
)
```

### Grandfathering Logic

Since network constraints are hardcoded, grandfathering is primarily needed for account limit changes. When an account's `monitor_per_server_limit` is reduced, existing assignments that exceed the new limit should be grandfathered.

```go
// selector_grandfathering.go
func (sl *selector) isGrandfathered(
    monitor *monitorCandidate,
    server *serverInfo,
    violation *constraintViolation,
) bool {
    // Only grandfather existing active/testing assignments
    if monitor.ServerStatus != ntpdb.ServerScoresStatusActive &&
       monitor.ServerStatus != ntpdb.ServerScoresStatusTesting {
        return false
    }

    // Network constraints are hardcoded, so they can't be grandfathered
    if violation.Type == violationNetwork {
        return false
    }

    // Account limit violations on existing assignments are grandfathered
    if violation.Type == violationLimit {
        return true
    }

    // Same account violations can't be grandfathered
    return false
}
```

### Selection Algorithm

#### Core Selection Logic
```go
// selector_process.go
func (sl *selector) processServer(db *ntpdb.Queries, serverID uint32) (bool, error) {
    // 1. Load server information
    server, err := sl.loadServerInfo(db, serverID)
    if err != nil {
        return false, err
    }

    // 2. Get all potential monitors using existing GetMonitorPriority query
    // This query now includes: monitor_status (global), account_id, monitor_ip, account_flags
    candidates, err := db.GetMonitorPriority(sl.ctx, serverID)
    if err != nil {
        return false, err
    }

    // 3. Build account limits from the monitor results (no separate query needed)
    accountLimits := sl.buildAccountLimitsFromMonitors(candidates)

    // 5. Categorize monitors with constraint checking
    categories := sl.categorizeMonitors(candidates, server, accountLimits)

    // 6. Apply selection rules
    changes := sl.applySelectionRules(categories, db, serverID)

    // 7. Execute changes
    return sl.executeChanges(changes, db, serverID)
}
```

#### Monitor Categorization with Constraints

**Important**: The `GetMonitorPriority` query returns ALL monitors that have recent monitoring data for this server, regardless of their global status. This includes monitors that might be globally pending, paused, or deleted but still have historical data. The categorization process MUST respect global status first before applying any other logic.

```go
// selector_process.go
func (sl *selector) categorizeMonitors(
    candidates []monitorCandidate,
    server *serverInfo,
    accountLimits map[uint32]*accountLimit,
) *monitorCategories {

    cat := &monitorCategories{
        active:           []evaluatedMonitor{},
        testing:          []evaluatedMonitor{},
        candidate:        []evaluatedMonitor{},
        available:        []evaluatedMonitor{},
        blocked:          []evaluatedMonitor{},
        globallyActiveCount: 0,
    }

    for _, monitor := range candidates {
        eval := evaluatedMonitor{
            monitor: monitor,
        }

        // Check constraints
        violation := sl.checkConstraints(&monitor, server, accountLimits)
        eval.violation = violation

        // Check if grandfathered
        if violation.Type != violationNone {
            violation.IsGrandfathered = sl.isGrandfathered(&monitor, server, violation)
        }

        // Determine candidate state based on violation and current status
        eval.recommendedState = sl.determineState(&monitor, violation)

        // Categorize based on current server-monitor status
        switch monitor.ServerStatus {
        case ntpdb.ServerScoresStatusActive:
            cat.active = append(cat.active, eval)
            if monitor.GlobalStatus == ntpdb.MonitorsStatusActive {
                cat.globallyActiveCount++
            }

        case ntpdb.ServerScoresStatusTesting:
            cat.testing = append(cat.testing, eval)
            if monitor.GlobalStatus == ntpdb.MonitorsStatusActive {
                cat.globallyActiveCount++
            }

        case ntpdb.ServerScoresStatusCandidate:
            cat.candidate = append(cat.candidate, eval)

        default: // new or unassigned
            if violation.Type == violationNone {
                cat.available = append(cat.available, eval)
            } else {
                cat.blocked = append(cat.blocked, eval)
            }
        }
    }

    return cat
}
```

#### State Determination Logic
```go
// selector_state.go
func (sl *selector) determineState(
    monitor *monitorCandidate,
    violation *constraintViolation,
) candidateState {

    // STEP 1: Check global monitor status FIRST (respecting monitors.status)
    switch monitor.GlobalStatus {
    case ntpdb.MonitorsStatusPending:
        // Pending monitors should gradually phase out to allow clean transition
        if monitor.ServerStatus == ntpdb.ServerScoresStatusActive ||
           monitor.ServerStatus == ntpdb.ServerScoresStatusTesting {
            log.Info("pending monitor will be gradually removed",
                "monitorID", monitor.ID,
                "globalStatus", monitor.GlobalStatus,
                "serverStatus", monitor.ServerStatus)
            return candidateOut // Gradual removal
        }
        // Pending monitors not assigned to this server should gradually phase out
        return candidateOut

    case ntpdb.MonitorsStatusPaused:
        // Paused monitors should stop all work immediately
        return candidateBlock

    case ntpdb.MonitorsStatusDeleted:
        // Deleted monitors must be removed
        return candidateBlock

    case ntpdb.MonitorsStatusTesting, ntpdb.MonitorsStatusActive:
        // Only these monitors can proceed to constraint checking
        // Continue to STEP 2

    default:
        // Unknown status = block
        log.Warn("unknown global monitor status",
            "monitorID", monitor.ID,
            "status", monitor.GlobalStatus)
        return candidateBlock
    }

    // STEP 2: Check for state inconsistencies
    if sl.hasStateInconsistency(monitor) {
        log.Warn("inconsistent monitor state detected",
            "monitorID", monitor.ID,
            "globalStatus", monitor.GlobalStatus,
            "serverStatus", monitor.ServerStatus)
        return candidateOut
    }

    // STEP 3: Apply constraint validation (only for globally active/testing monitors)
    if violation.Type != violationNone {
        if violation.IsGrandfathered {
            // Grandfathered violations get gradual removal
            return candidateOut
        }

        // New violations on unassigned monitors = block
        if monitor.ServerStatus == ntpdb.ServerScoresStatusNew {
            return candidateBlock
        }

        // New violations on assigned monitors = gradual removal
        return candidateOut
    }

    // STEP 4: Performance/health based logic
    if monitor.HasMetrics && !monitor.IsHealthy {
        return candidateOut
    }

    // STEP 5: Only globally active can be promoted to server-active
    if monitor.ServerStatus == ntpdb.ServerScoresStatusTesting &&
       monitor.GlobalStatus != ntpdb.MonitorsStatusActive {
        // Cannot promote to active unless globally active
        return candidateIn // Stay in testing
    }

    // Default: eligible for promotion/retention
    return candidateIn
}

// hasStateInconsistency checks for inconsistent global vs server states
func (sl *selector) hasStateInconsistency(monitor *monitorCandidate) bool {
    // Monitor globally pending but server-active/testing is inconsistent
    if monitor.GlobalStatus == ntpdb.MonitorsStatusPending &&
       (monitor.ServerStatus == ntpdb.ServerScoresStatusActive ||
        monitor.ServerStatus == ntpdb.ServerScoresStatusTesting) {
        return true
    }

    // Monitor globally deleted but still server-active is inconsistent
    if monitor.GlobalStatus == ntpdb.MonitorsStatusDeleted &&
       monitor.ServerStatus == ntpdb.ServerScoresStatusActive {
        return true
    }

    return false
}
```

### Constraint Validation Implementation

#### IP Assignment Note
All monitors have IP addresses assigned at registration time, so network constraint checking is always possible. The IP address is a required field when a monitor is registered in the system.

#### Network Constraint Checker
```go
// selector_constraints.go
import "net/netip"

func (sl *selector) checkNetworkConstraint(
    monitorIP string,
    serverIP string,
) error {
    if monitorIP == "" || serverIP == "" {
        return nil // Can't check without IPs
    }

    mAddr, err := netip.ParseAddr(monitorIP)
    if err != nil {
        return fmt.Errorf("invalid monitor IP: %w", err)
    }

    sAddr, err := netip.ParseAddr(serverIP)
    if err != nil {
        return fmt.Errorf("invalid server IP: %w", err)
    }

    // Must be same address family
    if mAddr.Is4() != sAddr.Is4() {
        return nil
    }

    var prefixLen int
    if mAddr.Is4() {
        prefixLen = defaultSubnetV4
    } else {
        prefixLen = defaultSubnetV6
    }

    // Check if in same subnet
    mPrefix, err := mAddr.Prefix(prefixLen)
    if err != nil {
        return fmt.Errorf("invalid prefix length %d: %w", prefixLen, err)
    }

    if mPrefix.Contains(sAddr) {
        return fmt.Errorf("monitor and server in same /%d network", prefixLen)
    }

    return nil
}
```

#### Account Constraint Checker
```go
// selector_constraints.go
func (sl *selector) checkAccountConstraints(
    monitor *monitorCandidate,
    server *serverInfo,
    accountLimits map[uint32]*accountLimit,
) error {
    // Check same account constraint
    if monitor.AccountID != nil && server.AccountID != nil {
        if *monitor.AccountID == *server.AccountID {
            return fmt.Errorf("monitor from same account as server")
        }
    }

    // Check account limits (per server, not global)
    // Each account can have up to X monitors per server (default 2)
    if monitor.AccountID != nil {
        limit, exists := accountLimits[*monitor.AccountID]
        if !exists {
            // No specific limit, use default
            limit = &accountLimit{
                AccountID: *monitor.AccountID,
                MaxPerServer: sl.defaultAccountLimit,
                CurrentCount: 0,
            }
        }

        // Count current assignments (excluding this monitor if already assigned)
        currentCount := limit.CurrentCount
        if monitor.ServerStatus == ntpdb.ServerScoresStatusActive ||
           monitor.ServerStatus == ntpdb.ServerScoresStatusTesting {
            currentCount-- // Don't count self
        }

        if currentCount >= limit.MaxPerServer {
            return fmt.Errorf("account %d at limit (%d/%d)",
                limit.AccountID, currentCount, limit.MaxPerServer)
        }
    }

    return nil
}
```

### Selection Rules Engine

#### Rule Application
```go
// selector_process.go
func (sl *selector) applySelectionRules(
    categories *monitorCategories,
    db *ntpdb.Queries,
    serverID uint32,
) []monitorChange {

    changes := []monitorChange{}

    // Calculate current state
    currentActive := len(categories.active)
    currentTesting := len(categories.testing)
    targetActive := 7        // Target number of active monitors
    targetTesting := 5      // Target number of testing monitors
    bootstrapThreshold := 3  // Minimum for bootstrap mode

    // Determine allowed changes
    maxChanges := sl.calculateMaxChanges(currentActive, targetActive)

    // Phase 1: Remove immediately blocked monitors
    changes = append(changes, sl.removeBlockedMonitors(categories.blocked, maxChanges)...)

    // Phase 2: Handle grandfathered violations
    changes = append(changes, sl.handleGrandfatheredMonitors(categories, maxChanges)...)

    // Phase 3: Ensure minimum testing pool requirements
    changes = append(changes, sl.ensureTestingPoolRequirements(categories, maxChanges)...)

    // Phase 4: Promote based on performance
    changes = append(changes, sl.promoteByPerformance(categories, maxChanges)...)

    // Phase 5: Fill available slots
    changes = append(changes, sl.fillAvailableSlots(categories, maxChanges)...)

    return changes
}
```

#### Testing Pool Requirements
```go
// selector_process.go
func (sl *selector) ensureTestingPoolRequirements(
    categories *monitorCategories,
    maxChanges int,
) []monitorChange {

    changes := []monitorChange{}
    minGloballyActive := 4

    // Count globally active monitors in testing/active states
    globallyActiveInTesting := 0
    for _, m := range categories.testing {
        if m.monitor.GlobalStatus == ntpdb.MonitorsStatusActive {
            globallyActiveInTesting++
        }
    }
    for _, m := range categories.active {
        if m.monitor.GlobalStatus == ntpdb.MonitorsStatusActive {
            globallyActiveInTesting++
        }
    }

    // Need more globally active monitors?
    if globallyActiveInTesting < minGloballyActive {
        needed := minGloballyActive - globallyActiveInTesting

        // Promote globally active candidates first
        for _, m := range categories.candidate {
            if needed <= 0 || len(changes) >= maxChanges {
                break
            }
            if m.monitor.GlobalStatus == ntpdb.MonitorsStatusActive &&
               m.violation.Type == violationNone {
                changes = append(changes, monitorChange{
                    MonitorID: m.monitor.ID,
                    From:      ntpdb.ServerScoresStatusCandidate,
                    To:        ntpdb.ServerScoresStatusTesting,
                    Reason:    "needed for globally active minimum",
                })
                needed--
            }
        }

        // Then look in available pool
        for _, m := range categories.available {
            if needed <= 0 || len(changes) >= maxChanges {
                break
            }
            if m.monitor.GlobalStatus == ntpdb.MonitorsStatusActive &&
               m.violation.Type == violationNone {
                changes = append(changes, monitorChange{
                    MonitorID: m.monitor.ID,
                    From:      ntpdb.ServerScoresStatusNew,
                    To:        ntpdb.ServerScoresStatusCandidate,
                    Reason:    "needed for globally active minimum",
                })
                needed--
            }
        }
    }

    return changes
}
```

#### Detailed Promotion Logic
```go
// selector_process.go
func (sl *selector) executePromotions(
    changes []monitorChange,
    db *ntpdb.Queries,
    serverID uint32,
) error {
    for _, change := range changes {
        switch {
        case change.From == ntpdb.ServerScoresStatusNew &&
             change.To == ntpdb.ServerScoresStatusCandidate:
            // Available → Candidate promotion
            // First, ensure server_score entry exists
            exists, err := sl.serverScoreExists(db, serverID, change.MonitorID)
            if err != nil {
                return fmt.Errorf("checking server_score existence: %w", err)
            }

            if !exists {
                // Insert new server_score entry
                err = db.InsertServerScore(sl.ctx, ntpdb.InsertServerScoreParams{
                    MonitorID: change.MonitorID,
                    ServerID:  serverID,
                    ScoreRaw:  0,
                    CreatedOn: time.Now(),
                })
                if err != nil {
                    log.Error("failed to insert server_score",
                        "monitorID", change.MonitorID,
                        "serverID", serverID,
                        "error", err)
                    continue
                }
            }

            // Update status to candidate
            err = db.UpdateServerScoreStatus(sl.ctx, ntpdb.UpdateServerScoreStatusParams{
                MonitorID: change.MonitorID,
                ServerID:  serverID,
                Status:    ntpdb.ServerScoresStatusCandidate,
            })
            if err != nil {
                return fmt.Errorf("updating to candidate: %w", err)
            }

            log.Info("promoted monitor to candidate",
                "monitorID", change.MonitorID,
                "serverID", serverID,
                "reason", change.Reason)

        case change.From == ntpdb.ServerScoresStatusCandidate &&
             change.To == ntpdb.ServerScoresStatusTesting:
            // Candidate → Testing promotion
            err := db.UpdateServerScoreStatus(sl.ctx, ntpdb.UpdateServerScoreStatusParams{
                MonitorID: change.MonitorID,
                ServerID:  serverID,
                Status:    ntpdb.ServerScoresStatusTesting,
            })
            if err != nil {
                return fmt.Errorf("updating to testing: %w", err)
            }

            log.Info("promoted monitor to testing",
                "monitorID", change.MonitorID,
                "serverID", serverID,
                "reason", change.Reason)

        case change.From == ntpdb.ServerScoresStatusTesting &&
             change.To == ntpdb.ServerScoresStatusActive:
            // Testing → Active promotion (performance-based)
            err := db.UpdateServerScoreStatus(sl.ctx, ntpdb.UpdateServerScoreStatusParams{
                MonitorID: change.MonitorID,
                ServerID:  serverID,
                Status:    ntpdb.ServerScoresStatusActive,
            })
            if err != nil {
                return fmt.Errorf("updating to active: %w", err)
            }

            log.Info("promoted monitor to active",
                "monitorID", change.MonitorID,
                "serverID", serverID,
                "reason", change.Reason)

        case change.To == ntpdb.ServerScoresStatusNew:
            // Demotion/removal - depends on current state
            // This handles candidateOut and candidateBlock removals
            if change.From == ntpdb.ServerScoresStatusActive ||
               change.From == ntpdb.ServerScoresStatusTesting {
                err := db.UpdateServerScoreStatus(sl.ctx, ntpdb.UpdateServerScoreStatusParams{
                    MonitorID: change.MonitorID,
                    ServerID:  serverID,
                    Status:    ntpdb.ServerScoresStatusTesting,
                })
                if err != nil {
                    return fmt.Errorf("demoting monitor: %w", err)
                }

                log.Info("demoted monitor",
                    "monitorID", change.MonitorID,
                    "serverID", serverID,
                    "from", change.From,
                    "reason", change.Reason)
            }
        }
    }

    return nil
}

// Helper to check if server_score entry exists
func (sl *selector) serverScoreExists(
    db *ntpdb.Queries,
    serverID uint32,
    monitorID uint32,
) (bool, error) {
    _, err := db.GetServerScore(sl.ctx, ntpdb.GetServerScoreParams{
        ServerID:  serverID,
        MonitorID: monitorID,
    })
    if err == sql.ErrNoRows {
        return false, nil
    }
    if err != nil {
        return false, err
    }
    return true, nil
}
```

## Implementation Phases

### Phase 1: File Restructuring (Foundation) ✅ COMPLETED
**Duration**: 1 day

1. Create new file structure ✅
2. Move existing code to appropriate files ✅
3. Ensure all tests still pass ✅
4. Update imports and dependencies ✅

**Files created**:
- `selector_types.go` - Moved newStatus and newStatusList type definitions ✅
- `selector_state.go` - Moved candidateState enum and IsOutOfOrder method ✅
- `selector.go` - Kept minimal with core logic ✅
- `candidatestate_enumer.go` - Generated by enumer tool ✅

**Commit**: 392bab0 - "refactor: extract selector types and state logic into separate files"

### Phase 2: Database Schema Updates ✅ COMPLETED
**Duration**: 1 day

1. Create migration scripts ✅
2. ~~Add `account_monitor_limits` table~~ Using existing `accounts.flags` JSON instead ✅
3. ~~Add `monitor_constraint_configs` table~~ Hardcoded constraints in application ✅
4. Extend `server_scores.status` enum ✅
5. Add constraint tracking columns ✅

**Migration**: Created #143 in `/Users/ask/src/ntppool/sql/ntppool.update` - APPLIED ✅

### Phase 3: Constraint System Core ✅ COMPLETED
**Duration**: 2-3 days

1. Implement `selector_constraints.go` ✅
   - Network constraint validation with hardcoded /24 and /48 limits ✅
   - Account constraint validation with state-based limits ✅
   - Build account limits from GetMonitorPriority results ✅
2. Update `GetMonitorPriority` query to include account_id, monitor_ip, and account_flags ✅
3. Implement state-based account limits (active=X, testing=X+1, total=X+1) ✅
4. Create basic grandfathering logic in `selector_grandfathering.go` ✅
5. Implement state determination logic in `selector_state.go` ✅
6. Add candidatePending to candidateState enum ✅

**Key deliverables**:
- Network subnet checking with net/netip using hardcoded limits ✅
- State-based account limit enforcement ✅
- No separate database query needed - all data from GetMonitorPriority ✅
- Basic constraint system with all core types and functions ✅

**Commit**: 970f83b - "feat(scorer): implement constraint validation system for monitor selection"

### Phase 4: Grandfathering System ✅ COMPLETED
**Duration**: 2 days

1. ~~Implement `selector_grandfathering.go`~~ Basic implementation done in Phase 3 ✅
2. Add constraint violation tracking in database ✅
   - Added constraint fields to GetMonitorPriority query ✅
   - Created UpdateServerScoreConstraintViolation query ✅
   - Created ClearServerScoreConstraintViolation query ✅
   - Implemented trackConstraintViolations() in selector_tracking.go ✅
3. ~~Implement historical config lookup~~ Not needed with hardcoded constraints ✅
4. ~~Create grandfathering detection logic~~ Enhanced to use stored violations ✅
5. Test grandfathering scenarios - TODO in Phase 7

**Key features**:
- ~~Detect configuration changes~~ Only for account limits (hardcoded network constraints) ✅
- Track violation start times using constraint_violation_since column ✅
- Differentiate new vs. grandfathered violations ✅
- Preserve violation timestamps across selector runs ✅

**New files**:
- `selector_tracking.go` - Database tracking of constraint violations

**Commit**: 5329158 - "feat(scorer): add constraint violation tracking to database"

### Phase 5: Enhanced State Machine ✅ COMPLETED
**Duration**: 1 day

1. ~~Add candidate state to enum~~ Done in Phase 2 (database) ✅
2. ~~Update state transition logic~~ Basic logic in selector_state.go ✅
3. ~~Implement state determination based on constraints~~ determineState() implemented ✅
4. Test all state transitions ✅

**States to handle**:
- available → candidate ✅
- candidate → testing ✅
- testing → active ✅
- any → removed (gradual or immediate) ✅

**Test coverage**:
- State machine transitions tested ✅
- Constraint validation tested ✅
- Grandfathering logic tested ✅
- Edge cases covered ✅

**Commit**: 0325b15 - "test(scorer): add comprehensive tests for state machine and constraints"

### Phase 6: Selection Algorithm Rewrite ✅ COMPLETED
**Duration**: 3-4 days

1. ~~Rewrite `processServer` in `selector_process.go`~~ ✅
2. ~~Implement monitor categorization~~ ✅
3. ~~Add testing pool requirements (4+ globally active)~~ ✅
4. ~~Implement gradual removal logic~~ ✅
5. ~~Add comprehensive logging~~ ✅

**Implementation details**:
- Created new `processServerNew` method to avoid conflicts during transition
- Integrated all constraint components from previous phases
- Used existing `GetServer` query instead of creating new one
- Added `GetAvailableMonitors` and `DeleteServerScore` queries
- Implemented selection rules with proper state transitions
- Added helper methods for counting and selection logic
- Created comprehensive test file to verify compilation

**Key components**:
- `selector_process.go` - Main selection algorithm (520 lines)
- `selector_process_test.go` - Unit tests for helpers
- Updated `query.sql` with 2 new queries
- Constants: 7 active, 5 testing, 4 globally active in testing

**Next steps**:
- Integration testing with real database
- Performance benchmarking
- Shadow mode implementation for safe rollout

### Phase 7: Testing Suite
**Duration**: 2-3 days

1. Unit tests for each module
2. Integration tests for workflows
3. Constraint change scenarios
4. Performance benchmarks
5. Migration testing

**Test scenarios**:
- Bootstrap with no monitors
- Constraint tightening/relaxing
- Account limit changes
- Network configuration changes
- Grandfathering edge cases

### Phase 8: Monitoring and Observability
**Duration**: 1 day

1. Add metrics for constraint violations
2. Track grandfathering statistics
3. Monitor removal rates
4. Add alerting for anomalies
5. Create operational dashboards

**Metrics to track**:
- Constraint violations by type
- Grandfathered monitor count
- Removal rates
- Selection algorithm performance

### Phase 9: Feature Flag and Rollout
**Duration**: 1 day

1. Implement feature flag system
2. Create gradual rollout plan
3. Document rollback procedures
4. Prepare operational runbooks
5. Train operations team

### Phase 10: Production Deployment
**Duration**: 1 week (gradual)

1. Deploy to staging environment
2. Enable for subset of servers
3. Monitor metrics and logs
4. Gradual production rollout
5. Full deployment

## Testing Strategy

### Unit Test Coverage

**selector_constraints_test.go**
```go
func TestNetworkConstraintIPv4(t *testing.T) {
    tests := []struct {
        name      string
        monitorIP string
        serverIP  string
        subnet    int
        shouldFail bool
    }{
        {"same /24", "192.168.1.10", "192.168.1.20", 24, true},
        {"different /24", "192.168.1.10", "192.168.2.20", 24, false},
        {"same /16", "192.168.1.10", "192.168.2.20", 16, true},
        {"different /16", "192.168.1.10", "192.169.1.20", 16, false},
    }
    // ... test implementation
}

func TestAccountLimitConstraint(t *testing.T) {
    tests := []struct {
        name         string
        accountID    uint32
        currentCount int
        limit        int
        shouldFail   bool
    }{
        {"under limit", 1, 1, 2, false},
        {"at limit", 1, 2, 2, true},
        {"over limit", 1, 3, 2, true},
    }
    // ... test implementation
}
```

**selector_grandfathering_test.go**
```go
func TestGrandfatheringDetection(t *testing.T) {
    tests := []struct {
        name              string
        assignedAt        time.Time
        oldConfig         constraintConfig
        newConfig         constraintConfig
        currentViolation  bool
        expectGrandfathered bool
    }{
        {
            name: "network constraint tightened",
            oldConfig: constraintConfig{SubnetV4: 24},
            newConfig: constraintConfig{SubnetV4: 16},
            currentViolation: true,
            expectGrandfathered: true,
        },
        // ... more test cases
    }
    // ... test implementation
}
```

### Integration Test Scenarios

**Full Workflow Tests**
1. **Bootstrap Scenario**: Empty server, only available monitors
2. **Constraint Change**: Tighten network constraints, verify gradual removal
3. **Account Limit Reduction**: Reduce limit from 3 to 2, verify behavior
4. **Mixed Constraints**: Multiple simultaneous constraint violations

**Performance Tests**
1. **Large Scale**: 1000+ monitors, 100+ servers
2. **Constraint Checking**: Measure overhead of validation
3. **Grandfathering Lookup**: Historical config retrieval performance

### Constraint Change Test Matrix

| Scenario | Initial State | Change | Expected Behavior |
|----------|--------------|--------|-------------------|
| Network tightening | /24 constraint, monitors in same /16 | Change to /16 | Grandfathered, gradual removal |
| Network relaxing | /16 constraint | Change to /24 | More monitors become available |
| Account limit decrease | 3 monitors per account | Reduce to 2 | Excess monitors marked for gradual removal |
| Account limit increase | 2 monitors per account | Increase to 3 | Can add more monitors |
| New same-account server | Active monitors | Add server from same account | Cannot assign those monitors |

## Migration and Deployment

### Database Migration Steps

**Migration Script** (`/Users/ask/src/ntppool/sql/ntppool.update`):
```sql
#143
-- Extend server_scores.status enum to include 'candidate' state
ALTER TABLE server_scores
MODIFY COLUMN status enum('new','candidate','testing','active') NOT NULL DEFAULT 'new';

-- Add constraint tracking columns to server_scores
ALTER TABLE server_scores
ADD COLUMN constraint_violation_type varchar(50) DEFAULT NULL,
ADD COLUMN constraint_violation_since datetime DEFAULT NULL,
ADD INDEX idx_constraint_violation (constraint_violation_type, constraint_violation_since);
```

### Feature Flag Implementation

```go
// selector.go
type featureFlags struct {
    UseEnhancedSelector bool
    EnforceNetworkConstraints bool
    EnforceAccountConstraints bool
    EnableGrandfathering bool
}

func (sl *selector) loadFeatureFlags() (*featureFlags, error) {
    flags := &featureFlags{
        UseEnhancedSelector: false,  // Start disabled
        EnforceNetworkConstraints: false,
        EnforceAccountConstraints: false,
        EnableGrandfathering: false,
    }

    // Load from database or environment
    if val, err := sl.db.GetSystemSetting(sl.ctx, "enhanced_selector"); err == nil {
        flags.UseEnhancedSelector = val == "true"
    }

    // Similar for other flags...
    return flags, nil
}
```

### Rollout Plan

**Week 1: Preparation**
- Deploy code with features disabled
- Run migration scripts
- Verify no regressions

**Week 2: Shadow Mode**
- Enable enhanced selector in log-only mode
- Compare decisions with production
- Identify any issues

**Week 3: Limited Rollout**
- Enable for 10% of servers
- Monitor metrics closely
- Verify constraint enforcement

**Week 4: Gradual Increase**
- Increase to 50% of servers
- Enable grandfathering
- Monitor removal rates

**Week 5: Full Deployment**
- Enable for all servers
- Remove feature flags
- Document new behavior

### Rollback Procedures

**Immediate Rollback** (any phase):
```sql
-- Disable enhanced selector
UPDATE system_settings SET value = 'false' WHERE key = 'enhanced_selector';
```

**Full Rollback** (if needed):
```sql
-- Revert server_scores status
ALTER TABLE server_scores
MODIFY COLUMN status enum('new','testing','active') NOT NULL DEFAULT 'new';

-- Remove constraint tracking
ALTER TABLE server_scores
DROP COLUMN constraint_violation_type,
DROP COLUMN constraint_violation_since;

-- Remove new tables (careful - may have data)
DROP TABLE IF EXISTS account_monitor_limits;
DROP TABLE IF EXISTS monitor_constraint_configs;
```

## Monitoring and Observability

### Key Metrics

**Constraint Metrics**
- `monitor_selector_constraint_violations_total{type="network|account|limit"}`
- `monitor_selector_grandfathered_monitors{type="network|account|limit"}`
- `monitor_selector_constraint_checks_duration_seconds`

**Selection Metrics**
- `monitor_selector_state_transitions_total{from="",to=""}`
- `monitor_selector_available_monitors{server=""}`
- `monitor_selector_testing_pool_size{server="",globally_active=""}`

**Performance Metrics**
- `monitor_selector_process_duration_seconds{server=""}`
- `monitor_selector_constraint_lookup_duration_seconds`

### Alerting Rules

```yaml
# High constraint violation rate
- alert: HighConstraintViolationRate
  expr: rate(monitor_selector_constraint_violations_total[5m]) > 10
  annotations:
    summary: "High rate of constraint violations"

# Testing pool below minimum
- alert: InsufficientGloballyActiveMonitors
  expr: monitor_selector_testing_pool_size{globally_active="true"} < 4
  annotations:
    summary: "Server {{ $labels.server }} has insufficient globally active monitors"

# Excessive grandfathering
- alert: ExcessiveGrandfathering
  expr: monitor_selector_grandfathered_monitors > 50
  annotations:
    summary: "High number of grandfathered monitors may indicate configuration issues"
```

### Operational Dashboards

**Constraint Overview Dashboard**
- Violation counts by type
- Grandfathered monitors over time
- Constraint configuration history
- Account limit distribution

**Selection Performance Dashboard**
- Selection algorithm duration
- State transition rates
- Monitor pool sizes
- Change frequencies

## Risk Assessment and Mitigation

### High-Risk Areas

1. **Mass Removal Risk**
   - **Scenario**: Configuration change causes many monitors to violate constraints
   - **Mitigation**: Grandfathering system, gradual removal limits
   - **Monitoring**: Alert on high removal rates

2. **Insufficient Monitor Coverage**
   - **Scenario**: Constraints too restrictive, can't find valid monitors
   - **Mitigation**: Minimum viable monitor count, bootstrap mode
   - **Monitoring**: Alert on servers below minimum monitors

3. **Performance Degradation**
   - **Scenario**: Constraint checking adds significant overhead
   - **Mitigation**: Efficient algorithms, caching, benchmarking
   - **Monitoring**: Track selection duration metrics

### Medium-Risk Areas

1. **Configuration Drift**
   - **Scenario**: Multiple configuration versions cause confusion
   - **Mitigation**: Clear versioning, audit trail
   - **Monitoring**: Configuration change tracking

2. **Account Limit Edge Cases**
   - **Scenario**: Dynamic account changes affect limits
   - **Mitigation**: Regular limit reconciliation
   - **Monitoring**: Account limit distribution metrics

### Mitigation Strategies

1. **Gradual Changes**: Never remove more than 20% of monitors in one cycle
2. **Dry Run Mode**: Test configuration changes before applying
3. **Emergency Override**: Allow operators to disable constraints if needed
4. **Audit Trail**: Log all constraint decisions for debugging

## Conclusion

This implementation plan provides a comprehensive approach to enhancing the monitor selector with sophisticated constraint validation and grandfathering support. The phased approach ensures safe deployment while the modular design improves maintainability. The grandfathering system ensures that configuration changes don't cause operational disruptions, while the comprehensive testing and monitoring strategy provides confidence in the system's behavior.

### Key Design Principle: Global Status First

The enhanced selector ALWAYS respects global monitor status as the primary filter:
1. **Step 1**: Check `monitors.status` (pending/paused/deleted → exclude)
2. **Step 2**: Check state consistency (inconsistent → gradual removal)
3. **Step 3**: Apply constraints (network/account violations)
4. **Step 4**: Evaluate performance (if data available)
5. **Step 5**: Make promotion decisions

This ensures that:
- Globally pending monitors are gradually phased out rather than immediately blocked
- Globally paused monitors stop all work immediately
- Only globally active monitors can be promoted to server-active status
- The existing `GetMonitorPriority` query's `monitor_status` field is properly utilized

Key benefits:
- **Operational Stability**: Gradual transitions prevent service disruptions
- **Flexibility**: Easy to adjust constraints as requirements evolve
- **Maintainability**: Modular design makes future changes easier
- **Observability**: Comprehensive metrics and logging for operations
- **Safety**: Multiple safeguards against mass removal scenarios
- **Correctness**: Global status is always respected before server-specific logic
