# LLM_CODING_AGENT.md

This file provides guidance to Claude Code (claude.ai/code) and other LLM-based coding agents when working with code in this repository.

## Project Overview

The NTP Pool Monitor is a distributed monitoring system for the NTP Pool project. It consists of three main components:

- `ntppool-agent` - Monitoring client that runs on distributed nodes
- `monitor-api` - Central API server for coordination and configuration
- `monitor-scorer` - Processes monitoring results and calculates server scores

## Task Management with Todo Tools

**Use TodoWrite/TodoRead tools for complex multi-step tasks:**

### When to Use Todo Tools
- Complex tasks requiring 3+ distinct steps or operations
- Non-trivial tasks requiring careful planning or multiple operations
- When user explicitly requests todo list management
- When user provides multiple tasks (numbered or comma-separated)
- After receiving new instructions to capture requirements

### Todo Management Best Practices
- **Break down complex tasks** into specific, actionable items
- **Update status in real-time**: Mark tasks as `in_progress` BEFORE starting work
- **Complete tasks immediately** after finishing - don't batch completions
- **Only have ONE task in_progress** at any time
- **Use clear, descriptive task names** that indicate exactly what needs to be done
- **Remove irrelevant tasks** from the list entirely when no longer needed

### Task States
- `pending`: Task not yet started
- `in_progress`: Currently working on (limit to ONE task at a time)
- `completed`: Task finished successfully

### Task Completion Requirements
- **ONLY mark completed when FULLY accomplished**
- Keep as `in_progress` if encountering errors, blockers, or partial completion
- Create new tasks for discovered issues or dependencies
- Never mark completed if tests fail, implementation is partial, or errors exist

## Pre-Commit Checklist

**MANDATORY: Before any git commit or task completion:**

1. **Run `gofumpt -w`** on all changed `.go` files to fix formatting and whitespace
2. **Run `go test ./...`** to ensure all tests pass
3. **Verify compilation** with `go build` for affected packages
4. **Check for Go lint tools** and run them if available (e.g., `golangci-lint run`, `go vet ./...`)
5. **Analyze concurrency and thread safety** - Check for race conditions, proper mutex usage, and thread-safe operations
6. **Verify error handling** - Ensure proper error propagation and structured logging
7. **Test incremental changes** - Validate each phase of complex changes independently

**Never commit changes unless explicitly asked by the user.**

## Development Commands

### Setup and Code Generation

```bash
make tools          # Install required development tools (buf, sqlc, protoc-gen-*)
make generate       # Generate code from protobuf and SQL definitions (required after schema/proto changes)
make sqlc           # Generate type-safe SQL code from query.sql
```

### Build and Test

```bash
make build          # Build all components
make test           # Run comprehensive test suite
gofumpt -w          # Format Go code (required before commits)
```

## Problem Analysis Framework

When diagnosing issues, follow this systematic approach:

1. **Understand the exact symptoms** - What specifically isn't working? When does it fail?
2. **Trace the code flow** - Follow the execution path step by step
3. **Identify state and caching** - Where is data stored, cached, or persisted?
4. **Consider simple explanations first** - Connection pooling, timing issues, configuration
5. **Verify assumptions** - Test each hypothesis before implementing solutions
6. **Check for race conditions** - Concurrent operations, database constraints, timing dependencies
7. **Prefer targeted fixes** - Avoid complex architectural changes when simple solutions exist

### Common Bug Patterns in Constraint Systems

When debugging constraint violations or unexpected behavior:

1. **Self-Reference Bugs**: Check if entities are being compared against lists that include themselves
   - Look for functions that take an entity ID and a list of entities
   - Verify that self-exclusion logic exists (e.g., `if existing.ID == currentID { continue }`)
   - Common in: network diversity checks, conflict detection, constraint validation

2. **Order-Dependent Logic**: Ensure processing order matches business priorities
   - Check SQL ORDER BY clauses to understand intended priority
   - Verify that "first wins" vs "last wins" logic aligns with business rules
   - Look for cases where evaluation order affects outcomes

3. **State Consistency Issues**: Verify that constraint checks use the correct state
   - Check constraints against target state for promotions, not current state
   - Ensure iterative updates don't create race conditions
   - Validate that account limits and counts are updated after each change

