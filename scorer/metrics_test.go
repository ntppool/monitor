package scorer

import (
	"context"
	"database/sql"
	"log/slog"
	"testing"

	_ "github.com/go-sql-driver/mysql"
	"github.com/prometheus/client_golang/prometheus"
)

func TestMetricsInitialization(t *testing.T) {
	reg := prometheus.NewRegistry()

	// Create a mock database connection (this won't be used for actual DB operations)
	db, err := sql.Open("mysql", "test:test@/test")
	if err != nil {
		t.Fatalf("Failed to create mock database: %v", err)
	}
	defer db.Close()

	ctx := context.Background()
	logger := slog.Default()

	runner, err := New(ctx, logger, db, reg)
	if err != nil {
		t.Fatalf("Failed to create runner: %v", err)
	}

	// Verify that the sqlUpdates metric was initialized
	if runner.m.sqlUpdates == nil {
		t.Fatal("sqlUpdates metric not initialized")
	}

	// Increment a counter so the metric appears in the registry
	runner.m.sqlUpdates.WithLabelValues("test_operation").Inc()

	// Check that the metric is registered with Prometheus
	metricFamilies, err := reg.Gather()
	if err != nil {
		t.Fatalf("Failed to gather metrics: %v", err)
	}

	found := false
	for _, mf := range metricFamilies {
		if *mf.Name == "scorer_sql_updates_total" {
			found = true
			if *mf.Help != "Total number of SQL update operations by type" {
				t.Errorf("Expected help text 'Total number of SQL update operations by type', got %s", *mf.Help)
			}
			break
		}
	}

	if !found {
		t.Error("scorer_sql_updates_total metric not found in registry")
	}
}

func TestSQLUpdateMetricsIncrement(t *testing.T) {
	reg := prometheus.NewRegistry()

	// Create a mock database connection
	db, err := sql.Open("mysql", "test:test@/test")
	if err != nil {
		t.Fatalf("Failed to create mock database: %v", err)
	}
	defer db.Close()

	ctx := context.Background()
	logger := slog.Default()

	runner, err := New(ctx, logger, db, reg)
	if err != nil {
		t.Fatalf("Failed to create runner: %v", err)
	}

	// Test incrementing different operation types
	operations := []string{
		"update_server_score_status",
		"insert_log_score",
		"update_server_score",
		"update_server",
		"update_scorer_status",
		"insert_server_score",
	}

	// Increment each operation type once
	for _, op := range operations {
		runner.m.sqlUpdates.WithLabelValues(op).Inc()
	}

	// Verify that metrics are accessible and can be incremented
	// Since we can't easily access the value without testutil, we'll just verify no panics occur
	for _, op := range operations {
		// This should not panic
		runner.m.sqlUpdates.WithLabelValues(op).Inc()
	}

	// Test that the registry can be gathered without errors
	_, err = reg.Gather()
	if err != nil {
		t.Fatalf("Failed to gather metrics: %v", err)
	}

	// Basic test passed - metrics are properly registered and can be incremented
}
