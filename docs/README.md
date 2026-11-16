# gofipe documentation for beginners

Welcome! These pages explain **what gofipe does and how it is built**, assuming you are just starting with programming. Every chapter uses everyday analogies and [Mermaid](https://mermaid.js.org) diagrams (GitHub renders them automatically).

Read them in order the first time:

| # | Chapter | You will learn |
|---|---------|----------------|
| 1 | [Overview](01-overview.md) | What the app does and the big picture |
| 2 | [Architecture](02-architecture.md) | The pieces of the system and how a request travels through them |
| 3 | [Backend in Go](03-backend-go.md) | A guided tour of the Go code, file by file |
| 4 | [Frontend](04-frontend.md) | How the web page talks to the backend |
| 5 | [Cache and price history](05-cache-and-history.md) | How the app avoids repeated work and builds the price chart |
| 6 | [Observability](06-observability.md) | Health checks, JSON logs (Loki), metrics (Prometheus), traces (OpenTelemetry, Tempo, Jaeger) and Grafana dashboards |
| 7 | [Deployment](07-deployment.md) | Docker images, Docker Compose, Kubernetes and Helm 4 |
| 8 | [Development workflow](08-development-workflow.md) | mise, make targets, tests, linters and releases |
| - | [Glossary](glossary.md) | Short definitions of every technical term used here |

> Tip: when a word looks strange, check the [glossary](glossary.md).
