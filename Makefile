.PHONY: build test lint clean

BINARY := bb
BIN_DIR := bin

build:
	mkdir -p $(BIN_DIR)
	go build -o $(BIN_DIR)/$(BINARY) ./cmd/bb

test:
	go test ./...
	./scripts/test_legacy_wrappers.sh

lint:
	golangci-lint run

clean:
	rm -rf $(BIN_DIR)
