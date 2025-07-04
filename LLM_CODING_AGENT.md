# LLM_CODING_AGENT.md

This file provides guidance to Claude Code (claude.ai/code) and other LLM-based coding agents when working with code in this repository.

## Project Overview

The NTP Pool Monitor is a distributed monitoring system for the NTP Pool project. It consists of three main components:

- `ntppool-agent` - Monitoring client that runs on distributed nodes
- `monitor-api` - Central API server for coordination and configuration
- `monitor-scorer` - Processes monitoring results and calculates server scores

## Task Management

**Use TodoWrite/TodoRead tools for complex multi-step tasks (3+ steps) or when user explicitly requests todo management.**

**Best Practices:**
- Break down complex tasks into specific, actionable items
- Mark tasks `in_progress` before starting work (only ONE at a time)
- Complete tasks immediately after finishing
- Only mark `completed` when fully accomplished

## Pre-Commit Checklist

**MANDATORY before any git commit:**

1. Run `gofumpt -w` on all changed `.go` files
2. Run `go test ./...` to ensure all tests pass
3. Verify compilation with `go build` for affected packages
4. Run lint tools if available (`golangci-lint run`, `go vet ./...`)
5. Check for race conditions and proper error handling

**Never commit changes unless explicitly asked by the user.**
**NEVER USE `git add -A` - always use explicit, targeted git staging.**

## Development Commands

```bash
make tools          # Install required development tools
make generate       # Generate all code (runs sqlc then go generate ./...)
make build          # Build all components
make test           # Run comprehensive test suite
```

## Code Generation

**Never edit generated files directly** - changes will be lost on regeneration.

**Generated file patterns:**
- `*.pb.go` - Protocol buffer generated files
- `*.sql.go` - sqlc generated database code
- `otel.go` - OpenTelemetry instrumentation wrappers
- `*_string.go` - Enum string methods
- Files in `api/pb/` and `gen/` directories

**Run `make generate` after:**
- Modifying `query.sql` or `.proto` files
- Adding/modifying `//go:generate` directives
- Use `./scripts/test-db.sh start` for SQL query testing

## Problem Analysis Framework

**Systematic debugging approach:**
1. Understand exact symptoms and failure conditions
2. Trace code flow step by step
3. Identify state, caching, and persistence points
4. Consider simple explanations first (connection pooling, timing, config)
5. Verify assumptions before implementing solutions
6. Check for race conditions and concurrent operations
7. Prefer targeted fixes over architectural changes

**Common constraint system bugs:**
- **Self-reference bugs**: Check if entities compare against lists including themselves
- **Order-dependent logic**: Verify processing order matches business priorities
- **State consistency**: Ensure constraint checks use correct state (target vs current)

**SQL analysis tips:**
- Examine ORDER BY clauses for data prioritization
- Check JOIN patterns for constraint relationships
- Use query ordering to understand conflict resolution

## Task Completion Criteria

**Before marking any coding task as complete:**
1. Code compiles successfully (`go build`)
2. Tests pass (`go test ./...`)
3. Code is formatted (`gofumpt -w`)
4. Basic functionality verified

**Never mark completed if:** tests fail, implementation is partial, or compilation errors exist.

## Constraint Checking Architecture Patterns

**Common patterns in monitor selection systems:**
- **Iterative constraint checking**: Check constraints after each promotion/demotion
- **Emergency override logic**: Safety overrides for critical system states
- **Self-exclusion**: Always exclude entity being evaluated from conflict detection
- **Target vs current state**: Check constraints against appropriate state
- **Grandfathering**: Allow existing violations to persist temporarily

**Implementation best practices:**
- Lazy evaluation, consistent error handling, audit logging

## Concurrency and Thread Safety

**Race condition detection:**
- Identify shared state accessed by multiple goroutines
- Ensure write operations use `Lock()`, not `RLock()`
- Check atomic operations and channel usage

**Mutex best practices:**
- Use write locks for modifications, read locks for reads
- Minimize lock scope and avoid nested locks
- Create `methodUnsafe()` variants that assume lock is held

