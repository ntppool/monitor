generate: sqlc
	go generate ./...

sqlc:
	sqlc compile
	sqlc generate

TAG ?= 0.1.0

docker:
	docker build -t harbor.ntppool.org/ntppool/monitor-api:$(TAG) .
	docker push harbor.ntppool.org/ntppool/monitor-api:$(TAG)

sign:
	drone sign --save ntppool/monitor
