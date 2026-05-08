#!/bin/sh
set -e

# yokai installer
# Usage: curl -fsSL https://raw.githubusercontent.com/spencerbull/Yokai/main/install.sh | sh

REPO="spencerbull/Yokai"
INSTALL_DIR="/usr/local/bin"
FALLBACK_INSTALL_DIR="$HOME/.local/bin"
BINARY="yokai"
TUI_BINARY="yokai-tui"
PROJECT_NAME="Yokai"

# Colors
RED='\033[0;31m'
GREEN='\033[0;32m'
YELLOW='\033[1;33m'
CYAN='\033[0;36m'
MAGENTA='\033[0;35m'
BOLD='\033[1m'
DIM='\033[2m'
NC='\033[0m'

banner() {
  printf "${MAGENTA}"
  cat << 'EOF'

  ██    ██  ██████  ██   ██  █████  ██
  ██    ██ ██    ██ ██  ██  ██   ██ ██
   ██████  ██    ██ █████   ███████ ██
        ██ ██    ██ ██  ██  ██   ██ ██
        ██  ██████  ██   ██ ██   ██ ██

EOF
  printf "${DIM}  GPU Fleet Management for Hackers${NC}\n\n"
}

info()    { printf "  ${CYAN}▸${NC} %s\n" "$1"; }
success() { printf "  ${GREEN}✔${NC} %s\n" "$1"; }
warn()    { printf "  ${YELLOW}!${NC} %s\n" "$1"; }
fail()    { printf "  ${RED}✘${NC} %s\n" "$1"; exit 1; }
step()    { printf "\n${BOLD}  %s${NC}\n" "$1"; }

banner

# ── Detect platform ──────────────────────────────────────────────
step "Detecting platform"

OS=$(uname -s | tr '[:upper:]' '[:lower:]')
ARCH=$(uname -m)

case "$ARCH" in
  x86_64|amd64) ARCH="amd64" ;;
  aarch64|arm64) ARCH="arm64" ;;
  *) fail "Unsupported architecture: $ARCH" ;;
esac

case "$OS" in
  linux|darwin) ;;
  *) fail "Unsupported OS: $OS" ;;
esac

success "Platform: ${OS}/${ARCH}"

# ── Fetch latest version ────────────────────────────────────────
step "Fetching latest release"

LATEST_URL=$(curl -fsSLI -o /dev/null -w '%{url_effective}' "https://github.com/$REPO/releases/latest")
VERSION=$(printf '%s\n' "$LATEST_URL" | sed -nE 's#.*/tag/v([^/]+)$#\1#p')

if [ -z "$VERSION" ]; then
  fail "Could not determine latest version. Check your internet connection."
fi

success "Latest version: v${VERSION}"

# ── Download ─────────────────────────────────────────────────────
step "Downloading"

FILENAME="${PROJECT_NAME}_${VERSION}_${OS}_${ARCH}.tar.gz"
URL="https://github.com/$REPO/releases/download/v${VERSION}/${FILENAME}"

TMP_DIR=$(mktemp -d)
trap 'rm -rf "$TMP_DIR"' EXIT

info "From: ${URL}"
curl -fsSL "$URL" -o "${TMP_DIR}/${FILENAME}" || fail "Download failed. Does v${VERSION} have a ${OS}/${ARCH} release?"
tar -xzf "${TMP_DIR}/${FILENAME}" -C "$TMP_DIR"
success "Downloaded and extracted"

# Locate binaries inside the extracted archive (GoReleaser wraps in a directory)
YOKAI_SRC=$(find "$TMP_DIR" -name "$BINARY" -type f | head -n 1)
TUI_SRC=$(find "$TMP_DIR" -name "$TUI_BINARY" -type f | head -n 1)

if [ -z "$YOKAI_SRC" ]; then
  fail "Could not find '${BINARY}' in the downloaded archive."
fi

# ── Install ──────────────────────────────────────────────────────
step "Installing"

USE_SUDO=0
if [ -w "$INSTALL_DIR" ]; then
  TARGET_DIR="$INSTALL_DIR"
elif command -v sudo >/dev/null 2>&1 && sudo -n true >/dev/null 2>&1; then
  TARGET_DIR="$INSTALL_DIR"
  USE_SUDO=1
else
  TARGET_DIR="$FALLBACK_INSTALL_DIR"
fi

install_bin() {
  _src="$1"
  _dst="$2"
  if [ "$USE_SUDO" -eq 1 ]; then
    sudo mv "$_src" "$_dst"
    sudo chmod +x "$_dst"
  else
    mv "$_src" "$_dst"
    chmod +x "$_dst"
  fi
}

if [ "$TARGET_DIR" = "$FALLBACK_INSTALL_DIR" ]; then
  mkdir -p "$TARGET_DIR"
fi

install_bin "$YOKAI_SRC" "${TARGET_DIR}/${BINARY}"
if [ -n "$TUI_SRC" ]; then
  install_bin "$TUI_SRC" "${TARGET_DIR}/${TUI_BINARY}"
fi

