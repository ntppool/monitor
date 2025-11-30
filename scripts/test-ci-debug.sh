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

# Export test database URL
export TEST_DATABASE_URL="postgres://${DB_USER}:${DB_PASSWORD}@${DB_HOST}:${DB_PORT}/${DB_NAME}?sslmode=disable"

print_status "Testing database connection..."
PGPASSWORD="$DB_PASSWORD" psql -h "$DB_HOST" -U "$DB_USER" -d "$DB_NAME" -c 'SELECT 1'

print_status "Running go test -v ./... -short..."
go test -v ./... -short

print_status "Running make test-integration..."
make test-integration

print_status "Running go build ./..."
go build ./...

EXIT_CODE=$?

if [ $EXIT_CODE -eq 0 ]; then
    print_success "All tests passed!"
else
    print_error "Tests failed with exit code: $EXIT_CODE"
fi

exit $EXIT_CODE
