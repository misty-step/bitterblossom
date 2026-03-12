.PHONY: build test lint clean test-python lint-python conductor-check conductor-start conductor-install-cron conductor-status conductor-stop

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
	python3 -m pytest -q base/hooks scripts/test_conductor.py scripts/test_conductor_supervise.py

lint-python:
	ruff check base/hooks scripts/conductor.py scripts/test_conductor.py scripts/test_conductor_supervise.py

conductor-check:
	python3 scripts/conductor.py check-env

conductor-start:
	./scripts/conductor-supervise.sh start $(CONDUCTOR_SUPERVISOR_ARGS)

conductor-install-cron:
	./scripts/conductor-supervise.sh install-cron $(CONDUCTOR_SUPERVISOR_ARGS)

conductor-status:
	./scripts/conductor-supervise.sh status

conductor-stop:
	./scripts/conductor-supervise.sh stop
