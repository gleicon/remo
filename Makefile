.PHONY: build test

build:
	GO111MODULE=on go build ./cmd/remo

test:
	GO111MODULE=on go test ./...
