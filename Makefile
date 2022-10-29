default: run

run: build
	@ echo "Run..."
	@ ./bin/rview --port=8090 --dir=_var --debug --rclone-target=$(shell pwd)

build:
	@ echo "Build..."
	@ CGO_ENABLED=0 go build -ldflags "-s -w" -o ./bin/rview

docker-build:
	@ echo "Build Docker Image..."
	@ docker build -t rview .

check: lint test build docker-build

test:
	@ echo "Run tests..."
	@ go test -v -count=1 \
		-cover -coverprofile=_cover.out -coverpkg=github.com/ShoshinNikita/rview/... \
		./...
	@ go tool cover -func=_cover.out
	@ rm _cover.out

# Use go cache to speed up execution: https://github.com/golangci/golangci-lint/issues/1004
lint:
	@ echo "Run golangci-lint..."
	@ docker run --rm -t \
		--network=none \
		--user $(shell id -u):$(shell id -g) \
		-e GOCACHE=/cache/go \
		-e GOLANGCI_LINT_CACHE=/cache/go \
		-v $(shell go env GOCACHE):/cache/go \
		-v $(shell go env GOPATH)/pkg:/go/pkg \
		-v $(shell pwd):/app \
		-w /app \
		golangci/golangci-lint:v1.50-alpine golangci-lint run --config .golangci.yml
