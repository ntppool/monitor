package testutil

import (
	"context"
	"database/sql"
	"fmt"
	"log/slog"
	"os"
	"testing"
	"time"

	_ "github.com/go-sql-driver/mysql"
	"go.ntppool.org/monitor/ntpdb"
)

// TestDB represents a test database connection with utilities
type TestDB struct {
	*sql.DB
	queries *ntpdb.Queries
	ctx     context.Context
}

// NewTestDB creates a new test database connection
func NewTestDB(t *testing.T) *TestDB {
	dbURL := os.Getenv("TEST_DATABASE_URL")
	if dbURL == "" {
		t.Skip("TEST_DATABASE_URL not set, skipping integration test")
	}

	db, err := sql.Open("mysql", dbURL)
	if err != nil {
		t.Fatalf("Failed to connect to test database: %v", err)
	}

	// Test connection
	if err := db.Ping(); err != nil {
		t.Fatalf("Failed to ping test database: %v", err)
	}

	return &TestDB{
		DB:      db,
		queries: ntpdb.New(db),
		ctx:     context.Background(),
	}
}

// Close closes the database connection
func (tdb *TestDB) Close() {
	tdb.DB.Close()
}

// Queries returns the ntpdb queries instance
func (tdb *TestDB) Queries() *ntpdb.Queries {
	return tdb.queries
}

// Context returns the test context
func (tdb *TestDB) Context() context.Context {
	return tdb.ctx
}

// CleanupTestData removes all test data from the database
func (tdb *TestDB) CleanupTestData(t *testing.T) {
	// Disable foreign key checks temporarily
	_, err := tdb.ExecContext(tdb.ctx, "SET FOREIGN_KEY_CHECKS = 0")
	if err != nil {
		t.Logf("Error disabling foreign key checks: %v", err)
	}

	// Clean up in reverse dependency order
	tables := []string{
		"server_scores",
		"log_scores",
		"servers_monitor_review",
		"servers",
		"monitors",
		"accounts",
		"system_settings",
	}

	for _, table := range tables {
		// Only clean test data (using ID ranges)
		query := fmt.Sprintf("DELETE FROM %s WHERE id >= 1000 AND id <= 9999", table)
		_, err := tdb.ExecContext(tdb.ctx, query)
		if err != nil {
			t.Logf("Error cleaning up table %s: %v", table, err)
		}
	}

	// Clean up monitors by account_id since they might not have sequential IDs
	_, err = tdb.ExecContext(tdb.ctx, "DELETE FROM monitors WHERE account_id >= 1000 AND account_id <= 9999")
	if err != nil {
		t.Logf("Error cleaning up monitors: %v", err)
	}

	// Clean up server_scores that reference test monitors
	_, err = tdb.ExecContext(tdb.ctx, "DELETE FROM server_scores WHERE monitor_id >= 2000 AND monitor_id <= 2999")
	if err != nil {
		t.Logf("Error cleaning up server_scores: %v", err)
	}

	// Clean up test system settings
	_, err = tdb.ExecContext(tdb.ctx, "DELETE FROM system_settings WHERE setting_key LIKE 'test_%'")
	if err != nil {
		t.Logf("Error cleaning up system_settings: %v", err)
	}

	// Re-enable foreign key checks
	_, err = tdb.ExecContext(tdb.ctx, "SET FOREIGN_KEY_CHECKS = 1")
	if err != nil {
		t.Logf("Error re-enabling foreign key checks: %v", err)
	}
}

// TimeController allows controlling time in tests
type TimeController struct {
	frozen  bool
	current time.Time
	offset  time.Duration
}

// NewTimeController creates a new time controller
func NewTimeController() *TimeController {
	return &TimeController{
		frozen:  false,
		current: time.Now(),
		offset:  0,
	}
}

// Freeze freezes time at the current moment
func (tc *TimeController) Freeze() {
	tc.frozen = true
	tc.current = time.Now().Add(tc.offset)
}

// Unfreeze unfreezes time
func (tc *TimeController) Unfreeze() {
	tc.frozen = false
	tc.offset = 0
}

// SetTime sets the current time
func (tc *TimeController) SetTime(t time.Time) {
	tc.current = t
	tc.offset = t.Sub(time.Now())
	tc.frozen = true
}

// Advance advances time by the given duration
func (tc *TimeController) Advance(d time.Duration) {
	if tc.frozen {
		tc.current = tc.current.Add(d)
	} else {
		tc.offset += d
	}
}

// Now returns the current controlled time
func (tc *TimeController) Now() time.Time {
	if tc.frozen {
		return tc.current
	}
	return time.Now().Add(tc.offset)
}

// DataFactory helps generate realistic test data
type DataFactory struct {
	tdb *TestDB
}

// NewDataFactory creates a new data factory
func NewDataFactory(tdb *TestDB) *DataFactory {
	return &DataFactory{tdb: tdb}
}

// CreateTestAccount creates a test account
func (df *DataFactory) CreateTestAccount(t *testing.T, id uint32, email string) {
	query := `
		INSERT INTO accounts (id, email, created_on)
		VALUES (?, ?, NOW())
		ON DUPLICATE KEY UPDATE email = VALUES(email)
	`
	_, err := df.tdb.ExecContext(df.tdb.ctx, query, id, email)
	if err != nil {
		t.Fatalf("Failed to create test account: %v", err)
	}
}

