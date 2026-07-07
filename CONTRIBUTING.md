# Contributing to yokai

Thanks for your interest in contributing to yokai. This guide covers everything you need to get started.

---

## Development Setup

### Prerequisites

| Tool | Version | Purpose |
|---|---|---|
| Go | 1.22+ | Build and test |
| Make | any | Build automation |
| golangci-lint | latest | Linting (optional for local dev, required in CI) |
| Docker | 20.10+ | Only needed if testing agent/container features locally |

### Clone and Build

```bash
git clone https://github.com/spencerbull/yokai.git
cd yokai
make build
```

The binary is written to `./bin/yokai`.

### Running Locally

```bash
# Launch the TUI
make run

# Run the agent (on the current machine, for testing)
make agent

# Run the daemon
make daemon
```

### Running Tests

```bash
# Run all tests
make test

# Run tests with verbose output
go test -v ./...

# Run tests for a specific package
go test -v ./internal/config/...

# Run with race detector
go test -race ./...

# Run short tests only (skips integration tests requiring Docker/GPU)
go test -short ./...
```

### Linting

```bash
# Install golangci-lint (if not already installed)
# https://golangci-lint.run/welcome/install/

# Run linter
make lint
```

---

## Project Structure

```
internal/
├── agent/          # Runs on target devices. REST API, Docker CLI wrapper, system metrics.
├── config/         # Config file management. Load, save, migrate, defaults.
├── daemon/         # Runs locally. SSH tunnel pool, metrics aggregation, command forwarding.
├── docker/         # Docker Hub/GHCR tag fetching with cache. Compose generation for monitoring.
├── hf/             # HuggingFace API client. Model search, GGUF file listing, token validation.
├── ssh/            # SSH connection, SCP upload, bootstrap (preflight + agent deploy).
├── tailscale/      # Tailscale CLI wrapper for peer discovery.
├── tui/            # Bubbletea application shell and view router.
│   ├── components/ # Reusable rendering widgets (not Bubbletea models, just Render() functions).
│   ├── theme/      # Tokyo Night color palette and lipgloss styles.
│   └── views/      # Individual TUI screens. Each implements the View interface.
├── upgrade/        # Self-update mechanism. Downloads from GitHub Releases.
└── vscode/         # VS Code settings.json read/merge/write with backup.
```

### Key Concepts

- **Three-tier architecture**: TUI (view layer) -> Daemon (local orchestrator) -> Agent (remote worker)
- **View interface**: All TUI screens implement `Init()`, `Update()`, `View()`, `KeyBinds()`
- **Stack-based navigation**: Views are pushed/popped via `NavigateMsg` and `PopViewMsg`
- **Config portability**: All state in `~/.config/yokai/config.json`, no database
- **CLI-based Docker**: Agent uses `os/exec` to call Docker CLI, not the Docker SDK, to keep dependencies minimal

---

## Code Style

### Go Conventions

