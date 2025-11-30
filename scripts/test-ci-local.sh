#!/bin/bash

# Script to run CI-like tests locally using native PostgreSQL
# This emulates the Drone CI environment

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

print_warning() {
    echo -e "${YELLOW}[WARNING]${NC} $1"
}

# Configuration
DB_NAME="monitor_test"
DB_USER="monitor"
DB_PASSWORD="test123"
DB_HOST="localhost"
DB_PORT="5432"

# Function to cleanup
cleanup() {
    print_status "Cleaning up..."
    "$SCRIPT_DIR/test-db.sh" stop 2>/dev/null || true
}

# Set trap to cleanup on exit (optional - comment out to keep DB running)
# trap cleanup EXIT

print_status "Starting CI-like test environment..."

# Start the test database
print_status "Setting up test database..."
"$SCRIPT_DIR/test-db.sh" restart

# Export test database URL
export TEST_DATABASE_URL="postgres://${DB_USER}:${DB_PASSWORD}@${DB_HOST}:${DB_PORT}/${DB_NAME}?sslmode=disable"

# Run the build
print_status "Building project..."
go build ./...

# Run short tests
print_status "Running short tests..."
go test -v ./... -short

# Run integration tests
print_status "Running integration tests..."
make test-integration

EXIT_CODE=$?

if [ $EXIT_CODE -eq 0 ]; then
    print_success "All tests passed!"
else
    print_error "Tests failed with exit code: $EXIT_CODE"
fi

exit $EXIT_CODE
