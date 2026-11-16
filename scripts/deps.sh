#!/usr/bin/env bash
#
# Check or install the development dependencies of the gofipe repository.
#
# Usage:
#   scripts/deps.sh check     # verify every dependency (non-zero exit on failure)
#   scripts/deps.sh install   # install OS packages + mise + the tools pinned in mise.toml
#
# Supported platforms:
#   - Ubuntu 22.04, 24.04 and 26.04 (amd64)
#   - macOS (amd64 / Intel and arm64 / Apple Silicon)
#
# Docker and Docker Compose are NOT installed by this script (install them
# separately, see REQUIREMENTS.md); they are only checked.
#
# Environment variables:
#   SKIP_PLATFORM_CHECK=1   run on an unsupported OS/arch at your own risk
#
# NOTE: keep this script compatible with Bash 3.2 (the default on macOS).

set -euo pipefail

REPO_ROOT="$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)"
SUPPORTED_UBUNTU_VERSIONS="22.04 24.04 26.04"
UBUNTU_PACKAGES="ca-certificates curl git make tar gzip unzip"
MISE_BIN_DIR="${HOME}/.local/bin"

# "binary|description" of every tool pinned in mise.toml
MISE_TOOLS="go|Go toolchain
golangci-lint|Go linters
govulncheck|Go vulnerability scanner
helm|Helm 4
helm-docs|Helm chart docs generator
kubectl|Kubernetes CLI
trivy|Image vulnerability scanner
hadolint|Dockerfile linter
shellcheck|Shell script linter"

# ---------------------------------------------------------------------------
# Output helpers
# ---------------------------------------------------------------------------
if [ -t 1 ]; then
  C_RED=$'\033[31m'; C_GREEN=$'\033[32m'; C_YELLOW=$'\033[33m'; C_BOLD=$'\033[1m'; C_RESET=$'\033[0m'
else
  C_RED=""; C_GREEN=""; C_YELLOW=""; C_BOLD=""; C_RESET=""
fi

info()    { printf '%s==>%s %s\n' "${C_BOLD}" "${C_RESET}" "$*"; }
ok()      { printf '  %s[OK]%s      %s\n' "${C_GREEN}" "${C_RESET}" "$*"; }
warn()    { printf '  %s[WARN]%s    %s\n' "${C_YELLOW}" "${C_RESET}" "$*"; }
missing() { printf '  %s[MISSING]%s %s\n' "${C_RED}" "${C_RESET}" "$*"; FAILURES=$((FAILURES + 1)); }
die()     { printf '%s[ERROR]%s %s\n' "${C_RED}" "${C_RESET}" "$*" >&2; exit 1; }

FAILURES=0

# Prefer the mise binary installed by this script when it is not on PATH yet.
export PATH="${MISE_BIN_DIR}:${PATH}"

# ---------------------------------------------------------------------------
# Platform detection
# ---------------------------------------------------------------------------
OS_NAME=""
detect_platform() {
  local kernel arch
  kernel="$(uname -s)"
  arch="$(uname -m)"

  case "${kernel}" in
    Linux)
      [ -r /etc/os-release ] || die "Cannot read /etc/os-release to detect the Linux distribution."
      # shellcheck disable=SC1091
      . /etc/os-release
      OS_NAME="ubuntu"
      if [ "${ID:-}" != "ubuntu" ]; then
        platform_error "Unsupported Linux distribution '${ID:-unknown}'. Supported: Ubuntu ${SUPPORTED_UBUNTU_VERSIONS}."
      elif ! echo " ${SUPPORTED_UBUNTU_VERSIONS} " | grep -q " ${VERSION_ID:-none} "; then
        platform_error "Unsupported Ubuntu version '${VERSION_ID:-unknown}'. Supported: ${SUPPORTED_UBUNTU_VERSIONS}."
      fi
      if [ "${arch}" != "x86_64" ]; then
        platform_error "Unsupported architecture '${arch}' on Ubuntu. Supported: amd64 (x86_64)."
      fi
      ok "Platform: Ubuntu ${VERSION_ID:-?} (${arch})"
      ;;
    Darwin)
      OS_NAME="macos"
      case "${arch}" in
        x86_64|arm64) ok "Platform: macOS $(sw_vers -productVersion 2>/dev/null || echo '?') (${arch})" ;;
        *) platform_error "Unsupported macOS architecture '${arch}'. Supported: amd64 (x86_64), arm64." ;;
      esac
      ;;
    *)
      platform_error "Unsupported operating system '${kernel}'. Supported: Ubuntu ${SUPPORTED_UBUNTU_VERSIONS} (amd64) and macOS (amd64/arm64)."
      ;;
  esac
}

