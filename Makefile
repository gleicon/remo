BINARY  := build/remo
MODULE  := github.com/gleicon/remo
SRC     := ./cmd/remo
VERSION ?= $(shell git describe --tags --always --dirty 2>/dev/null || echo dev)
LDFLAGS := -s -w -X main.version=$(VERSION)

PLATFORMS := linux/amd64 linux/arm64 darwin/amd64 darwin/arm64 windows/amd64

.PHONY: all build clean test test-v cover deps tidy fmt vet lint check dist help

all: check build

build:
	go build -ldflags "$(LDFLAGS)" -o $(BINARY) $(SRC)

dist:
	@mkdir -p dist
	@for platform in $(PLATFORMS); do \
		os=$${platform%%/*}; \
		arch=$${platform##*/}; \
		ext=""; \
		if [ "$$os" = "windows" ]; then ext=".exe"; fi; \
		echo "building dist/$(BINARY)-$$os-$$arch$$ext"; \
		GOOS=$$os GOARCH=$$arch go build -ldflags "$(LDFLAGS)" \
			-o dist/$(BINARY)-$$os-$$arch$$ext $(SRC); \
	done

clean:
	rm -f $(BINARY)
	rm -rf dist/
	go clean -cache -testcache

deps:
	go mod download

tidy:
	go mod tidy

fmt:
	gofmt -s -w .

vet:
	go vet ./...

lint: vet
	@if command -v staticcheck >/dev/null 2>&1; then \
		staticcheck ./...; \
	else \
		echo "staticcheck not installed, skipping (go install honnef.co/go/tools/cmd/staticcheck@latest)"; \
	fi

test:
	go test ./...

test-v:
	go test -v -count=1 ./...

cover:
	go test -coverprofile=coverage.out ./...
	go tool cover -func=coverage.out
	@rm -f coverage.out

check: fmt vet test

help:
	@echo "usage: make [target]"
	@echo ""
	@echo "targets:"
	@echo "  all      fmt, vet, test, then build (default)"
	@echo "  build    compile the remo binary"
	@echo "  dist     cross-compile for linux/darwin/windows (amd64+arm64)"
	@echo "  clean    remove binary, dist/, and Go caches"
	@echo "  deps     download module dependencies"
	@echo "  tidy     run go mod tidy"
	@echo "  fmt      format all Go source files"
	@echo "  vet      run go vet"
	@echo "  lint     run vet + staticcheck (if installed)"
	@echo "  test     run tests"
	@echo "  test-v   run tests verbose, no cache"
	@echo "  cover    run tests with coverage summary"
	@echo "  check    fmt + vet + test"
	@echo "  help     show this help"
