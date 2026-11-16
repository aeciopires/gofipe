#---------------------------
# VARIABLES
#---------------------------
# Compatible with GNU Make 3.81+ (the version shipped with macOS).

# Change the value according to new releases
VERSION := 4.1.0

APP_NAME := gofipe
BIN_FILE := bin/$(APP_NAME)
PATH_DOCKERFILE := ./Dockerfile
DOCKER_HUB_ACCOUNT ?= aeciopires
IMAGE := $(DOCKER_HUB_ACCOUNT)/$(APP_NAME)

# /bin/bash exists on both Ubuntu and macOS
SHELL := /bin/bash
.DEFAULT_GOAL := help

# Tools (go, golangci-lint, trivy...) are pinned in ../mise.toml.
# Override with MISE_EXEC= to use the tools already on your PATH.
MISE_EXEC ?= mise exec --

# Platforms supported. Reference: https://docs.docker.com/build/building/multi-platform/
SUPPORTED_PLATFORMS := linux/amd64,linux/arm/v6,linux/arm/v7,linux/arm64
BUILDER_NAME := multi-platform-builder

LDFLAGS := -s -w -X main.appVersion=$(VERSION)

# References about docker buildx:
# https://docs.docker.com/build/building/multi-platform/
# https://docs.docker.com/reference/cli/docker/buildx/build

.PHONY: help check-deps install-deps requirements run build test cover lint vuln fmt \
        update-deps clean image image-local scan up down logs recreate compose-up compose-down

#---------------------------
# HELP / DEPENDENCIES
#---------------------------

help: ## Show this help
	@echo "Usage: make <target>"
	@echo
	@grep -E '^[a-zA-Z_-]+:.*?## ' $(MAKEFILE_LIST) | awk 'BEGIN {FS = ":.*?## "}; {printf "  \033[36m%-14s\033[0m %s\n", $$1, $$2}'

check-deps: ## Check that every required software is installed
	@$(MAKE) --no-print-directory -C .. check-deps

install-deps: ## Install the required software (except Docker)
	@$(MAKE) --no-print-directory -C .. install-deps

# Kept for backward compatibility
requirements: check-deps

#---------------------------
# GO
#---------------------------

run: ## Run the app locally on http://localhost:8080 (PORT env var overrides)
	$(MISE_EXEC) go run .

build: ## Build the binary into bin/gofipe
	CGO_ENABLED=0 $(MISE_EXEC) go build -trimpath -ldflags "$(LDFLAGS)" -o $(BIN_FILE) .

test: ## Run unit tests with the race detector
	$(MISE_EXEC) go test -race -count=1 ./...

cover: ## Run tests and show coverage per function
	$(MISE_EXEC) go test -count=1 -coverprofile=bin/coverage.out ./...
	$(MISE_EXEC) go tool cover -func=bin/coverage.out

lint: ## Run golangci-lint and hadolint
	$(MISE_EXEC) golangci-lint run ./...
	$(MISE_EXEC) hadolint $(PATH_DOCKERFILE)

vuln: ## Scan Go dependencies and stdlib for known vulnerabilities
	$(MISE_EXEC) govulncheck ./...

fmt: ## Format Go code
	$(MISE_EXEC) golangci-lint fmt ./...

update-deps: ## Update Go module dependencies to their latest versions
	$(MISE_EXEC) go get -u ./...
	$(MISE_EXEC) go mod tidy

clean: ## Remove build artifacts
	rm -rf bin/

#---------------------------
# DOCKER IMAGE
#---------------------------

image: check-deps ## Build and push the multi-arch image to Docker Hub, then scan it with trivy
	@test -f $(PATH_DOCKERFILE) || { echo "[ERROR] File not found: $(PATH_DOCKERFILE)"; exit 1; }
	@set -e; \
	read -rp 'Username of Docker Hub: ' user; \
	read -rsp 'Password/token of Docker Hub: ' pass; echo; \
	printf '%s' "$$pass" | docker login -u "$$user" --password-stdin; \
	docker buildx inspect $(BUILDER_NAME) >/dev/null 2>&1 || \
	  docker buildx create --name $(BUILDER_NAME) --platform="$(SUPPORTED_PLATFORMS)"; \
	docker buildx build --builder $(BUILDER_NAME) --push \
	  --platform="$(SUPPORTED_PLATFORMS)" \
	  --build-arg VERSION=$(VERSION) \
	  -t "$$user/$(APP_NAME):$(VERSION)" \
	  -t "$$user/$(APP_NAME):latest" \
	  -f $(PATH_DOCKERFILE) .; \
	$(MISE_EXEC) trivy image "$$user/$(APP_NAME):$(VERSION)"

image-local: ## Build the image for the local platform only (no push)
	docker build --build-arg VERSION=$(VERSION) -t $(IMAGE):$(VERSION) -f $(PATH_DOCKERFILE) .

scan: ## Scan the local image with trivy (run image-local first)
	$(MISE_EXEC) trivy image --severity HIGH,CRITICAL $(IMAGE):$(VERSION)

#---------------------------
# RUN CONTAINERS
#---------------------------

up: ## Run the published image in background and follow the logs
	docker run -d -p 8080:8080 --rm --name $(APP_NAME) "$(IMAGE):$(VERSION)"
	$(MAKE) logs

down: ## Stop the container
	docker stop $(APP_NAME)

logs: ## Follow the container logs
	docker logs -f $(APP_NAME)

recreate: ## Recreate the container
	-$(MAKE) down
	$(MAKE) up

compose-up: ## Start gofipe + OTel Collector, Prometheus, Loki, Tempo, Jaeger, Grafana (Docker Compose)
	docker compose up -d --build

compose-down: ## Stop the Docker Compose stack
	docker compose down
