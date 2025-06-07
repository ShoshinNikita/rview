-include .env .env.local

.PHONY: build check default docker-build lint run test

default: run

run: build
	@ echo "Run..."
	@ ./bin/rview \
			--port=${SERVER_PORT} \
			--dir=_var \
			--rclone-target=${RCLONE_TARGET} \
			--image-preview-mode=${IMAGE_PREVIEW_MODE} \
			--log-level=${LOG_LEVEL} \
			--thumbnails-format=${THUMBNAILS_FORMAT} \
			--thumbnails-process-raw-files=${THUMBNAILS_PROCESS_RAW_FILES} \
			--read-static-files-from-disk

build:
	@ echo "Build..."
	@ CGO_ENABLED=0 go build -trimpath -ldflags "-s -w" -o ./bin/rview

docker-build:
	@ echo "Build docker image..."
	@ docker build -t rview .

check: build lint test docker-build

test:
	@ echo "Run tests..."
	@ go test -v -count=1 \
		-cover -coverprofile=_cover.out -coverpkg=github.com/ShoshinNikita/rview/... \
		./...
	@ go tool cover -func=_cover.out
	@ rm _cover.out

docker-test:
	@ echo "Run tests in docker..."
	@ docker build -f test.Dockerfile --progress=plain .

# Use go cache to speed up execution: https://github.com/golangci/golangci-lint/issues/1004
lint:
	@ echo "Run golangci-lint..."
	@ docker run --rm -t \
		--network=none \
		--user ${UID}:${GID} \
		-e GOCACHE=/cache/go \
		-e GOLANGCI_LINT_CACHE=/cache/go \
		-v $(shell go env GOCACHE):/cache/go \
		-v $(shell go env GOPATH)/pkg:/go/pkg \
		-v $(shell pwd):/app \
		-w /app \
		golangci/golangci-lint:v2.1.6-alpine golangci-lint run -v --config .golangci.yml
