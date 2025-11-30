#!/bin/bash

# Test Database Setup Script for NTP Monitor
# This script sets up a PostgreSQL test database for running integration tests
# Assumes PostgreSQL is running on localhost:5432

set -e

# Configuration
DB_NAME="monitor_test"
DB_USER="monitor"
DB_PASSWORD="test123"
DB_HOST="localhost"
DB_PORT="5432"
POSTGRES_ADMIN_USER="${POSTGRES_USER:-postgres}"

# Colors for output
RED='\033[0;31m'
GREEN='\033[0;32m'
YELLOW='\033[1;33m'
BLUE='\033[0;34m'
NC='\033[0m' # No Color

# Function to print colored output
print_status() {
    echo -e "${BLUE}[INFO]${NC} $1"
}

print_success() {
    echo -e "${GREEN}[SUCCESS]${NC} $1"
}

print_warning() {
    echo -e "${YELLOW}[WARNING]${NC} $1"
}

print_error() {
    echo -e "${RED}[ERROR]${NC} $1"
}

# Function to check if PostgreSQL is running
check_postgres() {
    if ! psql -U "$POSTGRES_ADMIN_USER" -h "$DB_HOST" -p "$DB_PORT" -d postgres -c '\q' >/dev/null 2>&1; then
        print_error "PostgreSQL is not running or not accessible on ${DB_HOST}:${DB_PORT}"
        print_error "Please ensure PostgreSQL is installed and running, then try again."
        exit 1
    fi
    print_status "PostgreSQL is running on ${DB_HOST}:${DB_PORT}"
}

# Function to check if database exists
database_exists() {
    psql -U "$POSTGRES_ADMIN_USER" -h "$DB_HOST" -p "$DB_PORT" -d postgres -tAc "SELECT 1 FROM pg_database WHERE datname='${DB_NAME}'" | grep -q 1
}

# Function to check if user exists
user_exists() {
    psql -U "$POSTGRES_ADMIN_USER" -h "$DB_HOST" -p "$DB_PORT" -d postgres -tAc "SELECT 1 FROM pg_user WHERE usename='${DB_USER}'" | grep -q 1
}

# Function to create test user
create_test_user() {
    if user_exists; then
        print_status "User '${DB_USER}' already exists"
    else
        print_status "Creating test user '${DB_USER}'..."
        psql -U "$POSTGRES_ADMIN_USER" -h "$DB_HOST" -p "$DB_PORT" -d postgres <<EOF
CREATE USER ${DB_USER} WITH PASSWORD '${DB_PASSWORD}';
EOF
        print_success "Test user '${DB_USER}' created"
    fi
}

# Function to create test database
create_database() {
    if database_exists; then
        print_warning "Database '${DB_NAME}' already exists"
        return 0
    fi

    print_status "Creating test database '${DB_NAME}'..."
    createdb -U "$POSTGRES_ADMIN_USER" -h "$DB_HOST" -p "$DB_PORT" -O "$DB_USER" "$DB_NAME"
    print_success "Database '${DB_NAME}' created"
}

# Function to grant privileges
grant_privileges() {
    print_status "Granting privileges to '${DB_USER}'..."
    psql -U "$POSTGRES_ADMIN_USER" -h "$DB_HOST" -p "$DB_PORT" -d "$DB_NAME" <<EOF
GRANT ALL PRIVILEGES ON DATABASE ${DB_NAME} TO ${DB_USER};
GRANT ALL ON SCHEMA public TO ${DB_USER};
ALTER DEFAULT PRIVILEGES IN SCHEMA public GRANT ALL ON TABLES TO ${DB_USER};
ALTER DEFAULT PRIVILEGES IN SCHEMA public GRANT ALL ON SEQUENCES TO ${DB_USER};
EOF
    print_success "Privileges granted to '${DB_USER}'"
}

# Function to load database schema
load_schema() {
    print_status "Loading database schema..."

    # Check if schema file exists
    if [ ! -f "schema.sql" ]; then
        print_error "schema.sql not found. Please run this script from the monitor root directory."
        exit 1
    fi

    # Load schema
    if PGPASSWORD="$DB_PASSWORD" psql -U "$DB_USER" -h "$DB_HOST" -p "$DB_PORT" -d "$DB_NAME" -f schema.sql >/dev/null 2>&1; then
        print_success "Database schema loaded"
    else
        print_error "Failed to load schema"
        PGPASSWORD="$DB_PASSWORD" psql -U "$DB_USER" -h "$DB_HOST" -p "$DB_PORT" -d "$DB_NAME" -f schema.sql
        exit 1
    fi
}