if [ "$TARGET_DIR" = "$FALLBACK_INSTALL_DIR" ]; then
  # Add to PATH in shell rc files
  PATH_ADDED=0
  for rc_file in "$HOME/.bashrc" "$HOME/.zshrc" "$HOME/.profile"; do
    if [ ! -f "$rc_file" ]; then
      continue
    fi
    if ! grep -Fq '.local/bin' "$rc_file"; then
      printf '\n# Added by yokai installer\nexport PATH="$HOME/.local/bin:$PATH"\n' >> "$rc_file"
      PATH_ADDED=1
    fi
  done

  success "Installed to ${TARGET_DIR}/${BINARY}"
  if [ "$PATH_ADDED" -eq 1 ]; then
    info "Added ~/.local/bin to PATH in shell config"
    warn "Run 'source ~/.bashrc' or restart your shell to use yokai"
  fi
else
  success "Installed to ${TARGET_DIR}/${BINARY}"
fi

# ── Verify ───────────────────────────────────────────────────────
step "Verifying"

export PATH="$TARGET_DIR:$PATH"
if command -v yokai >/dev/null 2>&1; then
  INSTALLED_VERSION=$(yokai version 2>/dev/null || echo "unknown")
  success "Verified: ${INSTALLED_VERSION}"
else
  warn "${TARGET_DIR} is not in your PATH"
  info "Run: export PATH=\"${TARGET_DIR}:\$PATH\""
fi

# ── Restart daemon ───────────────────────────────────────────────
# When a user reinstalls / upgrades yokai, any already-running `yokai
# daemon` process is still executing the OLD binary and will happily
# serve 404s for any route introduced in the new version (the TUI then
# appears broken for no obvious reason). Stop it cleanly and restart it
# so the installed version is what's serving /hf, /deploy, etc.
step "Restarting daemon"

find_daemon_pids() {
  # Match processes whose argv starts with `yokai daemon` (bare or path-prefixed
  # binary name), not arbitrary commands like `grep yokai daemon` or
  # `man yokai daemon` that merely mention the string.
  if command -v pgrep >/dev/null 2>&1; then
    pgrep -f '(^|/)yokai[[:space:]]+daemon([[:space:]]|$)' 2>/dev/null || true
  else
    # BusyBox / minimal images may lack pgrep; fall back to ps+awk. The
    # `[y]okai` trick keeps awk's own argv from matching itself.
    ps -eo pid,args 2>/dev/null \
      | awk '/(^|[[:space:]]|[/])[y]okai[[:space:]]+daemon([[:space:]]|$)/ {print $1}' \
      || true
  fi
}

DAEMON_PIDS=$(find_daemon_pids)

if [ -n "$DAEMON_PIDS" ]; then
  info "Stopping old daemon (PIDs: $(printf '%s' "$DAEMON_PIDS" | tr '\n' ' '))"
  for pid in $DAEMON_PIDS; do
    kill "$pid" 2>/dev/null || true
  done

  # Wait up to ~3s for graceful exit before escalating.
  waited=0
  while [ "$waited" -lt 15 ]; do
    still_running=""
    for pid in $DAEMON_PIDS; do
      if kill -0 "$pid" 2>/dev/null; then
        still_running="$pid"
        break
      fi
    done
    [ -z "$still_running" ] && break
    sleep 0.2
    waited=$((waited + 1))
  done

  for pid in $DAEMON_PIDS; do
    if kill -0 "$pid" 2>/dev/null; then
      warn "Daemon PID $pid did not exit cleanly; sending SIGKILL"
      kill -9 "$pid" 2>/dev/null || true
    fi
  done
  success "Old daemon stopped"

  # Start the freshly installed binary. Running as root (typically via
  # sudo curl | sudo sh) is the one case where auto-restart is risky —
  # the daemon would end up owned by root with a root-owned config dir,
  # which breaks the user's subsequent `yokai` invocations. Leave the
  # restart to them in that case.
  if [ "$(id -u)" -eq 0 ] && [ -n "${SUDO_USER:-}" ]; then
    warn "Running as root; not auto-restarting the daemon."
    info "As ${SUDO_USER}, run:  yokai daemon &   (or just  yokai  to auto-start)"
  else
    DAEMON_LOG_DIR="${XDG_CONFIG_HOME:-$HOME/.config}/yokai"
    mkdir -p "$DAEMON_LOG_DIR" 2>/dev/null || true
    DAEMON_LOG="$DAEMON_LOG_DIR/daemon.log"

    # nohup + & detaches the daemon so it survives this install script
    # exiting. We don't use `disown` because dash (the typical /bin/sh)
    # doesn't implement it.
    (nohup "${TARGET_DIR}/${BINARY}" daemon >>"$DAEMON_LOG" 2>&1 &) >/dev/null 2>&1

    # Give the daemon a moment to bind its port.
    sleep 1

    if [ -n "$(find_daemon_pids)" ]; then
      success "New daemon started (logs: ${DAEMON_LOG})"
    else
      warn "Daemon did not start cleanly — run 'yokai daemon &' manually, or launch 'yokai'"
    fi
  fi
else
  info "No running daemon detected — nothing to restart"
fi

# ── Done ─────────────────────────────────────────────────────────
printf "\n${GREEN}${BOLD}  ⚡ yokai v${VERSION} is ready!${NC}\n\n"
printf "${DIM}  Quick start:${NC}\n"
printf "    ${CYAN}yokai agent 7474${NC}      ${DIM}# Start an agent on a GPU node${NC}\n"
printf "    ${CYAN}yokai daemon${NC}           ${DIM}# Start the daemon on your workstation${NC}\n"
printf "    ${CYAN}yokai${NC}                  ${DIM}# Launch OpenTUI (auto-starts daemon)${NC}\n"
printf "\n${DIM}  Docs: https://github.com/${REPO}${NC}\n\n"
