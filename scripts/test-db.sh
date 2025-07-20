#!/bin/bash

# Test Database Setup Script for NTP Monitor
# This script sets up a MySQL database in Docker for running integration tests

set -e

# Configuration
CONTAINER_NAME="ntpmonitor-test-db"
DB_NAME="monitor_test"
DB_USER="monitor"
DB_PASSWORD="test123"
DB_ROOT_PASSWORD="rootpass123"
DB_PORT="3308"  # Unified port for all test databases
MYSQL_VERSION="8.0"

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

# Function to check if Docker is running
check_docker() {
    if ! docker info >/dev/null 2>&1; then
        print_error "Docker is not running. Please start Docker and try again."
        exit 1
    fi
    print_status "Docker is running"
}

# Function to stop and remove existing container
cleanup_existing() {
    if docker ps -a --format '{{.Names}}' | grep -q "^${CONTAINER_NAME}$"; then
        print_warning "Stopping and removing existing container: ${CONTAINER_NAME}"
        docker stop "${CONTAINER_NAME}" >/dev/null 2>&1 || true
        docker rm "${CONTAINER_NAME}" >/dev/null 2>&1 || true
        print_success "Existing container removed"
    fi
}

# Function to start MySQL container
start_mysql() {
    print_status "Starting MySQL ${MYSQL_VERSION} container..."

    docker run -d \
        --name "${CONTAINER_NAME}" \
        -e MYSQL_ROOT_PASSWORD="${DB_ROOT_PASSWORD}" \
        -e MYSQL_DATABASE="${DB_NAME}" \
        -e MYSQL_USER="${DB_USER}" \
        -e MYSQL_PASSWORD="${DB_PASSWORD}" \
        -p "${DB_PORT}:3306" \
        --health-cmd='mysqladmin ping --silent' \
        --health-interval=10s \
        --health-timeout=5s \
        --health-retries=5 \
        mysql:"${MYSQL_VERSION}" \
        --character-set-server=utf8mb4 \
        --collation-server=utf8mb4_unicode_ci \
        --sql-mode="TRADITIONAL,NO_ZERO_DATE,NO_ZERO_IN_DATE,ERROR_FOR_DIVISION_BY_ZERO" \
        --max-connections=200

    print_success "MySQL container started"
}

# Function to wait for MySQL to be ready
wait_for_mysql() {
    print_status "Waiting for MySQL to be ready..."

    local max_attempts=30
    local attempt=1

    while [ $attempt -le $max_attempts ]; do
        if docker exec "${CONTAINER_NAME}" mysqladmin ping -h localhost -u root -p"${DB_ROOT_PASSWORD}" --silent >/dev/null 2>&1; then
            print_success "MySQL is ready!"
            return 0
        fi

        echo -n "."
        sleep 2
        attempt=$((attempt + 1))
    done

    print_error "MySQL failed to start within $(($max_attempts * 2)) seconds"
    docker logs "${CONTAINER_NAME}"
    exit 1
}

# Function to load database schema
load_schema() {
    print_status "Loading database schema..."

    # Check if schema file exists
    if [ ! -f "schema.sql" ]; then
        print_error "schema.sql not found. Please run this script from the monitor root directory."
        exit 1
    fi

    # Copy schema to container and load it
    docker cp schema.sql "${CONTAINER_NAME}:/tmp/schema.sql"

    if ! docker exec "${CONTAINER_NAME}" mysql -u root -p"${DB_ROOT_PASSWORD}" "${DB_NAME}" -e "source /tmp/schema.sql" >/dev/null 2>&1; then
        print_error "Failed to load schema"
        # Show the error
        docker exec "${CONTAINER_NAME}" mysql -u root -p"${DB_ROOT_PASSWORD}" "${DB_NAME}" -e "source /tmp/schema.sql"
        exit 1
    fi

    print_success "Database schema loaded"
}

