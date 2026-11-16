# CLAUDE.md

This file provides guidance to Claude Code (claude.ai/code) when working with code in this repository.

## Repository layout

This repo has two independent, top-level components — treat them separately:

- `app/` — the Go web application (single-binary Go server + vanilla JS/HTML frontend).
- `helm-chart/` — a Helm 4 chart to deploy `app/`'s Docker image to Kubernetes.

Shared tooling at the root:

- `mise.toml` — the single source of truth for tool versions (Go, golangci-lint, govulncheck, Helm 4, helm-docs, kubectl, trivy, hadolint, shellcheck). Docker/Docker Compose are installed separately and are not in mise.
- `Makefile` — `check-deps`, `install-deps` (both call `scripts/deps.sh`), plus aggregate `lint` and `test`.
- `scripts/deps.sh` — supports Ubuntu 22.04/24.04/26.04 (amd64) and macOS (amd64/arm64). Keep it Bash 3.2 compatible (macOS default shell) and shellcheck-clean.
- `docs/` — beginner-oriented docs (analogies + Mermaid diagrams). Update them when behaviour/architecture changes.

All Makefiles must stay compatible with GNU Make 3.81 (macOS): no `.ONESHELL`, no `.SHELLFLAGS`; multi-line recipes use `\` continuations. Tools are invoked through `$(MISE_EXEC)` (`mise exec --`, overridable with `MISE_EXEC=`).

## `app/` — Go application

### Commands

All commands run from `app/` (`make help` lists everything).

```bash
cd app
make run            # go run . — serves on :8080 (PORT env var overrides)
make build          # bin/gofipe with -ldflags "-X main.appVersion=$(VERSION)"
make test           # go test -race ./...
make lint           # golangci-lint (config: .golangci.yml) + hadolint Dockerfile
make vuln           # govulncheck
make update-deps    # go get -u ./... && go mod tidy
make image-local scan   # local docker build + trivy
make image          # interactive Docker Hub login, multi-arch buildx push, trivy scan
make up / down / logs / recreate / compose-up / compose-down
```

Tests live in `app/main_test.go` and use an `httptest` fake FIPE server (via `config.FipeBaseURL`) — they never hit the internet. Add tests there for new handlers.

When cutting a release, update together: `VERSION` in `app/Makefile`, `version`/`date_create` labels in `app/Dockerfile`, image tag + `VERSION` build arg in `app/docker-compose.yaml`, `app/CHANGELOG.md`, and `helm-chart/values.yaml` `image.tag` / `Chart.yaml` `appVersion`. They are not derived from a single source of truth.

### Go conventions

- Go version is 1.27 (`go.mod`, `mise.toml`, `golang:` image in `Dockerfile` must match).
- **Always use the `modern-go-guidelines:use-modern-go` skill (JetBrains Modern Go Guidelines, https://github.com/JetBrains/go-modern-guidelines) whenever creating, modifying, refactoring, reviewing/analyzing or testing Go code** in this repo. Before touching a `.go` file, run the skill's `list` subcommand for that file (Go version is resolved from `app/go.mod`), read the full output, and apply every guideline that fits; call `explain <id>` before deciding to skip a relevant one. Treat its output as authoritative over older patterns in nearby code. The same applies to reviews: flag code that misses an applicable guideline.
- Idioms already in use: method-aware `ServeMux` patterns, `cmp.Or`, `errors.AsType`, `encoding/json/v2` for new JSON code, built-in `min`/`max`, `slices`/`maps`, `log/slog`, `slog.NewMultiHandler`, typed atomics (`atomic.Bool`), `wg.Go`, `t.Context()`.
- Logging: JSON `slog` only, created by `setupTelemetry`. Inside request handling log with the `*Context` variants (`slog.WarnContext(r.Context(), ...)`) so `trace_id`/`span_id` are added; keep field names stable (`msg`, `route`, `status` as a number, `duration_ms`) because Loki dashboards/queries parse them with `| json`.
- `make lint` must report 0 issues. gosec false positives are annotated inline with a justification.

### Architecture

Single `package main` split by responsibility, implementing a **BFF (Backend for Frontend)**:

- `main.go` — `config` from env (`PORT`, `FIPE_BASE_URL`, `FIPE_API_TOKEN`, `SHUTDOWN_DELAY`), `newServer`, `run` (calls `setupTelemetry`, binds the listener, sets `ready`; on SIGINT/SIGTERM sets `ready=false`, sleeps `SHUTDOWN_DELAY`, `Shutdown` with 15s timeout, flushes telemetry). `templates/` and `static/` are embedded with `//go:embed` — the binary is self-contained, the Dockerfile copies only the binary.
- `handlers.go` — `server.routes()` registers `GET /{$}`, `GET /static/` (no dir listing), `GET /health` + `GET /health/live` (liveness, static `{"status": "ok"}`), `GET /health/ready` (readiness: 503 `{"status": "unavailable"}` while `server.ready` is false — never checks FIPE), `GET /metrics` (OpenMetrics, needed for exemplars), `GET /api/brands|models|years|price|priceHistory`, wrapped as `traceHandler(instrument(mux))`. Input is validated (`vehicleTypes`, `idPattern`) before building upstream paths — never interpolate unvalidated query values into FIPE URLs. `writeError` maps errors to 400/404/429/502/504 and never leaks upstream details.
- `fipe.go` — `fipeClient.get(ctx, path)` (shared client, 10s timeout, `X-Subscription-Token` when a token is set, 5 MiB body limit), `*upstreamError`, `PriceResponse` (FIPE uses `fuelAcronym`), `parseFipePrice` (handles `1.234,56`, `1,234.56`, `71.229`).
- `cache.go` — `ttlCache` with `get`/`set`/`purgeExpired` and a janitor goroutine. Keys are plain strings like `"brands:cars"`, `"years:cars:59:5940"`, `"price:..."`, `"history:type:codeFipe:yearId"` — keep new cached endpoints consistent. TTLs: brands/models 12h, years 24h, price 1h, history 6h.
- `/api/priceHistory` first fetches the current price (to get `codeFipe`) and then the real FIPE v2 endpoint `/{type}/{codeFipe}/years/{yearId}/history` (`priceHistory` array). Free tier returns ~3 months; a token may return more. On failure it falls back to the current price as a single entry. Response shape `{"history":[PriceResponse...]}` newest first; `months` clamped to 1..24.
- `telemetry.go` — `setupTelemetry` (JSON slog to stdout always; when `OTEL_EXPORTER_OTLP_ENDPOINT`/`_TRACES_`/`_LOGS_ENDPOINT` is set and `OTEL_SDK_DISABLED!=true`: OTLP/HTTP tracer + logger providers, `slog.NewMultiHandler(stdout, otlpJSONHandler)` — the OTLP log body is the same JSON line), `traceContextHandler` (top-level `trace_id`/`span_id`), `traceHandler` (`otelhttp` server spans named `METHOD /route`, skips `/metrics` and `/health*`), `tracedTransport` (`FIPE GET` client spans with `peer.service="fipe-api"`). Tests never enable exporters.
- `metrics.go` — Prometheus metrics (`fipe_http_requests_total`, `fipe_http_request_duration_seconds` histogram with `trace_id` exemplars, `fipe_search_stats`, `fipe_price_min`/`fipe_price_max`, `fipe_fuel_count`, `fipe_brand_search_count`), `instrument()` middleware (labels = matched route from `r.Pattern`, unknown methods → `OTHER`; also writes the JSON access log `msg="http request"` except for `/metrics` and `/health*`) and `priceTracker` (real min/max). Only `handlePrice` records business metrics, after a successful lookup, using brand/model **from the FIPE response** (never client input) to bound cardinality. Follow this pattern for new business endpoints.
- Health endpoints: `/health` is used by the Docker `HEALTHCHECK`; the Helm chart uses `/health/live` (startup + liveness) and `/health/ready` (readiness). Don't change paths or shapes without updating `app/Dockerfile` and `helm-chart/values.yaml`.