### SQL Query Analysis for Business Logic Understanding

When working with data-driven systems:

1. **Examine ORDER BY Clauses**: Understanding data prioritization
   - Check how entities are ordered (priority, health, performance metrics)
   - Understand if processing order affects business outcomes
   - Look for composite ordering (multiple criteria)

2. **Analyze JOIN Patterns**: Understanding data relationships
   - Identify which tables provide which constraints
   - Check for INNER vs LEFT JOINs that affect data availability
   - Look for aggregate functions that summarize constraint data

3. **Query-Driven Behavior Analysis**: Let SQL inform code understanding
   - Use query ordering to understand which entities should "win" conflicts
   - Check WHERE clauses for filtering logic that affects constraint evaluation
   - Examine GROUP BY clauses to understand aggregation boundaries

Example: `ORDER BY healthy desc, monitor_priority asc` means healthy monitors with lower priority values are processed first, so they get first choice in constraint conflicts.

## Task Completion Criteria

Before marking any coding task as complete:

1. **Code compiles successfully** - Run `go build` on affected packages
2. **Tests pass** - Run `go test ./...` (or package-specific tests)
3. **Code is formatted** - Run `gofumpt -w` on changed files
4. **Basic functionality verified** - Test the implemented feature works as expected
5. **No compilation or runtime errors** - Verify the code actually works

Never mark a task as completed if:
- Tests are failing
- Implementation is partial
- Unresolved compilation errors exist
- You couldn't verify the functionality works

## Constraint Checking Architecture Patterns

### Common Patterns in Monitor Selection Systems

1. **Iterative Constraint Checking**
   - Check constraints against current state after each promotion/demotion
   - Update working limits and counts incrementally
   - Prevent simultaneous violations by processing changes sequentially

2. **Emergency Override Logic**
   - Implement safety overrides for critical system states (e.g., zero active monitors)
   - Document when constraint violations should be ignored for system health
   - Use clear naming and logging for override conditions

3. **Self-Exclusion in Constraint Checks**
   - Always exclude the entity being evaluated from conflict detection
   - Pass entity IDs to constraint functions for proper self-filtering
   - Test self-comparison scenarios explicitly

4. **Target State vs Current State Validation**
   - Check constraints against the target state for promotions
   - Check constraints against current state for status maintenance
   - Don't check constraints when demoting (constraints may be why we're demoting)

5. **Grandfathering and Gradual Transitions**
   - Allow existing constraint violations to persist temporarily
   - Implement gradual removal rather than immediate blocking
   - Track violation timestamps for aging out grandfathered exceptions

### Implementation Best Practices

- **Lazy Evaluation**: Only evaluate constraints when needed for decisions
- **Consistent Error Handling**: Return structured constraint violation information
- **Audit Logging**: Log all constraint checks and their outcomes for debugging
- **Performance Monitoring**: Track constraint evaluation overhead and optimize bottlenecks

## Concurrency and Thread Safety

### Race Condition Detection
- **Identify shared state**: Look for variables accessed by multiple goroutines
- **Analyze mutex patterns**: Ensure write operations use `Lock()`, not `RLock()`
- **Check atomic operations**: Verify proper synchronization for concurrent access
- **Review channel usage**: Ensure proper buffering and deadlock prevention

### Mutex Best Practices
- **Use write locks for write operations**: `Lock()` for modifications, `RLock()` for reads
- **Minimize lock scope**: Hold locks for the shortest time possible
- **Avoid nested locks**: Prevent deadlocks by consistent lock ordering
- **Split safe/unsafe methods**: Create `methodUnsafe()` variants that assume lock is held

### Common Concurrency Patterns
- **Configuration hot reloading**: Use mutex-protected load/save cycles
- **Background goroutines**: Ensure proper context cancellation and cleanup
- **Shared resources**: Protect with appropriate synchronization primitives
- **Channel communication**: Use buffered channels to prevent blocking

### Thread Safety Analysis
- **Load/save operations**: Ensure atomic file operations with proper locking
- **API call sequences**: Protect multi-step operations with single mutex
- **State transitions**: Verify consistent state during concurrent modifications
- **Error handling**: Ensure error paths don't leave locks held

