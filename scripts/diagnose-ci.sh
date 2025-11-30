#!/bin/bash

# Diagnostic script to understand CI failures
# This script runs each CI step individually to identify where failures occur

set -e

SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
PROJECT_ROOT="$(cd "$SCRIPT_DIR/.." && pwd)"

cd "$PROJECT_ROOT"

echo "=== CI Diagnostic Script ==="
echo "This script will run each CI step individually to help diagnose failures"
echo ""

# Configuration
DB_NAME="monitor_test"
DB_USER="monitor"
DB_PASSWORD="test123"
DB_HOST="localhost"
DB_PORT="5432"

# Function to run a command and check its result
run_step() {
    local step_name="$1"
    local cmd="$2"

    echo "----------------------------------------"
    echo "STEP: $step_name"
    echo "CMD: $cmd"
    echo "----------------------------------------"

    if eval "$cmd"; then
        echo "✅ SUCCESS: $step_name"
    else
        echo "❌ FAILED: $step_name (exit code: $?)"
        return 1
    fi
    echo ""
}

# Setup test database using native PostgreSQL
echo "Setting up test database..."
"$SCRIPT_DIR/test-db.sh" restart

# Export test database URL
export TEST_DATABASE_URL="postgres://${DB_USER}:${DB_PASSWORD}@${DB_HOST}:${DB_PORT}/${DB_NAME}?sslmode=disable"

# Run diagnostic steps
run_step "Test PostgreSQL connection" \
    "PGPASSWORD=$DB_PASSWORD psql -h $DB_HOST -U $DB_USER -d $DB_NAME -c 'SELECT 1'"

run_step "Verify tables exist" \
    "PGPASSWORD=$DB_PASSWORD psql -h $DB_HOST -U $DB_USER -d $DB_NAME -c '\dt'"

run_step "Check Go environment" \
    "go version"

run_step "Run simple Go test" \
    "go test -v -run TestGetServerName ./api"

run_step "Run short tests" \
    "go test -v -short ./api"

run_step "Build project" \
    "go build ./..."

echo "=== Diagnostic complete ==="