**Common patterns:**
- Configuration hot reloading with mutex protection
- Background goroutines with proper context cancellation
- Atomic file operations with proper locking

## Go Code Standards

**Logging:**
- Use `*slog.Logger` type for logger fields
- Use contextual logging: `log.InfoContext(ctx, ...)`
- Get logger from context: `log := logger.FromContext(ctx)`

**Error Handling:**
- Always include context in error messages
- Use structured logging for errors with relevant fields

**CLI Framework (Kong):**
- CLI commands defined as structs with Kong struct tags
- Common tags: `name:"flag-name"`, `short:"x"`, `help:"Description"`, `default:"value"`
- Follow existing patterns in `client/cmd/cmd.go`

**Testing:**
- Use table-driven tests
- Avoid `testify/assert` or similar tools
- SQL queries tested through integration tests
- Use `./scripts/test-db.sh start` for database testing

**Test Data Validation:**
- Validate constraint mathematics ensure expected outcomes are possible
- Cross-check test logic before writing assertions
- Document mathematical relationships in comments
- Use CI tools (`./scripts/test-ci-local.sh`) for debugging

## Key Architecture Components

### Core Packages

- `client/` - Client-side monitoring agent implementation
- `client/monitor/` - NTP monitoring logic using beevik/ntp library
- `client/config/` - Configuration management with TLS certificates and hot reloading
- `server/` - API server with JWT auth and Connect RPC endpoints
- `api/` - Protocol definitions using Protocol Buffers and Connect RPC
- `scorer/` - Server performance scoring algorithms
- `ntpdb/` - Database layer using MySQL with sqlc for type-safe queries

### Monitor Types

The system supports two distinct types of monitors with different purposes and lifecycles:

#### 1. Regular Monitors (`type = 'monitor'`)

**Purpose**: Distributed NTP monitoring clients that test server performance
- **Implementation**: `ntppool-agent` clients running on user systems
- **Data Flow**: Submit test results via monitor-api gRPC/ConnectRPC endpoints
- **Status Management**: Managed by selector through proper constraint checking
- **Status Progression**: `candidate` → `testing` → `active`
- **Assignment**: Automatically assigned to compatible servers via `GetServers` API
- **Location**: `client/` package and related monitoring code

**Lifecycle**:
1. Monitor submits test results → Creates `log_scores` entries
2. Scorer processes results → Creates `server_scores` with `candidate` status
3. Selector evaluates server → Promotes based on constraints and health
4. Constraint violations → Gradual demotion through status hierarchy

#### 2. Scorer Monitors (`type = 'score'`)

**Purpose**: Meta-monitors that calculate aggregate server performance scores
- **Implementation**: Backend processes that analyze monitoring data
- **Data Flow**: Process `log_scores` from regular monitors to compute server scores
- **Status Management**: Automatically set to `active` status when processing scores
- **Assignment**: Manually configured, not subject to selector constraint checking
- **Location**: `scorer/` package

**Lifecycle**:
1. Scorer processes `log_scores` entries from regular monitors
2. Creates/updates `server_scores` entries with calculated performance metrics
3. Status automatically forced to `active` during score calculation (this is correct behavior)
4. Not subject to selector's constraint checking or promotion logic

#### Key Differences

| Aspect | Regular Monitors | Scorer Monitors |
|--------|------------------|-----------------|
| **Purpose** | Test individual servers | Calculate aggregate scores |
| **Data Source** | Direct NTP measurements | Processed monitoring data |
| **Status Flow** | `candidate` → `testing` → `active` | Always `active` when processing |
| **Constraint Checking** | Full selector constraint validation | Not subject to constraints |
| **Assignment** | Automatic via selector logic | Manual configuration |
| **Count Limits** | Subject to account/network limits | No limits (system-managed) |

#### Database Identification

Monitors are identified by the `type` field in the `monitors` table:
```sql
SELECT * FROM monitors WHERE type = 'monitor';  -- Regular monitoring clients
SELECT * FROM monitors WHERE type = 'score';    -- Scorer/meta-monitors
```

