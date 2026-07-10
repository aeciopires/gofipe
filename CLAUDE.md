# CLAUDE.md

This file provides guidance to Claude Code (claude.ai/code) when working with code in this repository.

## Repository layout

This repo has two independent, top-level components — treat them separately:

- `app/` — the Go web application (single-binary Go server + vanilla JS/HTML frontend).
- `helm-chart/` — a Helm chart to deploy `app/`'s Docker image to Kubernetes.

There is no top-level build; each directory has its own `Makefile`.

## `app/` — Go application

### Commands

All commands run from `app/`.

```bash
cd app

# Run locally (no Docker)
go run main.go            # serves on :8080

# Build (also updates deps first, matching CI expectations)
make build                 # go get -u && go mod download && go build -o bin/gofipe && runs it
make clean                 # remove bin/gofipe

# Docker image lifecycle (interactive: prompts for Docker Hub creds, then buildx + trivy scan)
make image                 # multi-arch build+push (linux/amd64,arm/v6,arm/v7,arm64) + trivy scan
make up / make down / make logs / make recreate   # run/stop/tail/recreate the container locally
```

There are no automated tests in this repo (no `*_test.go` files) and no lint config — do not assume a test/lint command exists.

`make image` bumps a manually-maintained `VERSION` variable at the top of `app/Makefile`. When cutting a release, update `VERSION` there, the `version`/`date_create` labels in `app/Dockerfile`, the image tag in `app/docker-compose.yaml`, and `app/CHANGELOG.md` together — they are not derived from a single source of truth.

### Architecture

`app/main.go` is the entire backend — a single-file Go server (no internal packages) implementing a **BFF (Backend for Frontend)**:

- Serves the server-rendered frontend (`templates/index.html`) and static assets (`static/js`, `static/css`) via `http.ServeMux`.
- Proxies all vehicle data lookups to the public FIPE API (`https://fipe.parallelum.com.br/api/v2`, constant `FipeBaseURL`) rather than talking to FIPE directly from the browser. Endpoints: `/api/brands`, `/api/models`, `/api/years`, `/api/price`, `/api/priceHistory` (see `handleBrands`/`handleModels`/`handleYears`/`handlePrice`/`handlePriceHistory`).
- `/api/priceHistory` has no native FIPE history endpoint to rely on cleanly: it first tries a direct `/history` path, and if that fails, falls back to concurrently querying several *guessed* candidate URL shapes per past month (query-param and path variants) and normalizing whatever comes back into a synthesized `{"history": [...]}` payload with fabricated `referenceMonth` labels. Treat this handler as best-effort/heuristic, not a stable contract — read it fully before modifying.
- A simple in-memory TTL cache (`cacheStore`, guarded by `cacheMutex`) avoids hammering the external API: brands/models cached 12h, years 24h. Cache keys are plain strings like `"brands:cars"` or `"years:type:brandId:modelId"` — keep new cached endpoints consistent with that convention.
- Prometheus instrumentation lives at the top of `main.go` (`httpRequestsCounter`, `vehicleSearchCounter`, `minPriceGauge`/`maxPriceGauge`, `fuelTypeCounter`, `brandSearchCounter`), registered in `init()` and exposed at `/metrics`. `handlePrice` is the only handler that increments the business metrics (search/brand/price/fuel) — it does so by parsing the FIPE price string via `parseFipePrice` (handles both `.`/`,` as decimal separators). If you add a new business-relevant endpoint, follow this same pattern: record the HTTP counter via `recordHTTPRequest`, then any business metrics after a successful parse.
- `/health` returns a static `{"status": "ok"}` for k8s/Docker probes — the same path is wired into the Helm chart's readiness/liveness probes and Docker healthchecks, so don't change its response shape without updating `helm-chart/values.yaml`.

Frontend (`templates/index.html` + `static/js/app.js` + `static/css/style.css`) is plain server-rendered HTML with vanilla JS calling the `/api/*` endpoints above — no frontend build step or framework.

Local observability stack: `docker-compose.yaml` wires up `gofipe` + `prometheus` (config in `prometheus.yml`) + `grafana` (import dashboard from `dashboard/dash-grafana.json`) for manual testing of metrics end-to-end.

## `helm-chart/` — Kubernetes deployment

### Commands

All commands run from `helm-chart/` and shell out to Docker-pinned `helm`/`helm-docs` images (no local helm installation required):

```bash
cd helm-chart
make lint         # helm lint .
make template     # helm template gofipe . --namespace gofipe
make dry-run      # helm install --dry-run --debug
make install      # helm upgrade --install gofipe . --namespace gofipe --create-namespace
make uninstall    # helm uninstall gofipe --namespace gofipe
make package      # package chart into packages/
make gen-docs     # regenerate README.md from values.yaml + README.md.gotmpl via helm-docs — do not hand-edit README.md's generated tables
```

### Structure notes

- `helm-chart/README.md` is generated by `helm-docs` from `values.yaml` comments (`# --` doc comments) and `README.md.gotmpl`. After changing `values.yaml`, run `make gen-docs` rather than editing `README.md` directly.
- Chart version (`Chart.yaml` `version`) and app version (`Chart.yaml` `appVersion` / `values.yaml` `image.tag`) are versioned independently — bump both deliberately, they track chart changes vs. app releases respectively.
- Cloud-specific optional resources are gated by values flags rather than separate charts: `ingress.createGcpManagedCertificate` / `createCertManagerIssuer` (see `templates/gcp-certificate.yaml`, `templates/cert-manager-issuer.yaml`), `service.createGcpBackendAndFrontendConfig` (see `templates/gcp-backend-frontend-config.yaml`). Ingress annotation examples for AWS ALB / nginx / GCP are documented inline (commented out) in `values.yaml`.
- `serviceMonitor.enabled` (Prometheus Operator) is off by default; when enabled it scrapes `/metrics` per `values.yaml`'s `serviceMonitor.path`.

## Contribution workflow

See `CONTRIBUTING.md` for the fork/branch/PR flow. Relevant when making changes:
- Update `app/CHANGELOG.md` for app changes and `app/Makefile`'s `VERSION` together (see CONTRIBUTING.md's "Build image" reference).
- `REQUIREMENTS.md` documents local toolchain setup (asdf, Go 1.25+, Helm 3.19, helm-docs, kubectl) if environment setup is ever needed — not needed for routine code edits.
