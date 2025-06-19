# Client Configuration Management Test Documentation

This document provides comprehensive documentation of all tests in the `client/config` package, clarifying what each test validates and what it does NOT test.

## Test Structure Overview

The test suite is organized into 5 main categories:

1. **Test Utilities** (`testutil_test.go`) - Shared test infrastructure
2. **File System Operations & Persistence** (`config_persist_test.go`) - Core file operations
3. **Configuration Interface** (`appconfig_test.go`) - Public API behavior
4. **fsnotify & Hot Reloading** (`appconfig_manager_test.go`) - File watching and manager lifecycle
5. **Certificate Management** (`appconfig_certs_test.go`) - TLS certificate handling

---

## Test Utilities (`testutil_test.go`)

### Purpose
Provides shared test infrastructure and helper functions for all configuration tests.

### Test Utilities Provided

#### `setupTestConfig(t *testing.T) (*testEnv, func())`
**Tests:** Test environment initialization
**Validates:**
- Temporary directory creation works
- AppConfig can be created with test environment
- Proper cleanup function is returned
**Does NOT test:**
- Real production configuration loading
- Network connectivity or API calls
- File system permissions in production environments

#### `waitForEvent(t *testing.T, waiter *ConfigChangeWaiter, timeout time.Duration) bool`
**Tests:** Configuration change notification timing
**Validates:**
- Configuration change events can be detected within timeout
- Timeout behavior works correctly
**Does NOT test:**
- What triggers the configuration change
- Multiple simultaneous waiters
- Event ordering or sequencing

#### `generateFileChange(t *testing.T, path string, content []byte)`
**Tests:** Atomic file operations simulation
**Validates:**
- Atomic write-then-rename pattern works
- File creation with proper permissions
**Does NOT test:**
- Race conditions during the atomic operation
- File system failure scenarios
- Cross-platform atomic operation behavior

#### `createTestStateFile(t *testing.T, dir, apiKey string)`
**Tests:** Valid state.json file creation
**Validates:**
- Correct JSON structure generation
- Proper directory structure creation
**Does NOT test:**
- Invalid JSON scenarios
- Large file handling
- File encoding issues

#### `mockWatcher` struct and methods
**Tests:** File system event simulation
**Validates:**
- Controllable event generation for testing
- Proper channel management and cleanup
- Thread-safe operation
**Does NOT test:**
- Real file system event generation
- Platform-specific file system behavior
- Event timing accuracy

#### `countWaiters(cfg AppConfig) int`
**Tests:** Memory leak detection helper
**Validates:**
- Access to internal waiter count
**Does NOT test:**
- Waiter functionality itself
- Memory usage patterns
- Cleanup timing

---

## File System Operations & Persistence Tests (`config_persist_test.go`)

### `TestReplaceFileAtomic`

#### `TestReplaceFileAtomic/basic_file_replacement`
**Tests:** Core atomic file replacement
**Validates:**
- File content is written correctly
- Atomic rename operation succeeds
- Original file is replaced atomically
**Does NOT test:**
- Concurrent access during replacement
- File permission inheritance
- Cross-file-system rename behavior

#### `TestReplaceFileAtomic/empty_file_handling`
**Tests:** Edge case of zero-length files
**Validates:**
- Empty files can be written atomically
- No errors occur with zero-length content
**Does NOT test:**
- Very large file handling
- Binary vs text content differences

#### `TestReplaceFileAtomic/concurrent_readers`
**Tests:** Reader safety during atomic operations
**Validates:**
- Multiple readers can access file during replacement
- No partial reads occur during replacement
- No reader errors during atomic operation
**Does NOT test:**
- Concurrent writers (only one writer is atomic)
- File locking mechanisms
- Reader behavior after file replacement

#### `TestReplaceFileAtomic/error_handling`
**Tests:** Error recovery and cleanup
**Validates:**
- Temporary files are cleaned up on errors
- Directory creation errors are handled
- Write errors don't leave corrupt files
**Does NOT test:**
- Disk space exhaustion scenarios
- File system corruption
- Permission denied errors

#### `TestReplaceFileAtomic/concurrent_replacement`
**Tests:** Multiple simultaneous atomic operations
**Validates:**
- Multiple atomic operations don't interfere
- Each operation maintains atomicity
- No race conditions between operations
**Does NOT test:**
- Same-file concurrent replacement
- Shared temporary directory contention

### `TestStatePersistence`