Frontend (`templates/index.html` + `static/js/app.js` + `static/css/style.css`) is plain HTML with vanilla JS calling `/api/*` — no build step. CDN assets (Bootstrap, Chart.js) are version-pinned with SRI hashes; update both when bumping.

Docker image: multi-stage, `golang:1.27.4-alpine` builder cross-compiling for `linux/amd64,arm/v6,arm/v7,arm64`, `alpine:3.24.4` runtime, non-root UID 10001, `HEALTHCHECK`.

Local observability stack (`app/docker-compose.yaml`, project name `gofipe`, host ports overridable via `GOFIPE_PORT`, `GRAFANA_PORT`, `PROMETHEUS_PORT`, `JAEGER_PORT`, `LOKI_PORT`, `TEMPO_PORT`, `OTLP_GRPC_PORT`, `OTLP_HTTP_PORT`): gofipe → OTel Collector contrib (`observability/otel-collector/config.yaml`, exporters `otlp_grpc/tempo`, `otlp_grpc/jaeger`, `otlp_http/loki`) → Tempo (`observability/tempo/tempo.yaml`, metrics generator remote-writes span metrics/service graph to Prometheus), Jaeger (default all-in-one config) and Loki (`observability/loki/config.yaml`, native OTLP). Prometheus (`observability/prometheus/prometheus.yml`, flags `--web.enable-remote-write-receiver --enable-feature=exemplar-storage`) scrapes gofipe and the stack. Grafana provisioning in `observability/grafana/provisioning/` (data source UIDs `prometheus`, `loki`, `tempo` with exemplar/derived-field/traces-to-logs links) loads every JSON in `dashboard/` (`gofipe-overview.json` home, `gofipe-logs.json`, `gofipe-traces.json`, `dash-grafana.json` = business). Jaeger is intentionally **not** a Grafana data source: Grafana's Jaeger plugin needs Jaeger's legacy `/api/*`, removed in Jaeger 2.21+. When bumping these images, run the stack and check each service's logs (config keys change between versions) and that every dashboard panel returns data.

