BINARY_NAME := yokai
VERSION := $(shell git describe --tags --always --dirty 2>/dev/null || echo "dev")
BUILD_TIME := $(shell date -u +"%Y-%m-%dT%H:%M:%SZ")
LDFLAGS := -ldflags "-s -w -X main.version=$(VERSION) -X main.buildTime=$(BUILD_TIME)"
GO_VERSION ?= 1.25
GO := $(shell if command -v mise >/dev/null 2>&1; then printf 'mise exec go@$(GO_VERSION) -- go'; elif command -v go >/dev/null 2>&1; then printf 'go'; fi)
GOLANGCI_LINT_VERSION ?= v2.4.0

# Remote agent hosts — space-separated list of ssh host aliases or user@host
# Override with: make dev-restart AGENTS="finn kyber"
AGENTS ?= finn
AGENT_PORT ?= 7474
AGENT_PATH ?= /usr/local/bin/yokai
TUI_DIR ?= ui/tui

.PHONY: build run clean uninstall test lint check agent daemon
.PHONY: dev dev-restart dev-daemon dev-agents dev-tui dev-legacy-tui dev-push
.PHONY: dev-opentui tui-install tui-build tui-dev tui-test

build:
	@test -n "$(GO)" || (echo "go is required; install Go or make sure mise is available" && exit 1)
	$(GO) build $(LDFLAGS) -o bin/$(BINARY_NAME) ./cmd/yokai

run: build
	./bin/$(BINARY_NAME)

agent: build
	./bin/$(BINARY_NAME) agent

daemon: build
	./bin/$(BINARY_NAME) daemon

clean: uninstall
	rm -rf bin/

uninstall:
	@echo "removing installed $(BINARY_NAME) binaries (if present)..."
	@rm -f "$$HOME/.local/bin/$(BINARY_NAME)"
	@if [ -w /usr/local/bin ]; then \
		rm -f /usr/local/bin/$(BINARY_NAME); \
	elif command -v sudo >/dev/null 2>&1 && sudo -n true >/dev/null 2>&1; then \
		sudo rm -f /usr/local/bin/$(BINARY_NAME); \
	elif [ -e /usr/local/bin/$(BINARY_NAME) ]; then \
		echo "  WARNING: /usr/local/bin/$(BINARY_NAME) exists but needs sudo to remove"; \
	fi

test:
	@test -n "$(GO)" || (echo "go is required; install Go or make sure mise is available" && exit 1)
	$(GO) test ./...

lint:
	@test -n "$(GO)" || (echo "go is required; install Go or make sure mise is available" && exit 1)
	$(GO) run github.com/golangci/golangci-lint/v2/cmd/golangci-lint@$(GOLANGCI_LINT_VERSION) run

check: test lint tui-test tui-build

tidy:
	@test -n "$(GO)" || (echo "go is required; install Go or make sure mise is available" && exit 1)
	$(GO) mod tidy

cross:
	@test -n "$(GO)" || (echo "go is required; install Go or make sure mise is available" && exit 1)
	GOOS=linux GOARCH=amd64 $(GO) build $(LDFLAGS) -o bin/$(BINARY_NAME)-linux-amd64 ./cmd/yokai
	GOOS=linux GOARCH=arm64 $(GO) build $(LDFLAGS) -o bin/$(BINARY_NAME)-linux-arm64 ./cmd/yokai
	GOOS=darwin GOARCH=arm64 $(GO) build $(LDFLAGS) -o bin/$(BINARY_NAME)-darwin-arm64 ./cmd/yokai

# ---------- Local dev workflow ----------

# Full local dev loop: rebuild, restart daemon, then launch the OpenTUI frontend
dev: build dev-daemon tui-dev
	@echo "skipped remote agent deploy; run 'make dev-agents' when you want to push to $(AGENTS)"

# Rebuild and restart the local daemon, but don't launch TUI
dev-restart: build dev-daemon
	@echo "daemon restarted locally — run 'make dev' or 'make tui-dev' when ready"
	@echo "skipped remote agent deploy; run 'make dev-agents' when you want to push to $(AGENTS)"

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

# Launch the legacy Bubble Tea TUI (foreground)
dev-legacy-tui: build
	./bin/$(BINARY_NAME)

# Backward-compatible alias during the frontend migration
dev-tui: dev-legacy-tui

# Launch the OpenTUI frontend against the local daemon
dev-opentui: dev

# Install OpenTUI frontend dependencies
tui-install:
	@command -v bun >/dev/null 2>&1 || (echo "bun is required for the OpenTUI frontend" && exit 1)
	@cd $(TUI_DIR) && bun install

# Build the OpenTUI frontend bundle
tui-build: tui-install
	@command -v bun >/dev/null 2>&1 || (echo "bun is required for the OpenTUI frontend" && exit 1)
	@cd $(TUI_DIR) && bun run build

# Run the OpenTUI frontend directly
tui-dev: tui-install
	@command -v bun >/dev/null 2>&1 || (echo "bun is required for the OpenTUI frontend" && exit 1)
	@cd $(TUI_DIR) && bun run dev

# Run frontend tests
tui-test: tui-install
	@command -v bun >/dev/null 2>&1 || (echo "bun is required for the OpenTUI frontend" && exit 1)
	@cd $(TUI_DIR) && bun test

# Tail daemon logs
dev-logs:
	@tail -f /tmp/yokai-daemon.log

# Tail agent logs on a remote host (usage: make dev-agent-logs HOST=finn)
HOST ?= finn
dev-agent-logs:
	@ssh $(HOST) "tail -f /tmp/yokai-agent.log"
