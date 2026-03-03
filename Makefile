BINARY_NAME := yokai
VERSION := $(shell git describe --tags --always --dirty 2>/dev/null || echo "dev")
BUILD_TIME := $(shell date -u +"%Y-%m-%dT%H:%M:%SZ")
LDFLAGS := -ldflags "-s -w -X main.version=$(VERSION) -X main.buildTime=$(BUILD_TIME)"

.PHONY: build run clean test lint agent

build:
	go build $(LDFLAGS) -o bin/$(BINARY_NAME) ./cmd/yokai

run: build
	./bin/$(BINARY_NAME)

agent: build
	./bin/$(BINARY_NAME) agent

daemon: build
	./bin/$(BINARY_NAME) daemon

clean:
	rm -rf bin/

test:
	go test ./...

lint:
	golangci-lint run

tidy:
	go mod tidy

cross:
	GOOS=linux GOARCH=amd64 go build $(LDFLAGS) -o bin/$(BINARY_NAME)-linux-amd64 ./cmd/yokai
	GOOS=linux GOARCH=arm64 go build $(LDFLAGS) -o bin/$(BINARY_NAME)-linux-arm64 ./cmd/yokai
	GOOS=darwin GOARCH=arm64 go build $(LDFLAGS) -o bin/$(BINARY_NAME)-darwin-arm64 ./cmd/yokai
