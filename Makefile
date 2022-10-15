default: run

run:
	go run . --port=8090 --rclone-url=http://localhost:8080 --dir=_var

build:
	go build -o ./bin/rview .

check: lint test

# TODO: add golangci-lint
lint:
	go vet ./...

test:
	go test -v -count=1 ./...