## Go Code Standards

### Logging
- Use `*slog.Logger` type for logger fields
- Use contextual logging: `log.InfoContext(ctx, ...)` and `log.DebugContext(ctx, ...)`
- Get logger from context: `log := logger.FromContext(ctx)`
- Create loggers with: `logger.Setup().WithGroup("component-name")`

### Error Handling
- Always include context in error messages
- Use structured logging for errors with relevant fields
- Follow existing patterns for error wrapping and propagation

### Imports and Organization
- Follow existing import grouping patterns
- Use consistent naming conventions for packages
- Prefer explicit imports over dot imports

### CLI Framework (Kong)
- This codebase uses the Kong CLI framework for command-line argument parsing
- CLI commands are defined as structs with Kong struct tags:
  - `name:"flag-name"` - Sets the flag name
  - `short:"x"` - Sets single-letter short flag (e.g., `-x`)
  - `help:"Description"` - Sets help text for the flag
  - `default:"value"` - Sets default value
  - `env:"ENV_VAR"` - Allows setting via environment variable
  - `required:""` - Makes the flag required
  - `negatable:""` - Allows `--no-flag` variants for boolean flags
- Follow existing patterns in `client/cmd/cmd.go` and `client/cmd/setup.go` for new CLI options
- Command structs are embedded in the main `ClientCmd` struct with `cmd:""` tags
- Use descriptive help text that explains the purpose and usage of each flag

### Testing
- Use table-driven tests
- Avoid `testify/assert` or similar tools
- Use mocks only when necessary
- Follow existing test patterns in the codebase

### Test Data Validation and Mathematical Correctness

When creating test data:

1. **Validate Constraint Mathematics**: Ensure test conditions make the expected outcome possible
   - For account limits: verify that total counts don't exceed limits in impossible ways
   - For network constraints: confirm IP addresses are actually in different/same networks as intended
   - For time-based constraints: ensure timestamps and durations are realistic

2. **Cross-Check Test Logic**: Before writing assertions
   - Calculate expected outcomes manually based on test data
   - Verify that business rules would actually produce the expected result
   - Check for off-by-one errors in limits and counts

3. **Test Data Documentation**: Include comments explaining the mathematical relationships
   - Document why specific limits were chosen
   - Explain how counts relate to limits
   - Show the calculation that leads to expected outcomes

Example:
```go
// MaxPerServer=2, ActiveCount=1, TestingCount=2
// Total limit = MaxPerServer + 1 = 3
// Current total = 1 + 2 = 3 (at limit)
// Promoting testing->active: would become 2 active + 1 testing = 3 (still valid)
accountLimits := map[uint32]*accountLimit{
    1: {AccountID: 1, MaxPerServer: 2, ActiveCount: 1, TestingCount: 2},
}
```

### Integration Test Debugging

When debugging failing integration tests, especially those involving databases:

1. **Understand Data Dependencies**: Trace through test setup to identify all required data relationships
   - Check what monitors, servers, and scores are created
   - Verify that all foreign key relationships are satisfied
   - Pay special attention to the difference between monitor types ('monitor' vs 'score')

2. **Verify Query Requirements**: Check SQL queries to understand what data they expect
   - Use `Grep` to find query definitions
   - Read the actual SQL to understand JOIN conditions and WHERE clauses
   - Ensure test data satisfies all query conditions

3. **Use CI Tools First**: Before running tests manually, use the provided CI scripts:
   - `./scripts/test-ci-local.sh` - Full CI environment emulation
   - `./scripts/test-scorer-integration.sh` - Component-specific tests
   - These scripts ensure proper database setup and isolation

## Key Architecture Components

### Core Packages

- `client/` - Client-side monitoring agent implementation
- `client/monitor/` - NTP monitoring logic using beevik/ntp library
- `client/config/` - Configuration management with TLS certificates and hot reloading
- `server/` - API server with JWT auth and Connect RPC endpoints
- `api/` - Protocol definitions using Protocol Buffers and Connect RPC
- `scorer/` - Server performance scoring algorithms
- `ntpdb/` - Database layer using MySQL with sqlc for type-safe queries

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