#### `TestStatePersistence/save_load_cycle`
**Tests:** Basic persistence functionality
**Validates:**
- Configuration data survives save/load cycle
- JSON serialization/deserialization works
- File paths are correct
**Does NOT test:**
- Large configuration data
- Invalid JSON handling
- File format versioning

#### `TestStatePersistence/concurrent_save_load`
**Tests:** Thread safety of persistence operations
**Validates:**
- Simultaneous save/load operations don't corrupt data
- Proper locking prevents race conditions
**Does NOT test:**
- Process-level concurrency (multiple processes)
- Network file systems
- File locking across processes

#### `TestStatePersistence/partial_updates`
**Tests:** Incremental configuration changes
**Validates:**
- Only changed data triggers save operations
- Unchanged data doesn't cause unnecessary writes
**Does NOT test:**
- Change detection accuracy in complex data structures
- Memory usage patterns
- Change notification timing

#### `TestStatePersistence/large_state_files`
**Tests:** Performance with large configuration data
**Validates:**
- Large JSON files can be processed
- Memory usage remains reasonable
- Performance doesn't degrade significantly
**Does NOT test:**
- Extremely large files (>100MB)
- Memory-mapped file access
- Streaming JSON processing

#### `TestStatePersistence/corrupted_state_recovery`
**Tests:** Error recovery from invalid state files
**Validates:**
- Invalid JSON is detected and handled
- System continues with default configuration
- Corrupted files don't crash the application
**Does NOT test:**
- Automatic corruption repair
- Backup file creation
- User notification of corruption

---

## Configuration Interface Tests (`appconfig_test.go`)

### `TestAPIKeyManagement`

#### `TestAPIKeyManagement/set_and_get_api_key`
**Tests:** Basic API key operations
**Validates:**
- API key can be set and retrieved
- Empty API key handling works
- API key changes are persisted
**Does NOT test:**
- API key validation format
- Encryption or secure storage
- API key expiration

#### `TestAPIKeyManagement/api_key_change_notification`
**Tests:** Change notification system
**Validates:**
- Setting API key triggers configuration change notification
- Waiters are notified of API key changes
- Notification timing is immediate
**Does NOT test:**
- API key validation against server
- Network calls triggered by API key changes
- Multiple API key formats

**BEHAVIOR:** Setting an API key only notifies waiters and saves to disk. The manager will reload when:
- Timer fires (every 5 minutes)
- fsnotify detects state.json file change (if file watching works)
- fsnotify errors occur
- Context is cancelled

**BEHAVIORAL GAP IDENTIFIED:** The manager itself is NOT a waiter for config changes, so `SetAPIKey()` does not immediately trigger the manager's reload loop. This means there could be a delay before the new API key is used to fetch configuration from the server.

#### `TestAPIKeyManagement/concurrent_api_key_access`
**Tests:** Thread safety of API key operations
**Validates:**
- Multiple goroutines can safely read API key
- Setting API key is thread-safe
- No race conditions in API key access
**Does NOT test:**
- Process-level concurrency
- API key validation during concurrent access

#### `TestAPIKeyManagement/api_key_persistence`
**Tests:** API key disk persistence
**Validates:**
- API key survives application restart
- Changes are immediately written to disk
**Does NOT test:**
- Disk encryption
- File backup/recovery
- Cross-platform file compatibility

### `TestIPConfiguration`

#### `TestIPConfiguration/ipv4_ipv6_handling`
**Tests:** IP address configuration management
**Validates:**
- IPv4 and IPv6 addresses can be configured separately
- IP address parsing works correctly
- Status fields are handled properly
**Does NOT test:**
- IP address validation against network interfaces
- Dynamic IP address changes
- DNS resolution

#### `TestIPConfiguration/ip_status_changes`
**Tests:** Monitor status transitions
**Validates:**
- Status changes (active, testing, paused) are detected
- IsLive() method works correctly for different statuses
**Does NOT test:**
- Server-side status validation
- Status change business logic
- Historical status tracking

#### `TestIPConfiguration/concurrent_ip_access`
**Tests:** Thread safety of IP configuration
**Validates:**
- Multiple readers can access IP configuration safely
- IP configuration updates are atomic
**Does NOT test:**
- Network interface binding
- IP address conflicts
- Multi-homed system behavior

**BEHAVIOR:** `IsLive()` should only consider the status field. No additional factors like network connectivity or test results should be considered.

### `TestConfigurationChangeNotifications`

