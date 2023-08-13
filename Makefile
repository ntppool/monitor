generate: sqlc
	go generate ./...

sqlc:
	sqlc compile
	sqlc generate

TAG ?= ""

tag:
	GIT_COMMITTER_DATE="$(git show --format=%aD | head -1)" git tag -a $(TAG)

docker:
	docker build -t harbor.ntppool.org/ntppool/monitor-api:$(TAG) .
	docker push harbor.ntppool.org/ntppool/monitor-api:$(TAG)

sign:
	drone sign --save ntppool/monitor