platform_error() {
  if [ "${SKIP_PLATFORM_CHECK:-0}" = "1" ]; then
    warn "$1 (continuing because SKIP_PLATFORM_CHECK=1)"
  else
    die "$1 Set SKIP_PLATFORM_CHECK=1 to try anyway."
  fi
}

# ---------------------------------------------------------------------------
# Checks
# ---------------------------------------------------------------------------
check_command() {
  # check_command <binary> <description> <install hint>
  if command -v "$1" >/dev/null 2>&1; then
    ok "$2 ($1): $(command -v "$1")"
  else
    missing "$2 ($1). $3"
  fi
}

check_docker() {
  if command -v docker >/dev/null 2>&1; then
    ok "Docker: $(docker --version 2>/dev/null)"
    if ! docker info >/dev/null 2>&1; then
      warn "Docker daemon is not reachable (is it running? is your user in the 'docker' group?)."
    fi
  else
    missing "Docker. Install it separately: https://docs.docker.com/engine/install/"
  fi

  if docker compose version >/dev/null 2>&1; then
    ok "Docker Compose: $(docker compose version --short 2>/dev/null)"
  elif command -v docker-compose >/dev/null 2>&1; then
    ok "Docker Compose (standalone): $(docker-compose version --short 2>/dev/null)"
  else
    missing "Docker Compose. Install it separately: https://docs.docker.com/compose/install/"
  fi

  if docker buildx version >/dev/null 2>&1; then
    ok "Docker Buildx: $(docker buildx version 2>/dev/null | awk '{print $2}')"
  else
    warn "Docker Buildx not found: required only by 'make image' (multi-arch builds)."
  fi
}

check_mise_tools() {
  if ! command -v mise >/dev/null 2>&1; then
    missing "mise. Run 'make install-deps' (or see https://mise.jdx.dev/getting-started.html)."
    return
  fi
  ok "mise: $(mise --version 2>/dev/null | awk '{print $1}')"

  cd "${REPO_ROOT}"
  # 'mise trust --show' prints paths with $HOME abbreviated as '~'.
  local short_root="${REPO_ROOT/#${HOME}/\~}"
  if ! mise trust --show 2>/dev/null | grep -Eq "^(${REPO_ROOT}|${short_root})(/mise.toml)?: trusted"; then
    warn "mise.toml is not trusted yet. Run: mise trust ${REPO_ROOT}/mise.toml"
  fi

  local line bin desc path
  while IFS= read -r line; do
    bin="${line%%|*}"
    desc="${line#*|}"
    if path="$(mise which "${bin}" 2>/dev/null)" && [ -n "${path}" ]; then
      ok "${desc} (${bin}): $(tool_version "${bin}")"
    else
      missing "${desc} (${bin}). Run 'make install-deps' (or: mise install)."
    fi
  done <<EOF
${MISE_TOOLS}
EOF

  # Helm 3 is no longer supported by this repository.
  if mise which helm >/dev/null 2>&1; then
    local helm_version
    helm_version="$(mise exec -- helm version --short 2>/dev/null || true)"
    case "${helm_version}" in
      v4.*) ;;
      *) missing "Helm 4 is required, found '${helm_version:-unknown}'. Run: mise install helm" ;;
    esac
  fi
}

tool_version() {
  case "$1" in
    go)            mise exec -- go version 2>/dev/null | awk '{print $3}' ;;
    golangci-lint) mise exec -- golangci-lint version --short 2>/dev/null ;;
    govulncheck)   mise exec -- govulncheck -version 2>/dev/null | awk '/Scanner:/{print $2}' ;;
    helm)          mise exec -- helm version --short 2>/dev/null ;;
    helm-docs)     mise exec -- helm-docs --version 2>/dev/null | awk '{print $NF}' ;;
    kubectl)       mise exec -- kubectl version --client 2>/dev/null | awk '/Client Version/{print $3}' ;;
    trivy)         mise exec -- trivy --version 2>/dev/null | awk '/^Version/{print $2}' ;;
    hadolint)      mise exec -- hadolint --version 2>/dev/null | awk '{print $NF}' ;;
    shellcheck)    mise exec -- shellcheck --version 2>/dev/null | awk '/^version:/{print $2}' ;;
  esac
}

