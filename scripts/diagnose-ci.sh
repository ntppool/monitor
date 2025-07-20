#!/bin/bash

# Diagnostic script to understand CI failures
# This script runs each CI step individually to identify where failures occur

set -e

echo "=== CI Diagnostic Script ==="
echo "This script will run each CI step individually to help diagnose failures"
echo ""

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

# Check for existing database on port 3308
if lsof -i :3308 >/dev/null 2>&1; then
    echo "❌ ERROR: Port 3308 is already in use. Another database may be running."
    echo "You can stop the existing database with: make test-db-stop"
    exit 1
fi

# Clean up any existing containers
echo "Cleaning up existing containers..."
docker rm -f ci-diag-mysql 2>/dev/null || true

# Start MySQL
echo "Starting MySQL container..."
docker run -d \
    --name ci-diag-mysql \
    -e MYSQL_ROOT_PASSWORD=root \
    -e MYSQL_DATABASE=monitor_test \
    -e MYSQL_USER=monitor \
    -e MYSQL_PASSWORD=test123 \
    -p 3308:3306 \
    mysql:8.0

# Wait for MySQL to be ready
echo "Waiting for MySQL to start..."
sleep 15

# Run diagnostic steps
run_step "Test MySQL root connection" \
    "docker exec ci-diag-mysql mysql -u root -proot -e 'SELECT 1'"

run_step "Test MySQL monitor user connection" \
    "docker exec ci-diag-mysql mysql -u monitor -ptest123 -e 'SELECT 1' monitor_test"

run_step "Load schema into database" \
    "docker exec -i ci-diag-mysql mysql -u monitor -ptest123 monitor_test < schema.sql"

run_step "Verify tables exist" \
    "docker exec ci-diag-mysql mysql -u monitor -ptest123 -e 'SHOW TABLES' monitor_test"

# Now test from a golang container
echo "Testing from golang container (like CI)..."

run_step "Run golang container test" \
    "docker run --rm \
        --link ci-diag-mysql:database \
        -v $(pwd):/workspace \
        -w /workspace \
        golang:1.24 \
        bash -c '
            echo \"Installing mysql-client...\"
            apt-get update >/dev/null 2>&1 && apt-get install -y default-mysql-client >/dev/null 2>&1

            echo \"Testing connection to database...\"
            mysql -h database -u monitor -ptest123 -e \"SELECT 1\" monitor_test

            echo \"Checking Go environment...\"
            go version

            echo \"Running simple Go test...\"
            go test -v -run TestGetServerName ./api
        '"

# Test with exact CI environment variables
run_step "Test with CI environment" \
    "docker run --rm \
        --link ci-diag-mysql:database \
        -e TEST_DATABASE_URL='monitor:test123@tcp(database:3306)/monitor_test?parseTime=true&multiStatements=true' \
        -e GOCACHE=/cache/pkg/cache \
        -e GOMODCACHE=/cache/pkg/mod \
        -v $(pwd):/workspace \
        -w /workspace \
        golang:1.24 \
        bash -c '
            mkdir -p /cache/pkg/cache /cache/pkg/mod
            go test -v -short ./api
        '"

# Clean up
echo "Cleaning up..."
docker rm -f ci-diag-mysql

echo "=== Diagnostic complete ==="
