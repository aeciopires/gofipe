# Changelog

<!-- TOC -->

- [Changelog](#changelog)
- [v4.1.0](#v410)
- [v4.0.0](#v400)
- [v3.0.0](#v300)
- [v2.0.0](#v200)
- [v1.0.0](#v100)

<!-- TOC -->

# v4.1.0

Date: 11/16/2026

Observability:

- **JSON logs**: `log/slog` JSON handler on stdout (was text). Inside requests every line has top-level `trace_id`/`span_id`. New JSON access log per request (`msg="http request"`, `method`, `route`, `status` as a number, `duration_ms`); probes and `/metrics` are not logged.
- **Tracing with OpenTelemetry** (`telemetry.go`): server span per request named after the route (`otelhttp`) and a `FIPE GET` client span per FIPE call (`peer.service="fipe-api"`). Traces and logs are exported with OTLP/HTTP when `OTEL_EXPORTER_OTLP_ENDPOINT` (or the per-signal variants) is set; off by default. The OTLP log body is the same JSON line written to stdout.
- New metric `fipe_http_request_duration_seconds` (histogram by `path`, `method`, `code`) with `trace_id` **exemplars**; `/metrics` now uses the OpenMetrics format.
- **docker compose stack** with the latest versions, integrated: OpenTelemetry Collector contrib 0.165.0, Prometheus v3.16.0 (remote-write receiver + exemplar storage), Loki 3.7.10 (native OTLP), Tempo 3.1.1 (span metrics + service graph), Jaeger 2.23.0 and Grafana 13.3.1 (`grafana/grafana`, replaces the discontinued `grafana/grafana-oss` image). Configs in `app/observability/`; `app/prometheus.yml` moved to `app/observability/prometheus/prometheus.yml`. Host ports can be overridden (`GRAFANA_PORT`, `GOFIPE_PORT`, ...) and container names are no longer fixed (Compose project `gofipe`).
- **Grafana provisioning**: data sources (Prometheus, Loki, Tempo) with links metrics → traces (exemplars), logs → traces (`trace_id`), traces → logs and traces → metrics; dashboards loaded automatically: *gofipe - Overview* (new, home), *gofipe - Logs* (new), *gofipe - Traces* (new) and *gofipe - Business* (`dash-grafana.json`, rewritten: fixed data source, units, price range table). Jaeger is used through its own UI because Grafana's Jaeger plugin does not support Jaeger 2.21+.

Health checks:

- New `GET /health/live` (liveness) and `GET /health/ready` (readiness: `503 {"status": "unavailable"}` while starting and after `SIGTERM`). `GET /health` is kept for the Docker `HEALTHCHECK`.
- Graceful shutdown: on `SIGTERM` the app reports not ready, keeps serving for `SHUTDOWN_DELAY` (new variable, default `0s`), then shuts down and flushes telemetry. The listener is bound before the app reports ready; a second Ctrl+C exits immediately.
- Helm chart 2.1.0: `startupProbe` and `livenessProbe` on `/health/live`, `readinessProbe` on `/health/ready` with faster periods, new `shutdownDelay` (5s) and `terminationGracePeriodSeconds` (30) values, `OTEL_SERVICE_NAME` set automatically, OpenTelemetry example in `env`.

Other:

- New dependencies: `go.opentelemetry.io/otel` 1.47.0 (SDK, OTLP/HTTP trace and log exporters) and `go.opentelemetry.io/contrib/instrumentation/net/http/otelhttp` 0.72.0.
- Tests for readiness, `SHUTDOWN_DELAY` parsing, JSON logs with trace context, the OTLP JSON log handler, the access log and latency exemplars.
- `CLAUDE.md` requires the Modern Go Guidelines skill for every Go change, review and test.
- Documentation updated (`README.md`, `docs/`), including a rewritten observability chapter.

Trivy Report Summary

┌─────────────────────────────────────────┬──────────┬─────────────────┬─────────┐
│                 Target                  │   Type   │ Vulnerabilities │ Secrets │
├─────────────────────────────────────────┼──────────┼─────────────────┼─────────┤
│ aeciopires/gofipe:4.1.0 (alpine 3.24.4) │  alpine  │        0        │    -    │
├─────────────────────────────────────────┼──────────┼─────────────────┼─────────┤
│ app/gofipe                              │ gobinary │        0        │    -    │
└─────────────────────────────────────────┴──────────┴─────────────────┴─────────┘

# v4.0.0

Date: 11/16/2026

Upgrades:

- Go 1.25 -> 1.27.4 (`go.mod`, builder image `golang:1.27.4-alpine`), `prometheus/client_golang` 1.23.2 -> 1.25.0 and all indirect modules.
- Runtime image `alpine:3.23.3` -> `alpine:3.24.4`; docker-compose now pins `prom/prometheus:v3.16.0` and `grafana/grafana-oss:13.0.2`.
- Frontend: Bootstrap 5.3.0 -> 5.3.9 and Chart.js pinned to 4.6.1 (was unpinned), both with Subresource Integrity.
- Development tools are now managed by [mise](https://mise.jdx.dev) (`mise.toml`) instead of asdf. New `make check-deps` and `make install-deps` targets (Ubuntu 22.04/24.04/26.04 amd64 and macOS amd64/arm64).
- Helm chart requires Helm 4 (Helm 3 is no longer supported). Chart version 2.0.0.

Bug fixes:

- `/api/priceHistory` returned fabricated history: it guessed non-existent FIPE URLs and relabelled the current price with made-up months. It now uses the real FIPE v2 history endpoint (`/{type}/{codeFipe}/years/{yearId}/history`) with the real reference months.
- `fipe_price_min` and `fipe_price_max` were both set to the last observed price; they now hold the real minimum and maximum.
- The fuel acronym was never shown: FIPE returns `fuelAcronym`, not `acronymFuel`.
- Business metrics were incremented before the FIPE call (even on failures) and used client-supplied brand/model names as labels (unbounded cardinality). They now use the FIPE response and are recorded only on success.
- `fipe_http_requests_total` used the raw URL path as a label (unbounded cardinality); it now uses the matched route.
- Query parameters were concatenated into the upstream URL without validation (path/query injection). `type` and IDs are now validated (`400 Bad Request`).
- `/api/priceHistory?months=N` had no upper bound and could start thousands of goroutines/requests; it is now clamped to 1..24.
- Every unknown path rendered the home page; it now returns `404`. Non-GET methods return `405`.
- Template rendering errors were ignored; the `PORT` environment variable was ignored; cache entries never expired from memory.
- Upstream error messages (including internal URLs) were sent to the browser; they are now logged and a generic message is returned.
- Frontend: the chart of a previous vehicle stayed visible after selecting another one; errors loading models/years were unhandled promise rejections; query parameters were not URL-encoded.
- Dockerfile: the `licensce` label typo was fixed.
- Helm chart: `pdb.yaml` referenced a non-existent template (`go-fipe.selectorLabels`) and an undefined `maxUnavailable`; `NOTES.txt` used the release name as namespace; `env` was referenced but not defined in `values.yaml`; `appVersion` did not match the image tag.
- Makefile: `make image` failed on the second run (`mkdir /tmp/caches`, existing buildx builder), sent the password on the command line and built the image twice; `make build` updated dependencies and ran the binary.

Improvements:

- Code split into `main.go`, `handlers.go`, `fipe.go`, `cache.go`, `metrics.go`, following the [Modern Go Guidelines](https://github.com/JetBrains/go-modern-guidelines) (method-aware `ServeMux` patterns, `cmp.Or`, `errors.AsType`, `encoding/json/v2`, `slices`/`maps`, built-in `min`/`max`, `log/slog`).
- Frontend assets embedded in the binary with `go:embed`.
- HTTP server timeouts and graceful shutdown on `SIGINT`/`SIGTERM`; request contexts propagated to FIPE calls; shared HTTP client; upstream body size limit.
- Price and history responses are cached (1h / 6h); expired cache entries are purged every 10 minutes.
- Optional `FIPE_API_TOKEN` (sent as `X-Subscription-Token`) and `FIPE_BASE_URL` environment variables.
- Unit tests with a fake FIPE server (`make test`), golangci-lint config, govulncheck, hadolint and shellcheck.
- Docker image runs as non-root (UID 10001), has a `HEALTHCHECK`, smaller build context (`.dockerignore`), cached module downloads and the app version injected via `-ldflags`.
- Helm chart: secure defaults (`runAsNonRoot`, `readOnlyRootFilesystem`, drop all capabilities, `seccompProfile`), `automountServiceAccountToken: false`, configurable `containerPort`/`env`/`nodePort`/`loadBalancerIP`, removed support for Kubernetes < 1.29 API versions, `make dry-run` uses server-side validation.
- New beginner-friendly documentation in `docs/` with analogies and Mermaid diagrams.

Trivy Report Summary

┌─────────────────────────────────────────┬──────────┬─────────────────┬─────────┐
│                 Target                  │   Type   │ Vulnerabilities │ Secrets │
├─────────────────────────────────────────┼──────────┼─────────────────┼─────────┤
│ aeciopires/gofipe:4.0.0 (alpine 3.24.4) │  alpine  │        0        │    -    │
├─────────────────────────────────────────┼──────────┼─────────────────┼─────────┤
│ app/gofipe                              │ gobinary │        0        │    -    │
└─────────────────────────────────────────┴──────────┴─────────────────┴─────────┘

# v3.0.0

Date: 02/13/2026

- Fixed bug related to default port of container in dockerfile
- Bump Alpine image from 3.23.2 to 3.23.2
- Fixed bug in helm chart
- Change version in docker-compose file
- Changed helm values
- Updated `CHANGELOG.md` to document new features.
- Updated `README.md` to remove instructions about change port of application.

Trivy Report Summary

┌─────────────────────────────────────────┬──────────┬─────────────────┬─────────┐
│                 Target                  │   Type   │ Vulnerabilities │ Secrets │
├─────────────────────────────────────────┼──────────┼─────────────────┼─────────┤
│ aeciopires/gofipe:3.0.0 (alpine 3.23.3) │  alpine  │        0        │    -    │
├─────────────────────────────────────────┼──────────┼─────────────────┼─────────┤
│ app/gofipe                              │ gobinary │        0        │    -    │
└─────────────────────────────────────────┴──────────┴─────────────────┴─────────┘

# v2.0.0

Date: 12/30/2025

- Split frontend assets into `static/css/style.css` and `static/js/app.js` (clean separation of concerns).
- Implemented backend in-memory caching for brands, models and years to reduce repeated external FIPE requests.
- Added `/api/priceHistory` endpoint and frontend history UI to display price history (default 12 months).
- Added concurrent fetching helper and used parallel requests where appropriate.
- Improved error handling for external requests and clearer HTTP status codes.
- Added Prometheus metrics: `fipe_price_min`, `fipe_price_max`, `fipe_fuel_count`, `fipe_brand_search_count`.
- Updated Grafana dashboard (`app/dashboard/dash-grafana.json`) to include new metrics panels.
- Added day/night theme support and modernized colors and layout.
- Updated `README.md` to document new features.

Trivy Report Summary

┌─────────────────────────────────────────┬──────────┬─────────────────┬─────────┐
│                 Target                  │   Type   │ Vulnerabilities │ Secrets │
├─────────────────────────────────────────┼──────────┼─────────────────┼─────────┤
│ aeciopires/gofipe:2.0.0 (alpine 3.23.2) │  alpine  │        0        │    -    │
├─────────────────────────────────────────┼──────────┼─────────────────┼─────────┤
│ app/gofipe                              │ gobinary │        0        │    -    │
└─────────────────────────────────────────┴──────────┴─────────────────┴─────────┘

# v1.0.0

Date: 12/29/2025

- Initial version of the application.

Trivy Report Summary

┌─────────────────────────────────────────┬──────────┬─────────────────┬─────────┐
│                 Target                  │   Type   │ Vulnerabilities │ Secrets │
├─────────────────────────────────────────┼──────────┼─────────────────┼─────────┤
│ aeciopires/gofipe:1.0.0 (alpine 3.23.2) │  alpine  │        0        │    -    │
├─────────────────────────────────────────┼──────────┼─────────────────┼─────────┤
│ app/gofipe                              │ gobinary │        0        │    -    │
└─────────────────────────────────────────┴──────────┴─────────────────┴─────────┘