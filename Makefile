.PHONY: all build test lint clean test-hooks test-conductor conductor-check preflight

BINARY := bb
BIN_DIR := bin
VERSION ?= dev
COMMIT := $(shell git rev-parse --short HEAD 2>/dev/null || echo "unknown")
DATE := $(shell date -u +%Y-%m-%dT%H:%M:%SZ)
LDFLAGS := -X main.version=$(VERSION) -X main.commit=$(COMMIT) -X main.date=$(DATE)

all: build

build:
	mkdir -p $(BIN_DIR)
	go build -ldflags "$(LDFLAGS)" -o $(BIN_DIR)/$(BINARY) ./cmd/bb

test:
	go test ./...

lint:
	golangci-lint run

clean:
	rm -rf $(BIN_DIR)

test-hooks:
	python3 -m pytest -q base/hooks/

test-conductor:
	cd conductor && mix test

conductor-check:
	cd conductor && mix conductor check-env

preflight: build
	./bin/bb preflight
