#!/bin/bash

# Minimal test to reproduce CI environment issues
# Run just the failing tests in isolation

set -e

SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
PROJECT_ROOT="$(cd "$SCRIPT_DIR/.." && pwd)"

cd "$PROJECT_ROOT"

echo "=== Starting minimal CI test ==="

# Configuration
DB_NAME="monitor_test"
DB_USER="monitor"
DB_PASSWORD="test123"
DB_HOST="localhost"
DB_PORT="5432"

# Setup test database using native PostgreSQL
echo "Setting up test database..."
"$SCRIPT_DIR/test-db.sh" restart

# Export test database URL
export TEST_DATABASE_URL="postgres://${DB_USER}:${DB_PASSWORD}@${DB_HOST}:${DB_PORT}/${DB_NAME}?sslmode=disable"

# Test connection directly
echo "Testing database connection..."
PGPASSWORD="$DB_PASSWORD" psql -h "$DB_HOST" -U "$DB_USER" -d "$DB_NAME" -c "SELECT 1"

# Show tables
echo "Showing tables..."
PGPASSWORD="$DB_PASSWORD" psql -h "$DB_HOST" -U "$DB_USER" -d "$DB_NAME" -c '\dt'

# Run only the integration tests
echo "Running integration tests..."
go test -tags=integration -v ./scorer/... ./selector/... -run Integration

echo "=== Test complete ==="
