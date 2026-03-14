.PHONY: build test lint clean test-python lint-python test-conductor conductor-check

BINARY := bb
BIN_DIR := bin
VERSION ?= dev
COMMIT := $(shell git rev-parse --short HEAD 2>/dev/null || echo "unknown")
DATE := $(shell date -u +%Y-%m-%dT%H:%M:%SZ)
LDFLAGS := -X main.version=$(VERSION) -X main.commit=$(COMMIT) -X main.date=$(DATE)

build:
	mkdir -p $(BIN_DIR)
	go build -ldflags "$(LDFLAGS)" -o $(BIN_DIR)/$(BINARY) ./cmd/bb

test:
	go test ./...

lint:
	golangci-lint run

clean:
	rm -rf $(BIN_DIR)

test-python:
	python3 -m pytest -q base/hooks scripts/test_workflow_contract.py

lint-python:
	ruff check base/hooks scripts/test_workflow_contract.py

test-conductor:
	cd conductor && mix test

conductor-check:
	cd conductor && mix conductor check-env