#### `TestConfigurationChangeNotifications/waiter_lifecycle`
**Tests:** Configuration change waiter management
**Validates:**
- Waiters can be created and canceled properly
- Context cancellation works correctly
- No memory leaks in waiter management
**Does NOT test:**
- Waiter performance under high load
- Waiter behavior across process boundaries

#### `TestConfigurationChangeNotifications/multiple_waiters`
**Tests:** Multiple simultaneous waiters
**Validates:**
- Multiple waiters can exist simultaneously
- All waiters are notified of changes
- Waiter isolation (one waiter's cancellation doesn't affect others)
**Does NOT test:**
- Maximum number of waiters
- Waiter priority or ordering
- Cross-process notifications

#### `TestConfigurationChangeNotifications/waiter_cleanup`
**Tests:** Memory leak prevention
**Validates:**
- Canceled waiters are removed from internal maps
- No goroutine leaks occur
- Resource cleanup is automatic
**Does NOT test:**
- Long-term memory usage patterns
- Cleanup under error conditions
- Performance impact of many canceled waiters

#### `TestConfigurationChangeNotifications/notification_timing`
**Tests:** Event notification timing and reliability
**Validates:**
- Notifications are delivered promptly
- No notifications are lost
- Event ordering is preserved
**Does NOT test:**
- Notification delivery under system load
- Cross-thread notification timing
- Notification reliability during shutdown

**BEHAVIOR:** Notification ordering does not matter. Any order is acceptable as long as all waiters are notified.

#### `TestConfigurationChangeNotifications/edge_cases`
**Tests:** Unusual notification scenarios
**Validates:**
- Notifications work when no waiters exist
- Rapid successive changes are handled correctly
- Context cancellation during notification works
**Does NOT test:**
- Notification behavior during system shutdown
- Error recovery in notification system

---

## fsnotify & Hot Reloading Tests (`appconfig_manager_test.go`)

### `TestFileWatcherSetup`

#### `TestFileWatcherSetup/successful_watcher_creation`
**Tests:** Normal file watcher initialization
**Validates:**
- fsnotify.NewWatcher() succeeds
- Directory watching can be established
- Watcher channels are properly configured
**Does NOT test:**
- File system permission edge cases
- Network file system compatibility
- Watcher behavior on read-only file systems

#### `TestFileWatcherSetup/invalid_directory_handling`
**Tests:** Error handling in watcher setup
**Validates:**
- Nonexistent directories are handled gracefully
- Fallback to timer-only mode works
- Error logging is appropriate
**Does NOT test:**
- Recovery when directory becomes available later
- Permissions changing after setup
- Directory deletion during watching

#### `TestFileWatcherSetup/watcher_cleanup`
**Tests:** Resource cleanup on manager shutdown
**Validates:**
- File watchers are properly closed
- No resource leaks occur
- Cleanup works even if watcher creation failed
**Does NOT test:**
- Cleanup during system shutdown
- Forced termination scenarios
- Recovery from cleanup failures

### `TestDebounceTimer`

#### `TestDebounceTimer/rapid_file_changes`
**Tests:** Debounce logic for file system events
**Validates:**
- Multiple rapid file changes result in single reload
- Debounce delay works correctly (100ms)
- Previous debounce timers are canceled properly
**Does NOT test:**
- Very high frequency changes (>100/second)
- System clock changes affecting timer
- Debounce behavior under heavy system load

**BEHAVIOR:** The debounce timer should absorb all events and reload after 500ms of quiet time. Events arriving faster than the debounce interval should keep resetting the timer.

**IMPLEMENTATION GAP:** Current implementation uses 100ms debounce. Should be changed to 500ms.

#### `TestDebounceTimer/debounce_timing_accuracy`
**Tests:** Timing precision of debounce mechanism
**Validates:**
- Debounce timer fires at approximately correct time
- No premature timer firing occurs
- Timer accuracy within reasonable bounds
**Does NOT test:**
- Sub-millisecond timing accuracy
- Timer behavior during system sleep/wake
- High precision timing requirements

#### `TestDebounceTimer/concurrent_debounce_events`
**Tests:** Multiple file events during debounce period
**Validates:**
- Overlapping debounce timers are handled correctly
- Only one reload occurs after debounce period
- Event ordering doesn't affect debounce behavior
**Does NOT test:**
- Events from multiple monitored directories
- Cross-process file events
- Event coalescing by the OS

### `TestFileEventHandling`