## `helm-chart/` — Kubernetes deployment

### Commands

All commands run from `helm-chart/` and use the Helm 4 / helm-docs pinned in `mise.toml` (Helm 3 is not supported; `check-helm` enforces v4):

```bash
cd helm-chart
make lint         # helm lint --strict .
make template     # helm template gofipe . --namespace gofipe
make dry-run      # helm upgrade --install --dry-run=server --debug (needs a cluster)
make install      # helm upgrade --install gofipe . --namespace gofipe --create-namespace --wait
make uninstall    # helm uninstall gofipe --namespace gofipe
make package      # package chart into packages/
make gen-docs     # regenerate README.md from values.yaml + README.md.gotmpl via helm-docs — do not hand-edit README.md's generated tables
```

`RELEASE=` and `NAMESPACE=` override the defaults.

### Structure notes

- `helm-chart/README.md` is generated by `helm-docs` from `values.yaml` comments (`# --` doc comments) and `README.md.gotmpl`. After changing `values.yaml`, run `make gen-docs` rather than editing `README.md` directly.
- Chart version (`Chart.yaml` `version`) and app version (`Chart.yaml` `appVersion` / `values.yaml` `image.tag`) are versioned independently — bump both deliberately.
- `kubeVersion: >=1.29.0-0`; templates only use current APIs (`networking.k8s.io/v1`, `autoscaling/v2`, `policy/v1`).
- Secure defaults: pod runs as UID 10001, `readOnlyRootFilesystem`, all capabilities dropped, `automountServiceAccountToken: false`. The container gets `PORT` from `containerPort`; extra variables (e.g. `FIPE_API_TOKEN` from a Secret) go in `env`.
- Probes: `startupProbe` and `livenessProbe` → `/health/live`, `readinessProbe` → `/health/ready`. `shutdownDelay` (default `5s`) is passed as `SHUTDOWN_DELAY` and must stay below `terminationGracePeriodSeconds` minus ~15s. The deployment always sets `PORT`, `SHUTDOWN_DELAY` and `OTEL_SERVICE_NAME`; tracing is enabled by adding `OTEL_EXPORTER_OTLP_ENDPOINT` to `env`.
- Cloud-specific optional resources are gated by values flags: `ingress.createGcpManagedCertificate` / `createCertManagerIssuer` (`templates/gcp-certificate.yaml`, `templates/cert-manager-issuer.yaml`), `service.createGcpBackendAndFrontendConfig` (`templates/gcp-backend-frontend-config.yaml`). Ingress annotation examples for AWS ALB / nginx / GCP are commented in `values.yaml`.
- `serviceMonitor.enabled` (Prometheus Operator) and `podDisruptionBudget.enabled` are off by default.

## Contribution workflow

See `CONTRIBUTING.md` for the fork/branch/PR flow. Before committing run `make lint` and `make test` from the root. Update `app/CHANGELOG.md` and `app/Makefile`'s `VERSION` together for app changes. `REQUIREMENTS.md` documents environment setup with mise.
