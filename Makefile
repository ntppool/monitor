build: generate test server client

generate: sqlc
	go generate ./...

sqlc:
	@which gowrap  >& /dev/null || (echo "Run 'go install github.com/hexdigest/gowrap/cmd/gowrap@v1.3.2'" && exit 1)
	@which mockery >& /dev/null || (echo "Run 'go install github.com/vektra/mockery/v2@v2.42.0'" && exit 1)
	sqlc compile
	sqlc generate
	gowrap gen -t opentelemetry -i QuerierTx -p ./ntpdb -o ./ntpdb/otel.go
	mockery --dir ntpdb --name QuerierTx --config /dev/null

test:
	go test -v ./...

server:
	go build ./server/cmd/monitor-api

client:
	go build ./client/cmd/ntppool-monitor

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
