# Top-level Makefile: manages the developer toolchain shared by app/ and helm-chart/.
# Each component also has its own Makefile (cd app && make help, cd helm-chart && make help).
#
# Compatible with GNU Make 3.81+ (the version shipped with macOS).

SHELL := /bin/bash
.DEFAULT_GOAL := help

.PHONY: help check-deps install-deps lint test

help: ## Show this help
	@echo "Usage: make <target>"
	@echo
	@grep -E '^[a-zA-Z_-]+:.*?## ' $(MAKEFILE_LIST) | awk 'BEGIN {FS = ":.*?## "}; {printf "  \033[36m%-14s\033[0m %s\n", $$1, $$2}'
	@echo
	@echo "Component targets: 'make -C app help' and 'make -C helm-chart help'"

check-deps: ## Check that every required software is installed (Docker included)
	@./scripts/deps.sh check

install-deps: ## Install OS packages, mise and the tools pinned in mise.toml (not Docker)
	@./scripts/deps.sh install

lint: ## Lint Go code, Dockerfile, shell scripts and the Helm chart
	$(MAKE) -C app lint
	$(MAKE) -C helm-chart lint
	mise exec -- shellcheck scripts/*.sh

test: ## Run Go tests and render the Helm chart
	$(MAKE) -C app test
	$(MAKE) -C helm-chart template > /dev/null
