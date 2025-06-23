//go:build integration
// +build integration

package selector

import (
	"context"
	"database/sql"
	"testing"
	"time"

	"go.ntppool.org/monitor/ntpdb"
)

// Test helpers for integration tests

func createTestServer(t *testing.T, db *sql.DB, serverID uint32, ip, ipVersion string, accountID *uint32) {
	query := `INSERT INTO servers (id, ip, ip_version, account_id) VALUES (?, ?, ?, ?)`

	var accID sql.NullInt32
	if accountID != nil {
		accID = sql.NullInt32{Int32: int32(*accountID), Valid: true}
	}

	_, err := db.Exec(query, serverID, ip, ipVersion, accID)
	if err != nil {
		t.Fatalf("Failed to create test server: %v", err)
	}
}

func createTestMonitor(t *testing.T, db *sql.DB, monitorID uint32, status string, ip string, accountID *uint32) {
	query := `INSERT INTO monitors (id, status, ip, account_id) VALUES (?, ?, ?, ?)`

	var accID sql.NullInt32
	if accountID != nil {
		accID = sql.NullInt32{Int32: int32(*accountID), Valid: true}
	}

	_, err := db.Exec(query, monitorID, status, ip, accID)
	if err != nil {
		t.Fatalf("Failed to create test monitor: %v", err)
	}
}

func createTestServerScore(t *testing.T, db *sql.DB, serverID, monitorID uint32, status string, score float64) {
	query := `INSERT INTO server_scores (server_id, monitor_id, status, score_raw, created_on) VALUES (?, ?, ?, ?, NOW())`
	_, err := db.Exec(query, serverID, monitorID, status, score)
	if err != nil {
		t.Fatalf("Failed to create test server score: %v", err)
	}
}

func createTestLogScore(t *testing.T, db *sql.DB, serverID, monitorID uint32, score, offset float64, rtt *int32, ts time.Time) {
	query := `INSERT INTO log_scores (server_id, monitor_id, ts, score, offset, rtt) VALUES (?, ?, ?, ?, ?, ?)`

	var rttVal sql.NullInt32
	if rtt != nil {
		rttVal = sql.NullInt32{Int32: *rtt, Valid: true}
	}

	_, err := db.Exec(query, serverID, monitorID, ts, score, offset, rttVal)
	if err != nil {
		t.Fatalf("Failed to create test log score: %v", err)
	}
}

func int32Ptr(v int32) *int32 {
	return &v
}

func uint32Ptr(v uint32) *uint32 {
	return &v
}

func TestSelectorIntegration_CompleteFlow(t *testing.T) {
	t.Skip("Integration tests require database setup")

	// This test would require a test database setup
	// The actual test implementation would go here
}

func TestSelectorIntegration_ConstraintViolations(t *testing.T) {
	t.Skip("Integration tests require database setup")

	// Test constraint violation detection and tracking
}

func TestSelectorIntegration_GrandfatheredConstraints(t *testing.T) {
	t.Skip("Integration tests require database setup")

	// Test grandfathering logic for existing violations
}

func TestSelectorIntegration_LoadBalancing(t *testing.T) {
	t.Skip("Integration tests require database setup")

	// Test that monitors are distributed evenly across servers
}
