BINARY := hyundai-bluelink-mqtt
BIN_DIR := bin
PKG     := ./cmd

.PHONY: build run run-mock test test-e2e

## build: compile the binary into ./bin
build:
	go build -o $(BIN_DIR)/$(BINARY) $(PKG)

## test: run unit + integration tests (excludes the godog features suite)
test:
	go test $(shell go list ./... | grep -v /features)

## test-e2e: run the godog acceptance suite
test-e2e:
	go test ./features/...

## run: build then run the poll -> MQTT service (respects MODE from .env)
run: build
	./$(BIN_DIR)/$(BINARY) serve

## run-mock: build then run the standalone mock Bluelink API (--ccs1 for CCS1)
run-mock: build
	./$(BIN_DIR)/$(BINARY) mock
