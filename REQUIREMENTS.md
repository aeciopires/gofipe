# Requirements

<!-- TOC -->

- [Requirements](#requirements)
- [Supported platforms](#supported-platforms)
- [Quick start](#quick-start)
- [What gets installed](#what-gets-installed)
- [Docker and Docker Compose (installed separately)](#docker-and-docker-compose-installed-separately)
  - [Ubuntu](#ubuntu)
  - [macOS](#macos)
- [mise](#mise)
  - [Activating mise in your shell](#activating-mise-in-your-shell)
  - [Updating tool versions](#updating-tool-versions)
- [Manual installation (without make)](#manual-installation-without-make)
- [Troubleshooting](#troubleshooting)

<!-- TOC -->

This repository uses [mise](https://mise.jdx.dev) to install and pin the versions of every development tool. The versions live in a single file: [mise.toml](mise.toml).

# Supported platforms

| Operating system | Architecture |
|------------------|--------------|
| Ubuntu 22.04 LTS | amd64 (x86_64) |
| Ubuntu 24.04 LTS | amd64 (x86_64) |
| Ubuntu 26.04 LTS | amd64 (x86_64) |
| macOS            | amd64 (Intel) and arm64 (Apple Silicon) |

Other systems may work, but they are not tested. To try anyway, export `SKIP_PLATFORM_CHECK=1` before running the commands below.

# Quick start

From the root of the repository:

```bash
# 1. Install Docker + Docker Compose (see the next sections) - not done by make
# 2. Install everything else (OS packages, mise and the pinned tools)
make install-deps

# 3. Verify that everything is installed (exits with an error if something is missing)
make check-deps
```

Both targets are also available inside `app/` and `helm-chart/` (they delegate to the root `Makefile`, which calls [scripts/deps.sh](scripts/deps.sh)).

`make install-deps` does the following:

1. Checks that the OS and architecture are supported.
2. **Ubuntu:** installs `ca-certificates curl git make tar gzip unzip` with `apt-get` (uses `sudo` when you are not root).
   **macOS:** checks the Xcode Command Line Tools (they provide `git` and `make`) and starts their installer if missing.
3. Installs `mise` into `~/.local/bin` using the official installer (`curl https://mise.run | sh`), if it is not installed yet.
4. Runs `mise trust` and `mise install` to install the tools pinned in [mise.toml](mise.toml).
5. Runs `make check-deps`.

# What gets installed

| Tool | Version | Used for |
|------|---------|----------|
| [Go](https://go.dev) | 1.27.4 | Building and testing `app/` |
| [golangci-lint](https://golangci-lint.run) | 2.15.0 | `make -C app lint` |
| [govulncheck](https://go.dev/doc/tutorial/govulncheck) | 1.9.0 | `make -C app vuln` |
| [Helm](https://helm.sh) | **4.3.2** | Deploying `helm-chart/` (**Helm 3 is no longer supported**) |
| [helm-docs](https://github.com/norwoodj/helm-docs) | 1.14.2 | `make -C helm-chart gen-docs` |
| [kubectl](https://kubernetes.io/docs/reference/kubectl/) | 1.37.2 | Talking to Kubernetes clusters |
| [Trivy](https://trivy.dev) | 0.76.2 | Scanning the Docker image (`make -C app scan`) |
| [hadolint](https://github.com/hadolint/hadolint) | 2.15.1 | Linting the `Dockerfile` |
| [ShellCheck](https://www.shellcheck.net) | 0.11.0 | Linting `scripts/*.sh` |

Docker images used by the project (pinned in `app/Dockerfile` and `app/docker-compose.yaml`): `golang:1.27.4-alpine`, `alpine:3.24.4`, `prom/prometheus:v3.16.0`, `grafana/grafana:13.3.1`, `grafana/loki:3.7.10`, `grafana/tempo:3.1.1`, `jaegertracing/jaeger:2.23.0`, `otel/opentelemetry-collector-contrib:0.165.0`.

# Docker and Docker Compose (installed separately)

Docker is **not** installed by `make install-deps` because it needs system-level configuration (daemon, groups, Docker Desktop license on macOS). `make check-deps` only checks it.

## Ubuntu

```bash
curl -fsSL https://get.docker.com -o get-docker.sh
sudo sh get-docker.sh   # installs Docker Engine, Buildx and the Compose plugin
# Use docker without sudo (log out and in again afterwards)
sudo usermod -aG docker "$USER"
```

Reference: https://docs.docker.com/engine/install/ubuntu/ and https://docs.docker.com/engine/install/linux-postinstall/

## macOS

Install [Docker Desktop](https://docs.docker.com/desktop/setup/install/mac-install/) (choose the Apple Silicon or Intel build), or an alternative such as [Colima](https://github.com/abiosoft/colima) or [OrbStack](https://orbstack.dev). Docker Desktop already includes Compose and Buildx.

# mise

[mise](https://mise.jdx.dev) is a polyglot version manager (a faster replacement for `asdf`). When you are inside this repository, mise reads [mise.toml](mise.toml) and puts the right version of each tool on your `PATH`.

> This project **replaced asdf with mise**. If you used the old instructions, you can remove the asdf plugins for go/helm/helm-docs/kubectl; mise does not need them.

## Activating mise in your shell

The Makefiles call the tools through `mise exec --`, so they work even without activation. To use `go`, `helm`, etc. directly in your terminal, add one line to your shell configuration and open a new terminal:

```bash
# bash (Ubuntu default)
echo 'eval "$(~/.local/bin/mise activate bash)"' >> ~/.bashrc

# zsh (macOS default)
echo 'eval "$(~/.local/bin/mise activate zsh)"' >> ~/.zshrc
```

Check it:

```bash
cd gofipe
mise ls --current   # lists the tools and versions of this repository
go version          # go1.27.4
helm version        # v4.3.2
```

## Updating tool versions

1. Find the latest version: `mise latest helm` (or `mise ls-remote helm`).
2. Edit [mise.toml](mise.toml).
3. Run `mise install` (or `make install-deps`).
4. If you changed Go, also update the `go` directive in `app/go.mod` and the `golang:` image in `app/Dockerfile`.
5. Update the tables in this file.

# Manual installation (without make)

```bash
# Ubuntu only
sudo apt-get update && sudo apt-get install -y ca-certificates curl git make tar gzip unzip

# Ubuntu and macOS
curl -fsSL https://mise.run | sh
export PATH="$HOME/.local/bin:$PATH"
cd gofipe
mise trust
mise install
```

# Troubleshooting

| Problem | Solution |
|---------|----------|
| `mise ERROR Config files in .../mise.toml are not trusted` | Run `mise trust` in the repository root. |
| `Helm 4 is required, found 'v3.x'` | A global Helm 3 is shadowing the pinned one. Run `mise install helm` and make sure mise is activated, or call `mise exec -- helm ...`. |
| `Unsupported Ubuntu version` | Only 22.04, 24.04 and 26.04 are supported. Use `SKIP_PLATFORM_CHECK=1 make install-deps` at your own risk. |
| `Docker daemon is not reachable` | Start Docker (Docker Desktop on macOS, `sudo systemctl start docker` on Ubuntu) and check that your user is in the `docker` group. |
| Rate limit / `429 Too Many Requests` from the FIPE API | Create a token at https://fipe.online and run the app with `FIPE_API_TOKEN=<token>`. |
