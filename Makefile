BINARY_NAME := yokai
VERSION := $(shell git describe --tags --always --dirty 2>/dev/null || echo "dev")
BUILD_TIME := $(shell date -u +"%Y-%m-%dT%H:%M:%SZ")
LDFLAGS := -ldflags "-s -w -X main.version=$(VERSION) -X main.buildTime=$(BUILD_TIME)"

# Remote agent hosts — space-separated list of ssh host aliases or user@host
# Override with: make dev-restart AGENTS="finn kyber"
AGENTS ?= finn
AGENT_PORT ?= 7474
AGENT_PATH ?= /usr/local/bin/yokai

.PHONY: build run clean test lint agent daemon
.PHONY: dev dev-restart dev-daemon dev-agents dev-tui dev-push

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

# ---------- Local dev workflow ----------

# Full rebuild + restart everything (daemon + all agents + TUI)
dev: build dev-daemon dev-agents dev-tui

# Rebuild, restart daemon + agents, but don't launch TUI
dev-restart: build dev-daemon dev-agents
	@echo "daemon and agents restarted — run 'make dev-tui' or './bin/$(BINARY_NAME)' when ready"

# Kill old daemon and start a new one (backgrounded)
dev-daemon: build
	@echo "restarting daemon..."
	@-pid=$$(lsof -ti :7473 2>/dev/null) && kill $$pid 2>/dev/null; sleep 0.5
	@nohup ./bin/$(BINARY_NAME) daemon > /tmp/yokai-daemon.log 2>&1 & echo "  daemon pid: $$!"
	@sleep 1
	@curl -sf http://127.0.0.1:7473/health > /dev/null && echo "  daemon healthy" || echo "  WARNING: daemon not responding (check /tmp/yokai-daemon.log)"

# Push binary to remote agents and restart them
dev-agents: build
	@for host in $(AGENTS); do \
		echo "deploying agent to $$host..."; \
		scp -q bin/$(BINARY_NAME) $$host:/tmp/yokai.new && \
		ssh $$host "chmod +x /tmp/yokai.new && sudo mv -f /tmp/yokai.new $(AGENT_PATH)" && \
		ssh $$host "sudo systemctl restart yokai-agent 2>/dev/null || (pkill -f '$(BINARY_NAME) agent' 2>/dev/null; sleep 0.5; setsid $(AGENT_PATH) agent $(AGENT_PORT) > /tmp/yokai-agent.log 2>&1 < /dev/null &)" && \
		sleep 1.5 && \
		ssh $$host "curl -sf http://127.0.0.1:$(AGENT_PORT)/health > /dev/null" && \
		echo "  $$host agent healthy" || \
		echo "  WARNING: $$host agent not responding (check /tmp/yokai-agent.log on $$host)"; \
	done

# Push binary to agents without restarting (for when you want to restart manually)
dev-push: build
	@for host in $(AGENTS); do \
		echo "pushing binary to $$host..."; \
		scp -q bin/$(BINARY_NAME) $$host:/tmp/yokai.new && \
		ssh $$host "chmod +x /tmp/yokai.new && sudo mv -f /tmp/yokai.new $(AGENT_PATH)" && \
		echo "  $$host done" || echo "  $$host FAILED"; \
	done

# Launch the TUI (foreground)
dev-tui: build
	./bin/$(BINARY_NAME)

# Tail daemon logs
dev-logs:
	@tail -f /tmp/yokai-daemon.log

# Tail agent logs on a remote host (usage: make dev-agent-logs HOST=finn)
HOST ?= finn
dev-agent-logs:
	@ssh $(HOST) "tail -f /tmp/yokai-agent.log"