---

# LLM/Copilot Coding Agent Instructions

## Generated Files

- **Never edit generated files directly.**
  - Treat files in `api/`, files with `.pb.go` or `.sql.go` extensions, and files in `ntpdb/` as generated unless they are known to be hand-written (ask if unsure).
  - If a change is needed to a generated file, automatically run `go generate ./...`.

## Documentation

- **Per-command documentation:**
  - Maintain or create `cmd/[command]/README.md` files for each command.
  - The central `README.md` should reference per-command documentation and provide a high-level overview.
  - Follow existing formats for new documentation; ask for clarification if unsure.
- **Godoc comments:**
  - Use terse, standard Go doc comments.
- **User-facing options:**
  - Update the relevant README.md and example configuration files or CLI help output when user-facing options change.

## Backwards Compatibility

- **Agent and monitoring server:**
  - If a change affects both, update both together.
  - Maintain backwards compatibility with older monitoring server versions.
  - Use the `version.CheckVersion` function and follow existing versioning patterns.
  - Add comments when introducing new version-dependent logic.

## Deprecated Code

- Use the standard Go `// Deprecated:` comment above deprecated functions or types, including for operationally deprecated code.
- Track migration and deprecation paths in `MIGRATIONS.md` using a semi-structured format (date, summary, affected components, removal plan as appropriate).

## Testing

- Use table-driven tests.
- Avoid using `testify/assert` or similar tools.
- Use mocks only when necessary.

## Breaking Changes

- **Do not introduce breaking changes to the client API** without providing a migration or deprecation path.
- Document migration/deprecation paths in `MIGRATIONS.md` and in code comments as appropriate.
- Provide a summary and ask for explicit approval before making changes to shared interfaces or core types.

## Twirp API

- The legacy Twirp API is frozen and will be removed after legacy clients are upgraded.
- **Flag any new code using Twirp or modifying the Twirp-based API.**
- Prefer and prioritize the Connect RPC API for all new development.

## Security and Secrets

- **Never commit secrets, credentials, or sensitive data to git.**
- Secrets may exist in the working directory (e.g., `database.yaml`) but must be excluded from version control.

## Logging, Metrics, and Telemetry

- Use structured logging (`log/slog` via `go.ntppool.org/logger`) for all errors, always including context. If an exception is appropriate, ask for confirmation.
- For connection errors and reconnections, always log with context (e.g., remote address, error type, retry count).
- Follow existing patterns for metrics and telemetry instrumentation.

## Code Quality and Dependencies

- Flag any existing or new global variables and panics; avoid introducing new ones.
- When introducing a third-party package not already in go.mod, ask for confirmation and discuss options.
- Discourage global variables and disallow panics unless absolutely necessary and justified.

### Code Quality Best Practices

- **Use camelCase for variable names** - Follow Go naming conventions (e.g., `ipVersion` not `ip_version`)
- **Document exported identifiers** - All exported functions, types, and variables must have godoc comments
- **Define constants for magic numbers** - Replace hardcoded timeouts, limits, and other values with named constants
- **Check for nil pointers** - Always verify pointers are not nil before dereferencing
- **Remove unused code** - Eliminate unused variables, imports, and dead code that exists only to avoid compiler warnings

### Code Review Approach

- **Use the Task tool for comprehensive analysis** - When reviewing code quality across multiple files, use Task tool for systematic analysis rather than manual file-by-file review
- **Prioritize safety issues first** - Address potential panics, nil pointer dereferences, and other safety issues before style improvements
- **Batch similar changes** - Use MultiEdit for multiple changes in the same file to improve efficiency
- **Verify changes** - Run tests after code quality improvements to ensure functionality remains intact

## General Guidance

- If context is missing or something is unclear, ask for clarification before proceeding.
- When creating new files, place them inside `/Users/ask/src/go/ntp/monitor`.
- When editing files, avoid repeating existing code; use `...existing code...` comments to indicate unchanged regions.

## Default API Endpoint Configuration

The ntppool-agent determines the monitor-api server endpoint as follows:

