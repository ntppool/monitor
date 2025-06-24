#!/bin/bash

# Script to run CI-like tests locally using Docker Compose
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

# Function to cleanup containers
cleanup() {
    print_status "Cleaning up containers..."
    docker-compose -f docker-compose.test.yml down -v || true
}

# Set trap to cleanup on exit
trap cleanup EXIT

# Check if Docker is running
if ! docker info >/dev/null 2>&1; then
    print_error "Docker is not running. Please start Docker and try again."
    exit 1
fi

print_status "Starting CI-like test environment..."

# Stop any existing containers
cleanup

# Start the test environment
print_status "Starting database and test runner..."
docker-compose -f docker-compose.test.yml up --build --abort-on-container-exit --exit-code-from test-runner

# Check exit code
EXIT_CODE=$?

if [ $EXIT_CODE -eq 0 ]; then
    print_success "All tests passed!"
else
    print_error "Tests failed with exit code: $EXIT_CODE"

    # Show logs for debugging
    print_status "Showing recent logs..."
    docker-compose -f docker-compose.test.yml logs --tail=100
fi

exit $EXIT_CODE
