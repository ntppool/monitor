# NTP Pool Monitor Changes


- **MQTT session takeover backoff**
  When the broker reports a session takeover (reasonCode `0x8E`), the agent escalates the reconnect backoff (2m → 5m → 10m → 15m) instead of reconnecting on the 10s default, and pauses NTP checks and result submission while another client holds the session. The backoff resets after 30 minutes of stable connection. The disconnect reasonCode is now always logged.

## v4.1.2

### Client
- **Configurable log levels**: New `--log-level` flag and `MONITOR_LOG_LEVEL` env var for stderr; OTLP log level is server-controlled via gRPC config and cached to state.json; `--debug` overrides both to DEBUG
- **OTEL service name**: Set explicit `OTEL_SERVICE_NAME=ntppool-agent` so logs appear in Loki with the correct service name

### Packaging
- **Systemd detection**: Postinstall script exits gracefully on non-systemd systems and in containers where systemctl exists but systemd isn't PID 1

### API
- New internal CA

### Build
- Build with Go 1.26 and refresh dependencies
- Migrate CI from Drone to Woodpecker

## v4.1.1

### Packaging
- **Replace legacy units on upgrade**: Postinstall disables all `ntppool-monitor@*` systemd units and clears failed unit records; goreleaser `conflicts` directive ensures clean package replacement

### Build
- Upgrade goreleaser

## v4.1.0

### Server
- **Access logs**: Include real client IP (via Fastly + RFC1918 XFF) and monitor name from the auth context
- **Middleware ordering**: Run authentication before logging so the certificate name is available when logs are generated (fixes `monitor=unknown` and `certificateKey didn't return a string` errors)

### Scorer
- **Score deduplication**: Reject out-of-order timestamps, fix zero-score edge cases in percentage comparison, and extract magic numbers to named constants
- **Skip unresolvable old log_scores**: Prevents the scorer from falling more than 3 hours behind when older entries can't be scored
- **SQL update metrics**: New `scorer_sql_updates_total` counter broken down by operation type to identify frequent update patterns

### API
- **Fewer stratum updates**: Minimize stratum update queries that were unnecessarily busy on the database

### Packaging
- **Replace legacy package**: Add `replaces` directive so deb/rpm/apk packages automatically uninstall the old `ntppool-monitor` package on upgrade

### Build
- Build with Go 1.25
- Allow manual build triggers; disable MySQL/MariaDB cert verification in test config

## v4.0.5

### MQTT & Ad Hoc Requests
- **Handler registration fix**: Restored MQTT ad hoc NTP query functionality broken in router migration
- **MQTT connection fix**: Fix immediate disconnnections in the client
- **Version filtering**: Ad hoc requests now only sent to clients on version 4.0.4+ or exactly 3.8.6

## v4.0.3

### Monitor Selection
- **Performance-based replacement**: Testing monitors can replace worse-performing active monitors
- **Safety thresholds**: Prevent removing all monitors when performance is universally poor
- **Auto-pause for constraint violations**: Monitors with unchangeable constraints (subnet/account) are paused instead of repeatedly failing
- **Data point requirements**: Minimum 9 measurements for testing promotion, 60 for active promotion
- **Account limit fixes**: Allow monitor swaps at account boundaries
- **Priority 0 fix**: Optimal performance monitors can now be promoted correctly

### Scoring & Reliability
- **Deadlock retry**: Exponential backoff for database deadlocks with telemetry
- **Paused server scores**: Added support for paused status in scoring

### Developer Tools
- **Simulation mode**: `selector simulate` command for safe algorithm testing
- **Server targeting**: `--server-id` parameter for processing specific servers

### Build
- **32-bit support**: Added 386 architecture builds with softfloat

## v4.0.2

### Packaging Improvements
- **RPM dependency alternatives**: Enabled dependency alternatives for RPM packages to improve installation compatibility
- **APK dependency cleanup**: Removed unnecessary APK package dependencies for Alpine Linux builds

### Debugging Features
- **NTP query debugging**: Added `MONITOR_DEBUG_NTP_QUERIES` environment variable for detailed NTP query logging

## v4.0.1

### Bug Fixes
- **Mutex crash fix**: Updated common package dependency to resolve critical mutex-related crashes
- **Authentication fixes**: Improved dual mTLS/JWT authentication support with proper `RequestClientCert` handling

### API Changes
- **Legacy API blocking**: Twirp API now blocked for monitors who upgraded to v4.0.0 and newer

## v4.0.0

## Breaking Changes

### Monitor Registration (REQUIRED ACTION)
- **v3.x monitors must re-register** using the new `ntppool-agent setup` process
- Old registration methods no longer supported
- New API key system replaces Vault-based authentication

## Monitor Agent Changes

### New Registration System
- New `setup` command for monitor provisioning
- Simplified dual-stack operation: single API key manages both IPv4 and IPv6 monitoring
- Improved configuration management with persistent state directory
- Enhanced error handling and retry logic for API operations
- Much improved setup flow for new monitors on the website

### Reliability Improvements
- Enhanced authentication and API communication
- Reduced registration delays between IPv4/IPv6 protocols
- Better handling of network connectivity issues
- Improved logging and diagnostic capabilities

### Platform Support
- Added RISC-V 64-bit architecture support
- Updated to Go 1.24 with latest dependencies

## Scoring Algorithm Changes

### Monitor Selection
- **Dynamic testing pool sizing**: Testing pool now adjusts based on active monitor availability
- **Performance-based replacement**: Monitors are selected based on performance metrics
- **Network diversity constraints**: Improved geographic and network distribution
- **Per-status change limits**: Prevents mass monitor changes that could affect server scores

### Scoring Improvements
- **Multi-segment offset scoring**: Updated scoring algorithm with stricter optimal performance thresholds (25ms vs previous 75ms) and two-segment linear degradation ranges
- **Stratum validation**: Raised threshold to 10
- **Response selection**: Prefers valid NTP responses over timeout errors
- **Bootstrap logic**: Emergency override system helps new servers get initial monitoring coverage
- **Scheduler optimization**: Uses queue timestamps for more efficient server scheduling

### Status Management
- Improved constraint checking for monitor promotions
- Better handling of monitors with no historical scores

## Technical Changes

### API & Communication
- New Connect RPC API replacing legacy Twirp
- Enhanced authentication with JWT tokens and bearer authorization
- OpenTelemetry integration for improved monitoring and debugging
- Updated command-line interface using Kong for better flag parsing

### Configuration
- Uses systemd StateDirectory for persistent configuration storage
- Hot-reloading configuration changes without service restart
- Improved environment detection and API endpoint resolution

---

**Migration Note**: Operators running v3.x monitors should plan for re-registration as the old authentication system is no longer supported in v4.0.0.
