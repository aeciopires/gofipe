# Makefile for the gofipe Helm chart. Requires Helm 4 (Helm 3 is no longer supported).
# helm, helm-docs and kubectl are pinned in ../mise.toml.
# Compatible with GNU Make 3.81+ (the version shipped with macOS).

SHELL := /bin/bash
.DEFAULT_GOAL := help

RELEASE   ?= gofipe
NAMESPACE ?= gofipe

# Override with MISE_EXEC= to use the tools already on your PATH.
MISE_EXEC ?= mise exec --
HELM      := $(MISE_EXEC) helm
HELM_DOCS := $(MISE_EXEC) helm-docs

.PHONY: help check-deps install-deps check-helm lint template dry-run install uninstall package gen-docs

help: ## Show this help
	@echo "Usage: make <target> [RELEASE=gofipe] [NAMESPACE=gofipe]"
	@echo
	@grep -E '^[a-zA-Z_-]+:.*?## ' $(MAKEFILE_LIST) | awk 'BEGIN {FS = ":.*?## "}; {printf "  \033[36m%-14s\033[0m %s\n", $$1, $$2}'

check-deps: ## Check that every required software is installed
	@$(MAKE) --no-print-directory -C .. check-deps

install-deps: ## Install the required software (except Docker)
	@$(MAKE) --no-print-directory -C .. install-deps

check-helm:
	@version="$$($(HELM) version --short 2>/dev/null)"; \
	case "$$version" in \
	  v4.*) ;; \
	  *) echo "[ERROR] Helm 4 is required, found '$${version:-none}'. Run: make install-deps"; exit 1 ;; \
	esac

lint: check-helm ## Lint the chart (strict mode)
	$(HELM) lint --strict .

template: check-helm ## Render the manifests locally
	$(HELM) template $(RELEASE) . --namespace $(NAMESPACE)

dry-run: check-helm ## Simulate an install against the current cluster (server-side validation)
	$(HELM) upgrade --install $(RELEASE) . \
		--namespace $(NAMESPACE) \
		--create-namespace \
		--dry-run=server \
		--debug

install: check-helm ## Install/upgrade the chart in the current Kubernetes cluster
	$(HELM) upgrade --install $(RELEASE) . \
		--namespace $(NAMESPACE) \
		--create-namespace \
		--wait \
		--timeout 5m

uninstall: check-helm ## Uninstall the chart from the current Kubernetes cluster
	$(HELM) uninstall $(RELEASE) --namespace $(NAMESPACE)

package: check-helm ## Package the chart into packages/
	$(HELM) package . -d packages

gen-docs: ## Regenerate README.md from values.yaml and README.md.gotmpl
	$(HELM_DOCS)