cmd_check() {
  info "Checking platform"
  detect_platform

  info "Checking base packages"
  check_command git  "Git"  "Install it with 'make install-deps'."
  check_command curl "cURL" "Install it with 'make install-deps'."
  check_command make "GNU Make" "Install it with 'make install-deps'."
  check_command tar  "tar"  "Install it with 'make install-deps'."

  info "Checking Docker (installed separately)"
  check_docker

  info "Checking tools managed by mise (${REPO_ROOT}/mise.toml)"
  check_mise_tools

  echo
  if [ "${FAILURES}" -gt 0 ]; then
    die "${FAILURES} dependency problem(s) found. Run 'make install-deps' to fix the tools managed by this repository; Docker and Docker Compose must be installed separately (see REQUIREMENTS.md)."
  fi
  info "${C_GREEN}All dependencies are installed.${C_RESET}"
}

# ---------------------------------------------------------------------------
# Installation
# ---------------------------------------------------------------------------
as_root() {
  if [ "$(id -u)" -eq 0 ]; then
    "$@"
  elif command -v sudo >/dev/null 2>&1; then
    sudo "$@"
  else
    die "This step needs root privileges and 'sudo' is not available: $*"
  fi
}

install_os_packages() {
  case "${OS_NAME}" in
    ubuntu)
      info "Installing base packages with apt: ${UBUNTU_PACKAGES}"
      as_root apt-get update -y
      # shellcheck disable=SC2086 # word splitting is intended
      DEBIAN_FRONTEND=noninteractive as_root apt-get install -y --no-install-recommends ${UBUNTU_PACKAGES}
      ;;
    macos)
      info "Checking Xcode Command Line Tools (provides git and make)"
      if ! xcode-select -p >/dev/null 2>&1; then
        warn "Xcode Command Line Tools not found. Starting the installer..."
        xcode-select --install || true
        die "Finish the Xcode Command Line Tools installation and run 'make install-deps' again."
      fi
      ok "Xcode Command Line Tools: $(xcode-select -p)"
      ;;
  esac
}

install_mise() {
  if command -v mise >/dev/null 2>&1; then
    ok "mise already installed: $(command -v mise)"
    return
  fi
  info "Installing mise into ${MISE_BIN_DIR} (https://mise.jdx.dev)"
  curl -fsSL https://mise.run | sh
  command -v mise >/dev/null 2>&1 || die "mise installation failed."
  ok "mise installed: $(command -v mise)"
}

install_mise_tools() {
  info "Installing the tools pinned in ${REPO_ROOT}/mise.toml"
  cd "${REPO_ROOT}"
  mise trust "${REPO_ROOT}/mise.toml"
  mise install
}

print_activation_hint() {
  local shell_name rc_file
  shell_name="$(basename "${SHELL:-bash}")"
  case "${shell_name}" in
    zsh)  rc_file="${HOME}/.zshrc" ;;
    fish) rc_file="${HOME}/.config/fish/config.fish" ;;
    *)    rc_file="${HOME}/.bashrc" ;;
  esac
  cat <<EOF

${C_BOLD}Next step:${C_RESET} activate mise in your shell so the pinned tools are on PATH
when you are inside this repository. Add this line to ${rc_file} and open a new terminal:

    eval "\$(${MISE_BIN_DIR}/mise activate ${shell_name})"

(The Makefiles already find the tools through mise even without activation.)
EOF
}

cmd_install() {
  info "Checking platform"
  detect_platform
  install_os_packages
  install_mise
  install_mise_tools
  print_activation_hint
  echo
  cmd_check
}

# ---------------------------------------------------------------------------
# Main
# ---------------------------------------------------------------------------
case "${1:-}" in
  check)   cmd_check ;;
  install) cmd_install ;;
  *)
    echo "Usage: $0 {check|install}" >&2
    exit 2
    ;;
esac