# Function to drop test database
drop_database() {
    if ! database_exists; then
        print_warning "Database '${DB_NAME}' does not exist"
        return 0
    fi

    print_status "Dropping test database '${DB_NAME}'..."
    dropdb -U "$POSTGRES_ADMIN_USER" -h "$DB_HOST" -p "$DB_PORT" --if-exists "$DB_NAME"
    print_success "Database '${DB_NAME}' dropped"
}

# Function to test database connection
test_connection() {
    print_status "Testing database connection..."

    if PGPASSWORD="$DB_PASSWORD" psql -U "$DB_USER" -h "$DB_HOST" -p "$DB_PORT" -d "$DB_NAME" -c '\q' >/dev/null 2>&1; then
        print_success "Database connection test passed"
        return 0
    else
        print_error "Failed to connect to database"
        return 1
    fi
}

# Function to show connection information
show_connection_info() {
    echo
    print_success "Test database is ready!"
    echo
    echo "Connection Details:"
    echo "  Host: ${DB_HOST}"
    echo "  Port: ${DB_PORT}"
    echo "  Database: ${DB_NAME}"
    echo "  Username: ${DB_USER}"
    echo "  Password: ${DB_PASSWORD}"
    echo
    echo "Connection String for Tests:"
    echo "  TEST_DATABASE_URL=\"postgres://${DB_USER}:${DB_PASSWORD}@${DB_HOST}:${DB_PORT}/${DB_NAME}?sslmode=disable\""
    echo
    echo "To run integration tests:"
    echo "  export TEST_DATABASE_URL=\"postgres://${DB_USER}:${DB_PASSWORD}@${DB_HOST}:${DB_PORT}/${DB_NAME}?sslmode=disable\""
    echo "  go test ./scorer -tags=integration -v"
    echo
    echo "To connect with psql client:"
    echo "  psql postgres://${DB_USER}:${DB_PASSWORD}@${DB_HOST}:${DB_PORT}/${DB_NAME}"
    echo
    echo "To stop the test database:"
    echo "  $0 stop"
    echo
}

# Function to show usage
show_usage() {
    echo "Usage: $0 [COMMAND]"
    echo
    echo "Commands:"
    echo "  start     Create test database and load schema (default)"
    echo "  stop      Drop the test database"
    echo "  restart   Drop and recreate the test database"
    echo "  status    Show database status"
    echo "  shell     Open psql shell to test database"
    echo "  reset     Drop and recreate with fresh schema"
    echo "  help      Show this help message"
    echo
    echo "Prerequisites:"
    echo "  - PostgreSQL running on localhost:5432"
    echo "  - PostgreSQL admin access (default user: postgres)"
    echo
    echo "Examples:"
    echo "  $0                    # Create test database"
    echo "  $0 start              # Create test database"
    echo "  $0 stop               # Drop test database"
    echo "  $0 shell              # Open psql shell"
}

# Command handlers
cmd_start() {
    check_postgres
    create_test_user
    create_database
    grant_privileges
    load_schema
    test_connection
    show_connection_info
}

cmd_stop() {
    check_postgres
    drop_database
}

cmd_restart() {
    cmd_stop
    echo
    cmd_start
}

cmd_status() {
    check_postgres

    if database_exists; then
        print_success "Test database '${DB_NAME}' exists"

        if test_connection; then
            print_success "Database is accessible"
        fi
    else
        print_warning "Test database '${DB_NAME}' does not exist"
        echo "Run '$0 start' to create the test database"
    fi
}

cmd_shell() {
    check_postgres

    if ! database_exists; then
        print_error "Test database '${DB_NAME}' does not exist. Run '$0 start' first."
        exit 1
    fi

    print_status "Opening psql shell (use '\\q' or Ctrl+D to exit)..."
    PGPASSWORD="$DB_PASSWORD" psql -U "$DB_USER" -h "$DB_HOST" -p "$DB_PORT" -d "$DB_NAME"
}

cmd_reset() {
    print_status "Resetting test database..."
    cmd_stop
    echo
    cmd_start
}

# Main script logic
case "${1:-start}" in
    start)
        cmd_start
        ;;
    stop)
        cmd_stop
        ;;
    restart)
        cmd_restart
        ;;
    status)
        cmd_status
        ;;
    shell)
        cmd_shell
        ;;
    reset)
        cmd_reset
        ;;
    help|--help|-h)
        show_usage
        ;;
    *)
        print_error "Unknown command: $1"
        echo
        show_usage
        exit 1
        ;;
esac
