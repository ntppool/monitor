#!/bin/bash

# Test script for scorer integration tests
# This script sets up a PostgreSQL test database and runs the scorer integration tests

set -e

SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
PROJECT_ROOT="$(cd "$SCRIPT_DIR/.." && pwd)"

cd "$PROJECT_ROOT"

# Colors for output
RED='\033[0;31m'
GREEN='\033[0;32m'
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

# Configuration
DB_NAME="monitor_test"
DB_USER="monitor"
DB_PASSWORD="test123"
DB_HOST="localhost"
DB_PORT="5432"

# Setup test database using native PostgreSQL
print_status "Setting up test database..."
"$SCRIPT_DIR/test-db.sh" restart

# Run the integration tests
print_status "Running scorer integration tests..."
TEST_DATABASE_URL="postgres://${DB_USER}:${DB_PASSWORD}@${DB_HOST}:${DB_PORT}/${DB_NAME}?sslmode=disable" \
    go test ./scorer -tags=integration -v "$@"

EXIT_CODE=$?

if [ $EXIT_CODE -eq 0 ]; then
    print_success "All tests passed!"
else
    print_error "Tests failed with exit code: $EXIT_CODE"
fi

exit $EXIT_CODE
