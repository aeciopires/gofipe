# Glossary

| Term | Meaning |
|------|---------|
| **API** | A way for programs to talk to each other. FIPE's API answers questions such as "list the brands" with JSON. |
| **BFF (Backend for Frontend)** | A backend made specifically to serve one frontend. It hides other services from the browser. |
| **Cache** | A temporary copy of data kept close by, so it can be reused quickly. |
| **Cardinality** | The number of different label combinations of a metric. High cardinality makes Prometheus slow and memory-hungry. |
| **CDN** | Content Delivery Network: servers around the world that deliver static files (here, Bootstrap and Chart.js). |
| **Container** | A running instance of a Docker image, isolated from the rest of the system. |
| **Context (Go)** | A value passed through function calls that carries deadlines and cancellation signals. |
| **Counter / Gauge** | Prometheus metric types. A counter only increases; a gauge can go up and down. |
| **Endpoint** | A URL the server answers, such as `/api/brands`. |
| **FIPE** | Fundação Instituto de Pesquisas Econômicas, which publishes the reference prices of vehicles in Brazil. |
| **FIPE code** | Identifier of a vehicle version in the FIPE table, e.g. `005340-6`. |
| **Goroutine** | A lightweight task that runs concurrently with others in a Go program. |
| **Graceful shutdown** | Stopping a server only after the requests in progress have finished. |
| **Handler** | The Go function that answers one endpoint. |
| **Exemplar** | One real example attached to a metric bucket, e.g. "this 120 ms request had trace_id abc…". Links metrics to traces. |
| **Health check / probe** | A periodic request that tells Docker/Kubernetes how the app is. *Liveness* (`/health/live`): is it alive? *Readiness* (`/health/ready`): can it take traffic? *Startup*: has it finished starting? |
| **Histogram** | A Prometheus metric that counts observations in buckets (≤ 10 ms, ≤ 50 ms...), used to compute percentiles. |
| **Jaeger** | An open-source tracing backend with its own web UI. |
| **JSON logs** | Logs written as one JSON object per line, so tools can read each field (`level`, `route`, `trace_id`...). |
| **Helm / Helm chart** | Package manager for Kubernetes / the package itself. This project requires **Helm 4**. |
| **HTTP status code** | A number in each answer: 200 OK, 400 Bad Request, 404 Not Found, 502 Bad Gateway... |
| **Image (Docker)** | A packaged, read-only template with the program and everything it needs to run. |
| **JSON** | A text format for data, e.g. `{"code":"59","name":"VW - VolksWagen"}`. |
| **Kubernetes** | A system that runs and manages containers on a cluster of machines. |
| **Label (Prometheus)** | A key/value pair that splits a metric into series, e.g. `brand_name="Fiat"`. |
| **LogQL** | Loki's query language, e.g. `{service_name="gofipe"} \| json \| level="ERROR"`. |
| **Loki** | Grafana's log database. It indexes only labels and stores log lines compressed. |
| **Middleware** | Code that wraps a handler to do something before/after it (here: counting requests). |
| **mise** | A tool that installs and switches the exact versions of the development tools listed in `mise.toml`. |
| **Mutex** | A lock that lets only one goroutine change shared data at a time. |
| **OpenTelemetry (OTel)** | The vendor-neutral standard (APIs, SDKs and the Collector) for traces, metrics and logs. |
| **OTLP** | The OpenTelemetry protocol used to send telemetry (ports 4317 gRPC / 4318 HTTP). |
| **OTel Collector** | A service that receives telemetry and forwards it to one or more backends. |
| **p95 (percentile)** | 95 % of the requests were faster than this value. |
| **PromQL** | Prometheus' query language. |
| **Rate limit** | The maximum number of requests an API accepts in a period of time. |
| **Router / ServeMux** | The component that picks which handler answers a URL. |
| **SIGTERM** | The signal an operating system (or Docker/Kubernetes) sends to ask a program to stop. |
| **Span** | One step of a trace (e.g. "call FIPE"), with start time, duration and attributes. |
| **Structured metadata (Loki)** | Extra key/values stored with a log line without being an index label (e.g. `trace_id`). |
| **Tempo** | Grafana's trace database; it can also generate metrics from spans. |
| **Trace** | The full story of one request across components, made of spans. |
| **TraceQL** | Tempo's query language, e.g. `{resource.service.name="gofipe" && duration > 500ms}`. |
| **Time series** | A sequence of numbers with timestamps, like "requests per second over the day". |
| **TTL (time to live)** | How long a cached item is valid before it expires. |
