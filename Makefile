default: run

run:
	go run .

build:
	go build -o ./bin/rview .

check: lint test

# TODO: add golangci-lint
lint:
	go vet ./...

test:
	go test -v -count=1 ./...