# Function to create additional test user with full privileges
setup_test_user() {
    print_status "Setting up test user with full privileges..."

    docker exec "${CONTAINER_NAME}" mysql -u root -p"${DB_ROOT_PASSWORD}" -e "
        GRANT ALL PRIVILEGES ON \`${DB_NAME}\`.* TO '${DB_USER}'@'%';
        GRANT CREATE, DROP, REFERENCES ON *.* TO '${DB_USER}'@'%';
        FLUSH PRIVILEGES;
    " >/dev/null 2>&1

    print_success "Test user configured with full privileges"
}

# Function to generate test data
generate_test_data() {
    print_status "Generating test data..."

    # Create a SQL script with test data
    cat > /tmp/monitor_test_data.sql << 'EOF'
-- Test data for monitor integration tests

-- Insert test accounts
INSERT INTO accounts (id, email, created_on) VALUES
(1001, 'test1@example.com', NOW()),
(1002, 'test2@example.com', NOW()),
(1003, 'test3@example.com', NOW());

-- Insert test monitors
INSERT INTO monitors (id, tls_name, account_id, ip, status, type, created_on) VALUES
(2001, 'mon1.test.example', 1001, '10.0.0.1', 'active', 'monitor', NOW()),
(2002, 'mon2.test.example', 1001, '10.0.0.2', 'active', 'monitor', NOW()),
(2003, 'mon3.test.example', 1002, '10.0.0.3', 'testing', 'monitor', NOW()),
(2004, 'mon4.test.example', 1002, '10.0.0.4', 'active', 'monitor', NOW()),
(2005, 'mon5.test.example', 1003, '10.0.0.5', 'active', 'monitor', NOW());

-- Insert test servers
INSERT INTO servers (id, ip, ip_version, account_id, created_on) VALUES
(3001, '192.0.2.1', 'v4', 1001, NOW()),
(3002, '192.0.2.2', 'v4', 1002, NOW()),
(3003, '2001:db8::1', 'v6', 1001, NOW()),
(3004, '192.0.2.3', 'v4', NULL, NOW()),
(3005, '192.0.2.4', 'v4', 1003, NOW());

-- Insert some server_scores
INSERT INTO server_scores (server_id, monitor_id, status, score_raw, created_on) VALUES
(3001, 2001, 'active', 20.0, NOW()),
(3001, 2002, 'testing', 19.5, NOW()),
(3002, 2003, 'active', 18.0, NOW()),
(3002, 2004, 'candidate', 0, NOW());

-- Insert system settings
INSERT INTO system_settings (setting_key, setting_value) VALUES
('scorer', '{"batch_size": 100}'),
('monitor_global_max', '1000');

EOF

    # Copy and execute test data
    docker cp /tmp/monitor_test_data.sql "${CONTAINER_NAME}:/tmp/test_data.sql"

    if docker exec "${CONTAINER_NAME}" mysql -u root -p"${DB_ROOT_PASSWORD}" "${DB_NAME}" -e "source /tmp/test_data.sql" >/dev/null 2>&1; then
        print_success "Test data generated"
    else
        print_warning "Failed to generate test data (may already exist)"
    fi

    rm -f /tmp/monitor_test_data.sql
}

# Function to test database connection
test_connection() {
    print_status "Testing database connection..."

    # Test with docker exec
    if docker exec "${CONTAINER_NAME}" mysql -u "${DB_USER}" -p"${DB_PASSWORD}" -h localhost "${DB_NAME}" -e "SELECT 1;" >/dev/null 2>&1; then
        print_success "Database connection test passed"
    else
        print_error "Failed to connect to database"
        exit 1
    fi
}

# Function to show connection information
show_connection_info() {
    echo
    print_success "Test database is ready!"
    echo
    echo "Connection Details:"
    echo "  Host: localhost"
    echo "  Port: ${DB_PORT}"
    echo "  Database: ${DB_NAME}"
    echo "  Username: ${DB_USER}"
    echo "  Password: ${DB_PASSWORD}"
    echo
    echo "Connection String for Tests:"
    echo "  TEST_DATABASE_URL=\"${DB_USER}:${DB_PASSWORD}@tcp(localhost:${DB_PORT})/${DB_NAME}?parseTime=true&multiStatements=true\""
    echo
    echo "To run integration tests:"
    echo "  export TEST_DATABASE_URL=\"${DB_USER}:${DB_PASSWORD}@tcp(localhost:${DB_PORT})/${DB_NAME}?parseTime=true&multiStatements=true\""
    echo "  go test ./scorer/cmd -tags=integration -v"
    echo
    echo "To connect with mysql client:"
    echo "  mysql -h 127.0.0.1 -P ${DB_PORT} -u ${DB_USER} -p${DB_PASSWORD} ${DB_NAME}"
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
    echo "  start     Start the test database (default)"
    echo "  stop      Stop and remove the test database"
    echo "  restart   Restart the test database"
    echo "  status    Show database status"
    echo "  logs      Show database logs"
    echo "  shell     Open MySQL shell"
    echo "  reset     Stop, remove, and recreate with fresh data"
    echo "  help      Show this help message"
    echo
    echo "Examples:"
    echo "  $0                    # Start the test database"
    echo "  $0 start              # Start the test database"
    echo "  $0 stop               # Stop the test database"
    echo "  $0 shell              # Open MySQL shell"
}

# Command handlers
cmd_start() {
    check_docker
    cleanup_existing
    start_mysql
    wait_for_mysql
    load_schema
    setup_test_user
    generate_test_data
    test_connection
    show_connection_info
}

cmd_stop() {
    if docker ps --format '{{.Names}}' | grep -q "^${CONTAINER_NAME}$"; then
        print_status "Stopping test database..."
        docker stop "${CONTAINER_NAME}" >/dev/null
        docker rm "${CONTAINER_NAME}" >/dev/null
        print_success "Test database stopped and removed"
    else
        print_warning "Test database is not running"
    fi
}

cmd_restart() {
    cmd_stop
    sleep 2
    cmd_start
}

cmd_status() {
    if docker ps --format '{{.Names}}' | grep -q "^${CONTAINER_NAME}$"; then
        print_success "Test database is running"
        docker ps --filter name="${CONTAINER_NAME}" --format "table {{.Names}}\t{{.Status}}\t{{.Ports}}"

        # Test connection
        if docker exec "${CONTAINER_NAME}" mysqladmin ping -h localhost -u root -p"${DB_ROOT_PASSWORD}" --silent >/dev/null 2>&1; then
            print_success "Database is accepting connections"
        else
            print_warning "Database is not ready yet"
        fi
    else
        print_warning "Test database is not running"
        echo "Run '$0 start' to start the test database"
    fi
}

cmd_logs() {
    if docker ps -a --format '{{.Names}}' | grep -q "^${CONTAINER_NAME}$"; then
        docker logs "${CONTAINER_NAME}"
    else
        print_error "Test database container not found"
    fi
}

cmd_shell() {
    if docker ps --format '{{.Names}}' | grep -q "^${CONTAINER_NAME}$"; then
        print_status "Opening MySQL shell (use 'exit' to close)..."
        docker exec -it "${CONTAINER_NAME}" mysql -u "${DB_USER}" -p"${DB_PASSWORD}" "${DB_NAME}"
    else
        print_error "Test database is not running. Run '$0 start' first."
    fi
}

cmd_reset() {
    print_status "Resetting test database..."
    cmd_stop
    sleep 1
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
    logs)
        cmd_logs
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
