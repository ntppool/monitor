# NTP Pool Monitor Changes

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