The `server_scores` table contains entries from both types, but they serve different purposes:
- **Regular monitors**: Status managed by selector for constraint compliance
- **Scorer monitors**: Status automatically managed for operational needs

### Configuration Management Architecture

**Two separate configuration endpoints with different purposes and frequencies:**

1. **HTTP Config Endpoint** (`/monitor/api/config`)
   - **Frequency**: Every 5 minutes + immediate fsnotify triggers on state.json changes
   - **Purpose**: Basic monitor setup, IP assignments, and TLS certificate management
   - **Location**: `client/config/appconfig.go:LoadAPIAppConfig()`

2. **gRPC Config Endpoint** (`api.GetConfig`)
   - **Frequency**: Every 60 minutes + immediate triggers when HTTP config changes
   - **Purpose**: Monitor-specific operational configuration per IP version
   - **Location**: `client/cmd/monitor.go:fetchConfig()`

**Hot Reloading System:**
- `fsnotify` watches `state.json` for immediate response to setup command changes
- HTTP config changes trigger immediate gRPC config refresh for all monitor goroutines
- Context-based notification system with proper cleanup to prevent memory leaks
- Broadcast mechanism supports multiple concurrent monitor goroutines (one per IP version)

### Certificate Management

**Certificate Lifecycle and Timing:**
- **Initial Setup**: `ntppool-agent setup` obtains API key but NOT certificates
- **Activation Required**: Monitors must be marked "active" or "testing" in the API before certificates can be requested
- **First Certificate Request**: Happens on first `LoadAPIAppConfig()` call after monitor activation
- **Certificate Storage**: Stored alongside state.json in the state directory
- **Hot Reloading**: Certificate changes are immediately detected and loaded

**Wait Method Usage:**
- **`WaitUntilAPIKey()`**: Use when only API key is needed (e.g., initial setup verification)
- **`WaitUntilConfigured()`**: Use when both API key AND certificates are required (e.g., API operations)
- **`WaitUntilCertificatesLoaded()`**: Internal method for waiting specifically for certificates
- **`WaitUntilLive()`**: Use when monitor must be in active/testing state with valid IP assignment

### State Directory Configuration

**systemd StateDirectory vs RuntimeDirectory:**
- **StateDirectory** (`/var/lib/ntppool-agent`): Persistent storage that survives reboots
- **RuntimeDirectory** (`/var/run/ntppool-agent`): Temporary storage cleared on reboot
- **Migration**: Automatic migration from RuntimeDirectory to StateDirectory on startup
- **Priority Order**: `$MONITOR_STATE_DIR` > `$STATE_DIRECTORY` > user config directory