#### `TestFileEventHandling/write_events`
**Tests:** Standard file modification detection
**Validates:**
- fsnotify.Write events trigger reloads
- Write events are properly filtered for state.json
- Other files' write events are ignored
**Does NOT test:**
- Binary vs text file distinctions
- Large file write performance
- Partial write detection

#### `TestFileEventHandling/create_and_rename_events`
**Tests:** Atomic operation detection
**Validates:**
- fsnotify.Create events for atomic operations are detected
- fsnotify.Rename events trigger reloads
- Temporary file events (.tmp files) are recognized
**Does NOT test:**
- Platform-specific atomic operation patterns
- Cross-file-system rename operations
- Symbolic link handling

#### `TestFileEventHandling/irrelevant_file_filtering`
**Tests:** Event filtering for unrelated files
**Validates:**
- Events for files other than state.json are ignored
- Directory events don't trigger reloads
- Hidden file events are properly filtered
**Does NOT test:**
- Regular expression-based filtering
- Case sensitivity handling
- Unicode filename handling

#### `TestFileEventHandling/event_error_recovery`
**Tests:** Error handling in event processing
**Validates:**
- fsnotify errors don't crash the manager
- Event channel closure is handled gracefully
- Manager continues operating after errors
**Does NOT test:**
- Recovery from file system corruption
- Handling of permission denied errors
- Network file system specific errors

### `TestWatcherErrorRecovery`

#### `TestWatcherErrorRecovery/error_channel_handling`
**Tests:** fsnotify error channel processing
**Validates:**
- Errors from fsnotify are logged appropriately
- Error channel closure doesn't stop manager
- Multiple errors are handled correctly
**Does NOT test:**
- Automatic watcher recreation after errors
- Error categorization and specific handling
- Error rate limiting

#### `TestWatcherErrorRecovery/fallback_to_timer_mode`
**Tests:** Timer-only operation mode
**Validates:**
- Manager continues without file watcher
- Timer-based reloading works correctly
- No degradation in core functionality
**Does NOT test:**
- Performance comparison between modes
- Memory usage differences
- Timing accuracy in timer mode

#### `TestWatcherErrorRecovery/recovery_lifecycle`
**Tests:** Manager restart and recovery
**Validates:**
- Manager can be stopped and restarted
- State is properly reset on restart
- No resource leaks during restart cycles
**Does NOT test:**
- Hot swapping of manager instances
- State preservation across restarts
- Configuration migration during restarts

### `TestManagerLifecycle`

#### `TestManagerLifecycle/normal_startup_shutdown`
**Tests:** Standard manager lifecycle
**Validates:**
- Manager starts successfully with valid configuration
- Shutdown is clean and complete
- No hanging goroutines after shutdown
**Does NOT test:**
- Startup time performance
- Shutdown timeout behavior
- Resource usage during operation

#### `TestManagerLifecycle/multiple_restart_cycles`
**Tests:** Repeated start/stop operations
**Validates:**
- Manager can be restarted multiple times
- No resource accumulation over restarts
- State is properly reset each time
**Does NOT test:**
- Very rapid restart cycles
- Restart under load
- Cross-restart state consistency

#### `TestManagerLifecycle/context_cancellation_handling`
**Tests:** Graceful context-based shutdown
**Validates:**
- Context cancellation stops manager promptly
- All goroutines respect context cancellation
- Cleanup occurs even during cancellation
**Does NOT test:**
- Forced termination scenarios
- Context cancellation during file operations
- Partial cancellation states

**BEHAVIOR:** Manager should abort immediately when context is canceled. Simplicity is preferred over completing in-progress operations.

#### `TestManagerLifecycle/prometheus_metrics_registration`
**Tests:** Metrics system integration
**Validates:**
- Prometheus metrics are registered correctly
- Certificate expiration metric works
- No duplicate registration occurs
**Does NOT test:**
- Metric accuracy over time
- Metric performance impact
- Custom metric registry behavior

---

## Certificate Management Tests (`appconfig_certs_test.go`)

### `TestCertificateLoadSave`

#### `TestCertificateLoadSave/valid_certificate_persistence`
**Tests:** Basic certificate file operations
**Validates:**
- Certificates can be saved to disk correctly
- Certificate files are created in correct locations
- Certificates can be loaded from disk successfully
- HaveCertificate() returns correct status
**Does NOT test:**
- Certificate validation against CA
- Certificate format conversion
- Multiple certificate formats (only PEM)