// CreateTestMonitor creates a test monitor
func (df *DataFactory) CreateTestMonitor(t *testing.T, id uint32, tlsName string, accountID uint32, ip string, status string) {
	query := `
		INSERT INTO monitors (id, tls_name, account_id, ip, status, type, created_on)
		VALUES (?, ?, ?, ?, ?, 'monitor', NOW())
		ON DUPLICATE KEY UPDATE
			tls_name = VALUES(tls_name),
			account_id = VALUES(account_id),
			ip = VALUES(ip),
			status = VALUES(status)
	`
	_, err := df.tdb.ExecContext(df.tdb.ctx, query, id, tlsName, accountID, ip, status)
	if err != nil {
		t.Fatalf("Failed to create test monitor: %v", err)
	}
}

// CreateTestServer creates a test server
func (df *DataFactory) CreateTestServer(t *testing.T, id uint32, ip string, ipVersion string, accountID *uint32) {
	query := `
		INSERT INTO servers (id, ip, ip_version, account_id, created_on)
		VALUES (?, ?, ?, ?, NOW())
		ON DUPLICATE KEY UPDATE
			ip = VALUES(ip),
			ip_version = VALUES(ip_version),
			account_id = VALUES(account_id)
	`
	_, err := df.tdb.ExecContext(df.tdb.ctx, query, id, ip, ipVersion, accountID)
	if err != nil {
		t.Fatalf("Failed to create test server: %v", err)
	}
}

// CreateTestServerScore creates a test server score
func (df *DataFactory) CreateTestServerScore(t *testing.T, serverID, monitorID uint32, status string, scoreRaw float64) {
	query := `
		INSERT INTO server_scores (server_id, monitor_id, status, score_raw, created_on)
		VALUES (?, ?, ?, ?, NOW())
		ON DUPLICATE KEY UPDATE
			status = VALUES(status),
			score_raw = VALUES(score_raw)
	`
	_, err := df.tdb.ExecContext(df.tdb.ctx, query, serverID, monitorID, status, scoreRaw)
	if err != nil {
		t.Fatalf("Failed to create test server score: %v", err)
	}
}

// CreateTestLogScore creates a test log score
func (df *DataFactory) CreateTestLogScore(t *testing.T, serverID, monitorID uint32, score, step float64, rtt *int32, ts time.Time) {
	query := `
		INSERT INTO log_scores (server_id, monitor_id, ts, score, step, rtt, created_on)
		VALUES (?, ?, ?, ?, ?, ?, NOW())
	`
	_, err := df.tdb.ExecContext(df.tdb.ctx, query, serverID, monitorID, ts, score, step, rtt)
	if err != nil {
		t.Fatalf("Failed to create test log score: %v", err)
	}
}

// SetSystemSetting sets a system setting for tests
func (df *DataFactory) SetSystemSetting(t *testing.T, key, value string) {
	query := `
		INSERT INTO system_settings (setting_key, setting_value)
		VALUES (?, ?)
		ON DUPLICATE KEY UPDATE setting_value = VALUES(setting_value)
	`
	_, err := df.tdb.ExecContext(df.tdb.ctx, query, key, value)
	if err != nil {
		t.Fatalf("Failed to set system setting: %v", err)
	}
}

// TestLogger creates a test logger that outputs to testing.T
type TestLogger struct {
	t      *testing.T
	logger *slog.Logger
}

// NewTestLogger creates a test logger
func NewTestLogger(t *testing.T) *TestLogger {
	handler := slog.NewTextHandler(os.Stdout, &slog.HandlerOptions{
		Level: slog.LevelDebug,
	})
	logger := slog.New(handler)

	return &TestLogger{
		t:      t,
		logger: logger,
	}
}

// Logger returns the slog.Logger instance
func (tl *TestLogger) Logger() *slog.Logger {
	return tl.logger
}

// AssertNoError is a helper to assert no error occurred
func AssertNoError(t *testing.T, err error, msg string, args ...interface{}) {
	if err != nil {
		t.Fatalf(msg+": %v", append(args, err)...)
	}
}

// AssertError is a helper to assert an error occurred
func AssertError(t *testing.T, err error, msg string, args ...interface{}) {
	if err == nil {
		t.Fatalf(msg, args...)
	}
}

// AssertEqual is a helper to assert two values are equal
func AssertEqual[T comparable](t *testing.T, expected, actual T, msg string, args ...interface{}) {
	if expected != actual {
		t.Fatalf(msg+" - expected: %v, actual: %v", append(args, expected, actual)...)
	}
}

// AssertNotEqual is a helper to assert two values are not equal
func AssertNotEqual[T comparable](t *testing.T, expected, actual T, msg string, args ...interface{}) {
	if expected == actual {
		t.Fatalf(msg+" - both values equal: %v", append(args, expected)...)
	}
}

// AssertTrue is a helper to assert a boolean is true
func AssertTrue(t *testing.T, condition bool, msg string, args ...interface{}) {
	if !condition {
		t.Fatalf(msg, args...)
	}
}

// AssertFalse is a helper to assert a boolean is false
func AssertFalse(t *testing.T, condition bool, msg string, args ...interface{}) {
	if condition {
		t.Fatalf(msg, args...)
	}
}
