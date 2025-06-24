#!/bin/bash

# Debug script to test CI commands step by step
# This allows us to debug each step of the CI process

set -e

SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
PROJECT_ROOT="$(cd "$SCRIPT_DIR/.." && pwd)"

cd "$PROJECT_ROOT"

# Colors for output
RED='\033[0;31m'
GREEN='\033[0;32m'
YELLOW='\033[1;33m'
BLUE='\033[0;34m'
NC='\033[0m' # No Color

print_status() {
    echo -e "${BLUE}[INFO]${NC} $1"
}

print_success() {
    echo -e "${GREEN}[SUCCESS]${NC} $1"
}

print_error() {
    echo -e "${RED}[ERROR]${NC} $1"
}

# Start MySQL container separately
print_status "Starting MySQL container..."
docker run -d \
    --name monitor-test-db \
    -e MYSQL_ROOT_PASSWORD=root \
    -e MYSQL_DATABASE=monitor_test \
    -e MYSQL_USER=monitor \
    -e MYSQL_PASSWORD=test123 \
    -p 3308:3306 \
    --health-cmd='mysqladmin ping --silent' \
    --health-interval=10s \
    --health-timeout=5s \
    --health-retries=5 \
    mysql:8.0

# Wait for MySQL to be healthy
print_status "Waiting for MySQL to be healthy..."
for i in $(seq 1 30); do
    if docker exec monitor-test-db mysqladmin ping -h localhost --silent >/dev/null 2>&1; then
        print_success "MySQL is ready!"
        break
    fi
    echo "Waiting for MySQL... ($i/30)"
    sleep 2
done

# Load schema
print_status "Loading database schema..."
docker exec -i monitor-test-db mysql -u root -proot monitor_test < schema.sql

# Run tests in a golang container
print_status "Running tests in golang container..."
docker run --rm \
    --name monitor-test-runner \
    --link monitor-test-db:database \
    -e TEST_DATABASE_URL="monitor:test123@tcp(database:3306)/monitor_test?parseTime=true&multiStatements=true" \
    -v "$PROJECT_ROOT:/workspace" \
    -w /workspace \
    golang:1.24 \
    bash -c "
        set -e
        echo '=== Installing dependencies ==='
        apt update && apt install -y default-mysql-client git

        echo '=== Testing database connection ==='
        mysql -h database -u monitor -ptest123 -e 'SELECT 1' monitor_test

        echo '=== Running go test -v ./... -short ==='
        go test -v ./... -short

        echo '=== Running make test-integration ==='
        make test-integration

        echo '=== Running go build ./... ==='
        go build ./...
    "

EXIT_CODE=$?

# Cleanup
print_status "Cleaning up..."
docker stop monitor-test-db >/dev/null 2>&1 || true
docker rm monitor-test-db >/dev/null 2>&1 || true

if [ $EXIT_CODE -eq 0 ]; then
    print_success "All tests passed!"
else
    print_error "Tests failed with exit code: $EXIT_CODE"
fi

exit $EXIT_CODE
