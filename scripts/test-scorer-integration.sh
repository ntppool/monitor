#!/bin/bash

# Test script for scorer integration tests
# This script sets up a test database and runs the scorer integration tests

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

# Check for existing database on port 3308
if lsof -i :3308 >/dev/null 2>&1; then
    print_error "Port 3308 is already in use. Another database may be running."
    print_status "You can stop the existing database with: make test-db-stop"
    exit 1
fi

# Clean up any existing containers
print_status "Cleaning up existing containers..."
docker rm -f test-mysql 2>/dev/null || true

# Start MySQL container
print_status "Starting MySQL container..."
docker run -d \
    --name test-mysql \
    -e MYSQL_ROOT_PASSWORD=root \
    -e MYSQL_DATABASE=monitor_test \
    -e MYSQL_USER=monitor \
    -e MYSQL_PASSWORD=test123 \
    -p 3308:3306 \
    mysql:8.0

# Wait for MySQL to be ready
print_status "Waiting for MySQL to start..."
for i in $(seq 1 30); do
    if docker exec test-mysql mysqladmin ping -h localhost --silent >/dev/null 2>&1; then
        print_success "MySQL is ready!"
        break
    fi
    echo "Waiting for MySQL... ($i/30)"
    sleep 2
done

# Load schema
print_status "Loading database schema..."
docker exec -i test-mysql mysql -u monitor -ptest123 monitor_test < schema.sql

# Run the integration tests
print_status "Running scorer integration tests..."
TEST_DATABASE_URL="monitor:test123@tcp(localhost:3308)/monitor_test?parseTime=true&multiStatements=true" \
    go test ./scorer -tags=integration -v "$@"

EXIT_CODE=$?

# Clean up
print_status "Cleaning up..."
docker rm -f test-mysql >/dev/null 2>&1 || true

if [ $EXIT_CODE -eq 0 ]; then
    print_success "All tests passed!"
else
    print_error "Tests failed with exit code: $EXIT_CODE"
fi

exit $EXIT_CODE