**State Migration Best Practices:**
- Check for `RUNTIME_DIRECTORY` environment variable on startup
- Migrate state.json and certificate files if found
- Log migration operations for debugging
- Handle partial migrations gracefully (e.g., state.json exists but certificates don't)

### Configuration Sources and Hierarchy

**AppConfig (Local State):**
- Stored in state.json
- Contains: API key, monitor name, TLS name, IP assignments, status per protocol
- Updated via HTTP endpoint every 5 minutes
- Triggers immediate notifications on changes via `WaitForConfigChange()`

**gRPC Config (Operational Config):**
- Fetched via Connect RPC from monitor-api
- Contains: NTP test parameters, server lists, MQTT settings
- Updated every 60 minutes or when AppConfig changes
- Requires valid certificates for authentication

**Configuration Flow:**
1. Setup command → API key stored in state.json
2. Monitor activation in web UI → Status changes to "testing" or "active"
3. First LoadAPIAppConfig() → Receives certificates
4. Subsequent API calls → Can fetch gRPC config

### Monitor Lifecycle and Status Checking

**Status Values:**
- **active**: Monitor is fully operational
- **testing**: Monitor is in test mode (still operational)
- **pending**: Monitor should gradually phase out (allows clean transitions)
- **paused**: Monitor should stop all work immediately

**Status Checking Best Practices:**
- **Check in outer loop**: Before spawning monitor goroutines
- **Use fresh config**: Call `IPv4()`/`IPv6()` to get current status, not stale captures
- **Wait for activation**: Use `WaitForConfigChange()` when paused
- **Avoid inner loop checks**: Don't check status inside monitoring loops

**Example Pattern:**
```go
// Outer loop - check status before starting monitors
ipc := cli.Config.IPv4()
if !ipc.IsLive() {
    // Wait for activation using WaitForConfigChange
    for {
        configChangeCtx := cli.Config.WaitForConfigChange(ctx)
        select {
        case <-configChangeCtx.Done():
            ipc = cli.Config.IPv4() // Get fresh status
            if ipc.IsLive() {
                break
            }
        case <-ctx.Done():
            return nil
        }
    }
}
// Now safe to start monitoring
```

### Communication

- **Connect RPC** (replacing legacy Twirp) for client-server communication
- **MQTT** for real-time messaging and live monitoring updates
- **TLS certificates** for mutual authentication via Vault or API

### Database

- **MySQL** backend with **sqlc** for compile-time verified SQL
- **ClickHouse** support for analytics and traceroute data

### Database Schema Management

- **Schema Changes**: Database schema changes are handled automatically by the deployment system
- **Schema File**: `schema.sql` contains the current database schema
- **Local Development**: Use MySQL 8 in Docker (available via `make test-db-start` or `./scripts/test-db.sh start`)
- **No Manual Migrations**: The codebase handles schema updates automatically during deployment
- **Version Tracking**: Schema versions always increment forward and are managed separately from the code

### Concurrent Operations and Race Conditions

When implementing database operations that might be called concurrently:

1. **Check for Duplicate Key Constraints**: Review table schemas for unique constraints
2. **Use Idempotent Operations**: Prefer `INSERT ... ON DUPLICATE KEY UPDATE` over plain `INSERT`
3. **Test Concurrent Scenarios**: Integration tests should include concurrent operation tests
4. **Regenerate After SQL Changes**: Always run `make sqlc` after modifying query.sql

Example pattern for safe inserts:
```sql
INSERT INTO table (col1, col2) VALUES (?, ?)
ON DUPLICATE KEY UPDATE col2 = VALUES(col2);
```

## Environment Configuration

Key environment variables:

- `DEPLOYMENT_MODE` - Environment (devel/test/prod)
- `DATABASE_DSN` - MySQL connection string
- `JWT_KEY` - JWT signing key for MQTT auth
- `VAULT_ADDR` - Vault server URL for secrets
- `OTEL_EXPORTER_OTLP_ENDPOINT` - OpenTelemetry collector

Database credentials can be provided via `database.yaml`:

```yaml
mysql:
  user: some-db-user
  pass: password
```

## Incremental Development Methodology

### Phase-Based Development
For complex changes, break work into distinct phases:

**Phase 1: Foundation/Bug Fixes**
- Fix critical bugs, race conditions, and safety issues first
- Establish proper synchronization and error handling
- Ensure existing functionality remains intact
- Complete testing and validation before proceeding

**Phase 2: Core Implementation**
- Add new features and functionality
- Implement hot reloading, configuration changes, or new APIs
- Maintain backward compatibility throughout
- Test each increment independently

**Phase 3: Future Considerations**
- Document potential improvements and optimizations
- Plan for scalability and maintenance
- Consider deprecation paths for legacy components
- Defer non-essential changes to future iterations

### Implementation Best Practices
- **Test each phase independently**: Don't proceed until current phase is stable
- **Maintain rollback capability**: Each phase should be independently revertible
- **Use plan mode for complex changes**: Present architectural decisions for approval
- **Document assumptions and dependencies**: Make implicit requirements explicit
- **Prefer targeted fixes over architectural overhauls**: Simple solutions first

### Change Management
- **Incremental commits**: Each phase gets its own commit with clear description
- **Interface stability**: Don't break existing APIs without migration paths
- **Configuration compatibility**: Ensure new config works with existing deployments
- **Monitoring continuity**: Verify metrics and logging remain functional

## Development Workflow

1. **After schema changes**: Run `make sqlc` to regenerate database code
2. **After protobuf changes**: Run `make generate` to regenerate RPC code
3. **Before commits**: Run `gofumpt -w` on all modified Go files
4. **Testing**: Run `make test` to validate changes
5. **Phase validation**: Complete and test each development phase before proceeding
6. **Incremental commits**: Commit each stable phase independently

## Monitoring Features

- Dual-stack IPv4/IPv6 monitoring with automatic IP detection
- High-precision NTP accuracy testing with configurable sampling
- Network traceroute integration for path analysis
- Real-time scoring algorithms for server performance evaluation
- OpenTelemetry integration for distributed tracing and metrics
- Certificate-based mutual authentication

## Additional Guidelines

**Documentation:**
- Maintain `cmd/[command]/README.md` files for each command
- Use terse, standard Go doc comments
- Update README.md when user-facing options change

**Backwards Compatibility:**
- If a change affects both agent and monitoring server, update both together
- Maintain backwards compatibility with older monitoring server versions
- Use `version.CheckVersion` function and follow existing versioning patterns

**Security:**
- Never commit secrets, credentials, or sensitive data to git
- Secrets may exist in working directory but must be excluded from version control

**Code Quality:**
- Flag any existing or new global variables and panics
- When introducing third-party packages, ask for confirmation
- Discourage global variables and disallow panics unless absolutely necessary

**APIs:**
- Legacy Twirp API is frozen and will be removed after legacy clients are upgraded
- Flag any new code using Twirp or modifying the Twirp-based API
- Prefer Connect RPC API for all new development

**General Guidance:**
- If context is missing or unclear, ask for clarification before proceeding
- When creating new files, place them inside `/Users/ask/src/go/ntp/monitor`
- When editing files, use `...existing code...` comments to indicate unchanged regions

**API Endpoint Configuration:**
- Deployment environment set via `--env` flag or `DEPLOYMENT_MODE` environment variable
- Environment represented by `depenv.DeploymentEnvironment` type from `go.ntppool.org/common/config/depenv`
- API endpoint resolved by calling `depenv.DeploymentEnvironment.APIHost()` or `MonitorAPIHost()`
- Override with `DEVEL_API_SERVER` environment variable
- Default endpoints defined in `depenv` package: prod (`https://api.mon.ntppool.dev`), test (`https://api.test.mon.ntppool.dev`), devel (`https://api.devel.mon.ntppool.dev`)

## CI Tools and Testing Infrastructure

**Key scripts in `scripts/` directory:**
- `test-db.sh` - Primary test database management (MySQL 8.0 on port 3308)
- `test-ci-local.sh` - Full CI environment emulation
- `test-scorer-integration.sh` - Component-specific testing
- `diagnose-ci.sh` - CI failure diagnostics

**Database ports:**
- 3307: Scorer integration tests
- 3308: Main test database (default)
- 3309: CI diagnostics

**Local workflow:**
1. `./scripts/test-db.sh start`
2. `go test ./...`
3. `./scripts/test-db.sh stop`

## Recent Architecture Changes

**Key recent changes:**
- **Selector Package Refactoring**: Moved to dedicated `selector/` package with new constraint validation algorithm
- **"New" Status Elimination**: Eliminated "new" status entirely; simplified flow to `candidate → testing → active`
- **Per-Status-Group Change Limits**: Separate limits for each status transition type in `selector/process.go`
- **Dynamic Testing Pool Sizing**: Testing pool adjusts based on active monitor gap
- **Monitor Limit Enforcement**: Fixed monitor count tracking and rule execution order
- **Configuration Management**: Transitioned to systemd StateDirectory for persistent storage

**Monitor Selection Rules** (in `selector/process.go`):
- Rule 1: Immediate blocking
- Rule 2: Gradual constraint removal
- Rule 1.5: Active excess demotion
- Rule 3: Testing to active promotion
- Rule 5: Candidate to testing promotion
- Rule 2.5: Testing pool management
- Rule 6: Bootstrap promotion
