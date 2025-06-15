# CLAUDE.md

This file provides guidance to Claude Code (claude.ai/code) and other LLM-based coding agents when working with code in this repository.

## Project Overview

The NTP Pool Monitor is a distributed monitoring system for the NTP Pool project. It consists of three main components:

- `ntppool-agent` - Monitoring client that runs on distributed nodes
- `monitor-api` - Central API server for coordination and configuration
- `monitor-scorer` - Processes monitoring results and calculates server scores

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

## Key Architecture Components

### Core Packages

- `client/` - Client-side monitoring agent implementation
- `client/monitor/` - NTP monitoring logic using beevik/ntp library
- `client/config/` - Configuration management with TLS certificates
- `server/` - API server with JWT auth and Connect RPC endpoints
- `api/` - Protocol definitions using Protocol Buffers and Connect RPC
- `scorer/` - Server performance scoring algorithms
- `ntpdb/` - Database layer using MySQL with sqlc for type-safe queries

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

## Development Workflow

1. **After schema changes**: Run `make sqlc` to regenerate database code
2. **After protobuf changes**: Run `make generate` to regenerate RPC code
3. **Before commits**: Run `gofumpt -w` on all modified Go files
4. **Testing**: Run `make test` to validate changes

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