#### `TestCertificateLoadSave/malformed_certificate_handling`
**Tests:** Invalid certificate data handling
**Validates:**
- Invalid certificate data is rejected
- System remains stable with bad certificates
- Error handling is appropriate
**Does NOT test:**
- Certificate format auto-detection
- Partial certificate recovery
- Certificate repair attempts

#### `TestCertificateLoadSave/missing_file_scenarios`
**Tests:** File not found error handling
**Validates:**
- Missing certificate files return appropriate errors
- os.IsNotExist() error type is returned
- System continues without certificates
**Does NOT test:**
- Automatic certificate download
- Certificate file recreation
- Fallback certificate sources

#### `TestCertificateLoadSave/concurrent_certificate_updates`
**Tests:** Thread safety of certificate operations
**Validates:**
- Multiple goroutines can save certificates safely
- Certificate loading is thread-safe
- No data corruption during concurrent access
**Does NOT test:**
- Process-level concurrency
- File locking mechanisms
- Certificate rotation under load

#### `TestCertificateLoadSave/partial_file_corruption_recovery`
**Tests:** Resilience to file system issues
**Validates:**
- Corrupted certificate files are detected
- System handles partial file corruption gracefully
- Error messages are informative
**Does NOT test:**
- Automatic file repair
- Backup certificate management
- Corruption prevention

### `TestCertificateValidity`

#### `TestCertificateValidity/various_expiration_times`
**Tests:** Certificate expiration logic
**Validates:**
- Far future certificates are considered valid
- Near-expiration certificates trigger renewal
- Expired certificates are marked invalid
- Renewal threshold logic (1/3 of lifetime)
**Does NOT test:**
- Time zone handling
- Certificate revocation checking
- Custom validity periods

**BEHAVIOR:** The 1/3 lifetime renewal threshold should be a constant (not configurable) for now.

#### `TestCertificateValidity/renewal_timing_calculation`
**Tests:** Certificate renewal scheduling
**Validates:**
- Next check timing is calculated correctly
- Long-lived certificates have appropriate check intervals
- Certificate dates are extracted properly
**Does NOT test:**
- Network time synchronization issues
- Leap second handling
- Certificate pre-renewal buffering

#### `TestCertificateValidity/clock_skew_handling`
**Tests:** Time synchronization edge cases
**Validates:**
- Certificates with future notBefore are handled
- Small clock differences don't break validation
- System remains functional with time skew
**Does NOT test:**
- Large clock skew scenarios (>1 hour)
- NTP synchronization integration
- Time zone changes

#### `TestCertificateValidity/invalid_certificate_rejection`
**Tests:** Expired certificate detection
**Validates:**
- Already expired certificates are detected
- Validity status reflects actual expiration
- System behavior with expired certificates
**Does NOT test:**
- Grace periods for expired certificates
- User notification of expiration
- Automatic certificate renewal

### `TestCertificateRenewalFlow`

#### `TestCertificateRenewalFlow/automatic_renewal_triggering`
**Tests:** Renewal initiation logic
**Validates:**
- Certificates near expiration trigger renewal
- Renewal decision logic works correctly
- Next check timing is appropriate
**Does NOT test:**
- Actual API calls for renewal
- Network failure during renewal
- Certificate provisioning process

#### `TestCertificateRenewalFlow/certificate_replacement_atomicity`
**Tests:** Certificate update operations
**Validates:**
- New certificates replace old ones properly
- Certificate dates change after replacement
- Replacement is atomic (no partial states)
**Does NOT test:**
- Rollback on renewal failure
- Certificate chain validation
- Multiple certificate management

#### `TestCertificateRenewalFlow/notification_on_certificate_status_change`
**Tests:** Certificate change notifications
**Validates:**
- Certificate status changes trigger notifications
- Waiters are notified when certificates are added
- Notification timing is correct
**Does NOT test:**
- Notification ordering guarantees
- Cross-process notifications
- Notification delivery reliability

**BEHAVIOR:** Certificate replacement notifications do not need to distinguish between updates and additions. They should work the same way.

### `TestTLSConfiguration`

#### `TestTLSConfiguration/GetClientCertificate_callback`
**Tests:** TLS client certificate provider
**Validates:**
- GetClientCertificate returns loaded certificates
- Callback works with TLS configuration
- Certificate data is properly formatted
**Does NOT test:**
- Certificate selection logic for multiple certs
- Client authentication negotiation
- Certificate chain building

