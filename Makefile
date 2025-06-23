build: generate test server client

generate: sqlc
	go generate ./...

sqlc:
	sqlc compile
	sqlc generate

test:
	go test -v ./...

# Test database management
test-db-start:
	./scripts/test-db.sh start

test-db-stop:
	./scripts/test-db.sh stop

test-db-restart:
	./scripts/test-db.sh restart

test-db-status:
	./scripts/test-db.sh status

test-db-shell:
	./scripts/test-db.sh shell

test-db-reset:
	@echo "Resetting test database (drop, recreate, load schema)..."
	./scripts/test-db.sh reset

# Run unit tests only (fast, no dependencies)
test-unit:
	go test ./... -short -v

# Run integration tests (requires test database)
test-integration:
	@if [ -z "$$TEST_DATABASE_URL" ]; then \
		echo "Starting test database..."; \
		./scripts/test-db.sh start > /dev/null 2>&1 || true; \
		echo "Running integration tests..."; \
		TEST_DATABASE_URL="monitor:test123@tcp(localhost:3308)/monitor_test?parseTime=true&multiStatements=true" go test ./... -tags=integration -v; \
	else \
		echo "Using existing TEST_DATABASE_URL..."; \
		go test ./... -tags=integration -v; \
	fi

# Run load tests (requires test database)
test-load:
	@if [ -z "$$TEST_DATABASE_URL" ]; then \
		echo "Starting test database..."; \
		./scripts/test-db.sh start > /dev/null 2>&1 || true; \
		echo "Running load tests..."; \
		TEST_DATABASE_URL="monitor:test123@tcp(localhost:3308)/monitor_test?parseTime=true&multiStatements=true" go test ./... -tags=load -v -timeout=30m; \
	else \
		echo "Using existing TEST_DATABASE_URL..."; \
		go test ./... -tags=load -v -timeout=30m; \
	fi

# Run all tests (unit + integration + load)
test-all: test-unit test-integration

# Generate test coverage report
coverage:
	go test -coverprofile=coverage.out ./...
	go tool cover -html=coverage.out -o coverage.html
	@echo "Coverage report generated: coverage.html"

# Benchmark tests
benchmark:
	go test -bench=. -benchmem ./...

tools:
	go install connectrpc.com/connect/cmd/protoc-gen-connect-go@latest
	go install github.com/bufbuild/buf/cmd/buf@latest
	go install github.com/fullstorydev/grpcurl/cmd/grpcurl@latest
	go install google.golang.org/protobuf/cmd/protoc-gen-go@latest
	go install github.com/twitchtv/twirp/protoc-gen-twirp@latest

TAG ?= ""

tag:
	GIT_COMMITTER_DATE="$(git show --format=%aD | head -1)" git tag -a $(TAG)

docker:
	docker build -t harbor.ntppool.org/ntppool/monitor-api:$(TAG) .
	docker push harbor.ntppool.org/ntppool/monitor-api:$(TAG)

sign:
	drone sign --save ntppool/monitor
