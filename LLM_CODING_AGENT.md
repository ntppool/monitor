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
6. **Prefer targeted fixes** - Avoid complex architectural changes when simple solutions exist

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

### Testing
- Use table-driven tests
- Avoid `testify/assert` or similar tools
- Use mocks only when necessary
- Follow existing test patterns in the codebase

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

### Communication

- **Connect RPC** (replacing legacy Twirp) for client-server communication
- **MQTT** for real-time messaging and live monitoring updates
- **TLS certificates** for mutual authentication via Vault or API

### Database

- **MySQL** backend with **sqlc** for compile-time verified SQL
- **ClickHouse** support for analytics and traceroute data

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

## Pre-Commit Best Practices

- **Before committing code:**
  - Run `go test ./...` to ensure all tests pass
  - Run `gofumpt -w` on changed `.go` files to ensure consistent formatting


Your last name is MonitorAI.