- The deployment environment is set via the `--env` flag or `DEPLOYMENT_MODE` environment variable (see `client/cmd/cmd.go`).
- The environment is represented by the `depenv.DeploymentEnvironment` type from `go.ntppool.org/common/config/depenv`.
- The API endpoint is resolved by calling `depenv.DeploymentEnvironment.APIHost()` or `MonitorAPIHost()`.
- In `api/rpc.go`, `getServerName()` uses `depenv.GetDeploymentEnvironmentFromName(clientName)` and then `depEnv.MonitorAPIHost()`.
- If the `DEVEL_API_SERVER` environment variable is set, it overrides the endpoint.
- The default endpoints are defined in the `depenv` package:
  - prod: `https://api.mon.ntppool.dev`
  - test: `https://api.test.mon.ntppool.dev`
  - devel: `https://api.devel.mon.ntppool.dev`

To change the default, set the appropriate environment variable or update the logic in the `depenv` package. See also the `README.md` for user-facing configuration options.

## CI Tools and Testing Infrastructure

### Test Database Management

The project includes comprehensive CI tools in the `scripts/` directory:

- **`scripts/test-db.sh`** - Primary test database management
  - Commands: `start`, `stop`, `restart`, `status`, `logs`, `shell`, `reset`
  - Creates MySQL 8.0 container on port 3308
  - Auto-loads schema and generates test data
  - Usage: `./scripts/test-db.sh start` then get connection string with `./scripts/test-db.sh status`

### CI Test Runners

- **`scripts/test-ci-local.sh`** - Emulates full Drone CI locally using Docker Compose
- **`scripts/test-minimal-ci.sh`** - Minimal test runner for debugging specific failures
- **`scripts/test-ci-debug.sh`** - Step-by-step CI debugging with verbose output
- **`scripts/test-scorer-integration.sh`** - Dedicated scorer component testing

### When to Use CI Tools

**Always use CI tools instead of manual testing when**:
- Debugging integration test failures
- Testing with a clean database state
- Reproducing CI environment issues locally
- Running concurrent or performance tests

**Tool Selection Guide**:
- `test-ci-local.sh`: Full CI replication - use for final validation
- `test-scorer-integration.sh`: Component testing - use for focused debugging
- `test-minimal-ci.sh`: Quick isolation testing
- `test-db.sh`: Direct database management

### Diagnostic Tools

- **`scripts/diagnose-ci.sh`** - Comprehensive CI failure diagnostics
  - Tests MySQL connectivity, Go environment, and runs basic tests
  - Use when CI tests fail to quickly identify root cause

### Build and Release

- **`scripts/build-linux-race.sh`** - Builds Linux amd64 binary with race detector
- **`scripts/run-goreleaser`** - Automated release packaging with Harbor registry integration
- **`scripts/update-man-page`** - Generates man pages for ntppool-agent

### Database Port Allocation

To avoid conflicts, different test scenarios use specific ports:
- **3307**: Scorer integration tests
- **3308**: Main test database (default)
- **3309**: CI diagnostics

### Local Development Workflow

1. **Start test database**: `./scripts/test-db.sh start`
2. **Run tests**: `go test ./...`
3. **Debug failures**: `./scripts/test-ci-debug.sh` or `./scripts/diagnose-ci.sh`
4. **Stop database**: `./scripts/test-db.sh stop`

## Recent Architecture Changes (June 2025)

### Selector Package Refactoring
- Selector implementation moved to dedicated `selector/` package
- New constraint validation algorithm for server scoring
- Added candidate status tracking in `server_scores` table

### Testing Infrastructure Improvements
- Enhanced integration test framework
- Improved error handling for monitor activation tests
- Added comprehensive testing patterns for API operations

### Configuration Management Updates
- Transitioned to systemd StateDirectory for persistent storage
- Added account parameter to setup command
- Improved hot-reloading system with better error recovery

## Pre-Commit Best Practices

- **Before committing code:**
  - Run `go test ./...` to ensure all tests pass
  - Run `gofumpt -w` on changed `.go` files to ensure consistent formatting
  - For integration tests, use `./scripts/test-db.sh start` and run full test suite
  - Use `./scripts/test-ci-local.sh` to validate against full CI environment


Your last name is MonitorAI.