- Run `gofmt` (or let your editor handle it). All code must be `gofmt`-clean.
- Follow [Effective Go](https://go.dev/doc/effective_go) and the [Go Code Review Comments](https://go.dev/wiki/CodeReviewComments).
- Exported functions and types must have doc comments.
- Error messages should be lowercase, no trailing punctuation: `return fmt.Errorf("connecting to device: %w", err)`
- Use `%w` for error wrapping.

### Naming

- Packages: short, lowercase, singular (`config`, `agent`, `ssh`)
- Interfaces: `-er` suffix when it makes sense (`View`, not `IView`)
- Test files: `*_test.go` in the same package

### Imports

Group imports in this order, separated by blank lines:

```go
import (
    "fmt"                    // stdlib
    "net/http"

    "github.com/pelletier/go-toml/v2"         // third-party

    "github.com/spencerbull/yokai/internal/config"  // internal
)
```

---

## Testing

### Writing Tests

- Use `t.Run()` for subtests and table-driven tests.
- Use `t.Parallel()` where safe (no shared file I/O or global state).
- Use `t.TempDir()` for file system tests.
- Use `t.Setenv()` for environment variable tests.
- Use `httptest.NewServer()` to mock external APIs (Docker Hub, GHCR, HuggingFace).
- Mark tests requiring Docker or GPU hardware with `testing.Short()`:

```go
func TestContainerDeploy(t *testing.T) {
    if testing.Short() {
        t.Skip("skipping integration test in short mode")
    }
    // ...
}
```

### Test Coverage

Tests exist for all core packages:

| Package | Test file | What's tested |
|---|---|---|
| `config` | `config_test.go` | Load, save, roundtrip, device CRUD, migration, defaults |
| `agent` | `server_test.go` | HTTP endpoints, auth middleware, error handling |
| `agent` | `metrics_test.go` | Metrics collection, JSON serialization, graceful fallback |
| `agent` | `docker_test.go` | Container name sanitization, Docker operations |
| `daemon` | `types_test.go` | JSON roundtrip for deploy request/result types |
| `docker` | `catalog_test.go` | Tag fetching, caching, Docker Hub/GHCR API mocking |
| `docker` | `compose_test.go` | Compose generation with/without GPU, Prometheus config |
| `hf` | `client_test.go` | Model search, GGUF listing, token validation |
| `ssh` | `bootstrap_test.go` | Preflight result structure, systemd unit validation |
| `vscode` | `settings_test.go` | Settings path detection, endpoint add/remove, backup |
| `tui/components` | `metricsbar_test.go` | Rendering at various widths and percentages |
| `tui/components` | `sparkline_test.go` | Rendering with various data patterns |
| `tui/views` | `messages_test.go` | Navigation command generation |

### CI Checks

Every pull request runs:

1. **Build** -- `go build ./...` across Go 1.22 and 1.23
2. **Test** -- `go test -v -race -coverprofile=coverage.out ./...`
3. **Lint** -- `make lint`

All three must pass before merge.

---

## Pull Request Process

### Before Submitting

1. **Branch from `main`**: Create a feature branch with a descriptive name.
   ```bash
   git checkout -b feat/add-amd-gpu-support
   git checkout -b fix/dashboard-resize-crash
   ```

2. **Make your changes**: Keep commits focused. One logical change per commit.

3. **Run checks locally**:
   ```bash
   make test
   make lint
   go build ./...
   ```

4. **Write or update tests**: New features should include tests. Bug fixes should include a regression test where practical.

### Submitting

1. Push your branch and open a pull request against `main`.
2. Fill in the PR description:
   - **What** changed and **why**
   - Any breaking changes or migration notes
   - How to test the change manually (if applicable)
3. CI will run automatically. Fix any failures before requesting review.

### Review

- PRs require at least one approving review.
- Address review feedback with new commits (don't force-push during review).
- Once approved and CI is green, the PR will be merged.

### Commit Messages

Use clear, descriptive commit messages:

```
feat: add AMD ROCm GPU metrics support

Add rocm-smi parser alongside existing nvidia-smi support.
Detect GPU vendor during preflight and use the appropriate
metrics collector.
```

Prefix with a type when helpful:
- `feat:` new feature
- `fix:` bug fix
- `refactor:` code restructuring with no behavior change
- `test:` adding or updating tests
- `docs:` documentation changes
- `ci:` CI/CD changes

---

## Reporting Issues

### Bug Reports

Include the following:

- **yokai version**: output of `yokai version`
- **OS and architecture**: `uname -a`
- **Steps to reproduce**: what you did
- **Expected behavior**: what you expected to happen
- **Actual behavior**: what actually happened
- **Logs or error output**: copy-paste any relevant terminal output

### Feature Requests

Describe:
- **The problem** you're trying to solve
- **Your proposed solution** (if you have one)
- **Alternatives** you've considered

---

## Release Process

Releases are automated via [GoReleaser](https://goreleaser.com/) and GitHub Actions.

1. A maintainer tags a new version: `git tag v1.2.3 && git push --tags`
2. The `release.yml` workflow triggers automatically
3. GoReleaser cross-compiles binaries for all supported platforms
4. A GitHub Release is created with:
   - `yokai_<version>_linux_amd64.tar.gz`
   - `yokai_<version>_linux_arm64.tar.gz`
   - `yokai_<version>_darwin_arm64.tar.gz`
   - `checksums.txt`
5. Users can update with `yokai upgrade` or re-run the install script

Versioning follows [Semantic Versioning](https://semver.org/):
- **MAJOR**: breaking changes to CLI, config schema, or agent API
- **MINOR**: new features, backward-compatible
- **PATCH**: bug fixes, backward-compatible
