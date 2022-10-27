default: run

run: build
	@ echo "Run..."
	@ ./bin/rview --port=8090 --rclone-url=http://localhost:8080 --dir=_var --debug

build:
	@ echo "Build..."
	@ go build -o ./bin/rview

check: lint test

test:
	@ echo "Run tests..."
	@ go test -v -count=1 ./...

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
		golangci/golangci-lint:v1.50-alpine golangci-lint run --config .golangci.yml -v
