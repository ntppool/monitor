# Monitor Testing Infrastructure

This directory contains scripts and utilities for testing the NTP Monitor system.

## Test Database Setup

### Quick Start

```bash
# Start test database
./scripts/test-db.sh start

# Run unit tests
make test-unit

# Run integration tests
make test-integration

# Run all tests
make test-all

# Stop test database
./scripts/test-db.sh stop
```

### Test Database Script

The `test-db.sh` script manages a MySQL database in Docker for integration testing:

- **Database**: `monitor_test` on port 3308
- **User**: `monitor` / `test123`
- **Container**: `ntpmonitor-test-db`

#### Commands

```bash
./scripts/test-db.sh start      # Start database with schema
./scripts/test-db.sh stop       # Stop and remove database
./scripts/test-db.sh restart    # Restart database
./scripts/test-db.sh status     # Show database status
./scripts/test-db.sh shell      # Open MySQL shell
./scripts/test-db.sh reset      # Reset with fresh data
```

## Test Categories

### Unit Tests (`go test ./... -short`)
- Fast tests with no external dependencies
- Mocks for database and external services
- Test individual functions and components
- Run in < 5 seconds

### Integration Tests (`go test ./... -tags=integration`)
- Real database interactions
- End-to-end component testing
- Full selector algorithm testing
- Require `TEST_DATABASE_URL` environment variable

### Load Tests (`go test ./... -tags=load`)
- Performance and scalability testing
- High-volume data processing
- Concurrent operation testing
- Memory usage validation
- Run with `-timeout=30m`

## Test Data Management

### Test ID Ranges
- **Accounts**: 1000-9999
- **Monitors**: 2000-2999
- **Servers**: 3000-9999
- **Test-specific ranges**: 10000+

### Data Factory Usage

```go
factory := testutil.NewDataFactory(tdb)

// Create test entities
factory.CreateTestAccount(t, 1001, "test@example.com")
factory.CreateTestMonitor(t, 2001, "mon.test", 1001, "10.0.0.1", "active")
factory.CreateTestServer(t, 3001, "192.0.2.1", "v4", nil)

// Create relationships
factory.CreateTestServerScore(t, 3001, 2001, "active", 20.0)
factory.CreateTestLogScore(t, 3001, 2001, 20.0, 0.9, &rtt, time.Now())
```

### Cleanup

Test cleanup is automatic via `defer tdb.CleanupTestData(t)` which:
- Removes all data in test ID ranges
- Preserves production data
- Disables foreign key checks during cleanup
- Re-enables foreign key checks after cleanup

## Continuous Integration

### Drone CI Pipeline

The `.drone.yml` includes:
- MySQL service container
- Database schema loading
- Unit test execution
- Integration test execution
- Build verification

### Local Development

```bash
# Setup
make test-db-start

# Development cycle
make test-unit          # Fast feedback
make test-integration   # Full testing
make coverage          # Coverage report

# Cleanup
make test-db-stop
```

## Test Utilities

### TestDB
- Database connection management
- Query execution helpers
- Automatic cleanup

### TimeController
- Control time in tests
- Freeze/advance time
- Test time-based behaviors

### DataFactory
- Generate realistic test data
- Consistent test setups
- Parameterized data creation

### Assertion Helpers
- `AssertNoError(t, err, msg, ...)`
- `AssertEqual(t, expected, actual, msg, ...)`
- `AssertTrue(t, condition, msg, ...)`

## Performance Testing

### Benchmarks

```bash
# Run benchmarks
make benchmark

# Profile memory
go test -bench=. -memprofile=mem.prof ./scorer/cmd
go tool pprof mem.prof

# Profile CPU
go test -bench=. -cpuprofile=cpu.prof ./scorer/cmd
go tool pprof cpu.prof
```

### Load Testing

```bash
# Full load tests
make test-load

# Specific load test
go test ./scorer/cmd -tags=load -v -run=TestSelector_LoadTest
```

## Troubleshooting

### Database Connection Issues

```bash
# Check database status
./scripts/test-db.sh status

# View database logs
./scripts/test-db.sh logs

# Reset database
./scripts/test-db.sh reset
```

### Test Failures

```bash
# Run specific test
go test ./scorer/cmd -v -run=TestSpecificTest

# Run with database debugging
TEST_DATABASE_URL="..." go test ./scorer/cmd -tags=integration -v

# Check test data
./scripts/test-db.sh shell
mysql> SELECT * FROM monitors WHERE id >= 2000;
```

### Performance Issues

```bash
# Run tests with timeout
go test ./scorer/cmd -timeout=10m -v

# Monitor memory usage
go test ./scorer/cmd -tags=load -v -run=MemoryUsage
```

## Adding New Tests

### Integration Test Template

```go
//go:build integration
// +build integration

func TestNewFeature_Integration(t *testing.T) {
    tdb := testutil.NewTestDB(t)
    defer tdb.Close()
    defer tdb.CleanupTestData(t)

    factory := testutil.NewDataFactory(tdb)

    // Setup test data
    factory.CreateTestAccount(t, 1001, "test@example.com")

    // Test scenarios
    t.Run("Scenario1", func(t *testing.T) {
        // Test implementation
    })
}
```

### Load Test Template

```go
//go:build load
// +build load

func TestNewFeature_Load(t *testing.T) {
    if testing.Short() {
        t.Skip("Skipping load test in short mode")
    }

    // Large dataset setup
    // Performance measurement
    // Assertions on performance metrics
}
```

## Environment Variables

- `TEST_DATABASE_URL`: Database connection string for integration tests
- `GOMAXPROCS`: Control CPU usage in tests
- `GOGC`: Control garbage collection in performance tests

## Docker Dependencies

- **MySQL 8.0**: Test database
- **Docker**: Container runtime
- **Docker Compose**: Multi-service testing (future)

## Future Enhancements

- Chaos engineering tests
- Multi-database testing
- Performance regression detection
- Test result reporting
- Parallel test execution
- Test data generation tools
