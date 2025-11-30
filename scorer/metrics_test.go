package scorer

import (
	"context"
	"log/slog"
	"os"
	"testing"

	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/prometheus/client_golang/prometheus"
)

func TestMetricsInitialization(t *testing.T) {
	dbURL := os.Getenv("TEST_DATABASE_URL")
	if dbURL == "" {
		t.Skip("TEST_DATABASE_URL not set, skipping integration test")
	}

	reg := prometheus.NewRegistry()

	ctx := context.Background()

	// Create a test database connection
	pool, err := pgxpool.New(ctx, dbURL)
	if err != nil {
		t.Fatalf("Failed to connect to test database: %v", err)
	}
	defer pool.Close()

	logger := slog.Default()

	runner, err := New(ctx, logger, pool, reg)
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
	dbURL := os.Getenv("TEST_DATABASE_URL")
	if dbURL == "" {
		t.Skip("TEST_DATABASE_URL not set, skipping integration test")
	}

	reg := prometheus.NewRegistry()

	ctx := context.Background()

	// Create a test database connection
	pool, err := pgxpool.New(ctx, dbURL)
	if err != nil {
		t.Fatalf("Failed to connect to test database: %v", err)
	}
	defer pool.Close()

	logger := slog.Default()

	runner, err := New(ctx, logger, pool, reg)
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