#### `TestTLSConfiguration/GetCertificate_for_server_mode`
**Tests:** TLS server certificate provider
**Validates:**
- GetCertificate returns certificates for server mode
- Server name indication (SNI) handling
- Certificate data structure is correct
**Does NOT test:**
- Certificate selection based on SNI
- Multiple domain certificates
- Certificate chain validation

#### `TestTLSConfiguration/certificate_chain_validation`
**Tests:** Certificate usage in TLS context
**Validates:**
- Certificates can be used in TLS configuration
- TLS config callbacks are properly set
- Certificate structure is TLS-compatible
**Does NOT test:**
- Actual TLS handshake validation
- Certificate trust chain verification
- TLS protocol version compatibility

#### `TestTLSConfiguration/no_certificate_available`
**Tests:** Missing certificate error handling
**Validates:**
- Appropriate errors when no certificate exists
- TLS callbacks handle missing certificates gracefully
- Error types are correct
**Does NOT test:**
- Fallback certificate mechanisms
- TLS handshake failure scenarios
- Client behavior with missing server certs

### `TestCertificateEdgeCases`

#### `TestCertificateEdgeCases/zero-length_certificate_files`
**Tests:** Empty file handling
**Validates:**
- Empty certificate files are rejected
- Zero-length files don't crash system
- Appropriate error messages for empty files
**Does NOT test:**
- Very small but valid certificates
- File truncation scenarios
- Streaming certificate loading

#### `TestCertificateEdgeCases/certificate_file_permissions`
**Tests:** File security requirements
**Validates:**
- Private key files have restrictive permissions (0600)
- Group and other permissions are removed
- Certificate files are created securely
**Does NOT test:**
- Permission inheritance from parent directories
- Cross-platform permission handling
- Permission changes after creation

#### `TestCertificateEdgeCases/certificate_dates_edge_cases`
**Tests:** Certificate date API error handling
**Validates:**
- CertificateDates() returns appropriate errors when no cert
- CheckCertificateValidity() handles missing certificates
- Error types are correct (ErrNoCertificate)
**Does NOT test:**
- Date parsing edge cases
- Certificate date validation
- Time zone handling in dates

#### `TestCertificateEdgeCases/HaveCertificate_state_management`
**Tests:** Certificate presence detection
**Validates:**
- HaveCertificate() returns false before loading
- HaveCertificate() returns true after successful loading
- State tracking is accurate
**Does NOT test:**
- HaveCertificate() performance
- State persistence across restarts
- Cached vs real-time status

---

## Cross-Cutting Test Concerns

### What ALL Tests Do NOT Cover

1. **Network Integration**
   - Actual API calls to monitor-api server
   - Network failure scenarios
   - Certificate validation against real CA
   - DNS resolution issues

2. **Production Environment Factors**
   - Different file systems (NFS, CIFS, etc.)
   - Container environments
   - Limited disk space scenarios
   - System-level permissions

3. **Performance at Scale**
   - Very large configuration files (>1GB)
   - High-frequency configuration changes (>1000/sec)
   - Long-running stability (>24 hours)
   - Memory usage patterns over time

4. **Cross-Platform Compatibility**
   - Windows vs Linux file behavior
   - macOS specific file system features
   - File path length limitations
   - Character encoding differences

5. **Security Scenarios**
   - Malicious configuration injection
   - File system security attacks
   - Certificate tampering
   - Privilege escalation

6. **Recovery and Resilience**
   - System crash during file operations
   - Disk full scenarios
   - Power failure during writes
   - Partial system failures

---

## Implementation Gaps Identified

Based on the behavioral clarifications, the following implementation gaps were identified:

1. **Manager Reload Trigger Gap:** The manager itself is not a waiter for config changes, so `SetAPIKey()` does not immediately trigger the manager's reload loop. This could cause delays (up to 5 minutes) before new API keys are used to fetch configuration.

2. **Debounce Timer Setting:** Current implementation uses 100ms debounce interval, but should be 500ms per requirements.

3. **Certificate Renewal Threshold:** Should verify the 1/3 lifetime threshold is implemented as a named constant rather than a magic number.

## Behavioral Specifications Confirmed

All behavioral questions have been resolved:
- API key setting only notifies waiters (manager gap identified above)
- Debounce timer should use 500ms and absorb all events during quiet period
- Manager should abort immediately on context cancellation
- Certificate renewal threshold should be a constant (1/3 lifetime)
- Notification ordering does not matter
- IsLive() only considers status field
- Certificate notifications treat updates and additions the same
