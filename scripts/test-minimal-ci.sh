#!/bin/bash

# Minimal test to reproduce CI environment issues
# Run just the failing tests in isolation

set -e

echo "=== Starting minimal CI test ==="

# Cleanup any existing containers
docker rm -f monitor-ci-test-db 2>/dev/null || true

# Start MySQL
echo "Starting MySQL..."
docker run -d \
    --name monitor-ci-test-db \
    -e MYSQL_ROOT_PASSWORD=root \
    -e MYSQL_DATABASE=monitor_test \
    -e MYSQL_USER=monitor \
    -e MYSQL_PASSWORD=test123 \
    mysql:8.0

# Wait for MySQL
echo "Waiting for MySQL..."
sleep 10

# Test connection directly
echo "Testing direct connection..."
docker exec monitor-ci-test-db mysql -u monitor -ptest123 -e "SELECT 1" monitor_test

# Load schema
echo "Loading schema..."
docker exec -i monitor-ci-test-db mysql -u monitor -ptest123 monitor_test < schema.sql

# Run just the integration tests
echo "Running tests..."
docker run --rm \
    --link monitor-ci-test-db:database \
    -e TEST_DATABASE_URL="monitor:test123@tcp(database:3306)/monitor_test?parseTime=true&multiStatements=true" \
    -v "$(pwd):/app" \
    -w /app \
    golang:1.24 \
    bash -c "
        # Install mysql client
        apt-get update && apt-get install -y default-mysql-client

        # Test database connection from golang container
        echo 'Testing connection from golang container...'
        mysql -h database -u monitor -ptest123 -e 'SHOW TABLES' monitor_test

        # Run only the integration tests
        echo 'Running integration tests...'
        go test -tags=integration -v ./scorer/... ./selector/... -run Integration
    "

# Cleanup
echo "Cleaning up..."
docker rm -f monitor-ci-test-db

echo "=== Test complete ==="
