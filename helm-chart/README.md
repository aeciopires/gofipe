# gofipe

![Type: application](https://img.shields.io/badge/Type-application-informational?style=flat-square)  ![Version: 2.1.0](https://img.shields.io/badge/Version-2.1.0-informational?style=flat-square)

Helm chart of example for deploy gofipe (requires Helm 4)

# Prerequisites

- **Helm 4** (Helm 3 is no longer supported), `kubectl` and `helm-docs`. They are pinned in [mise.toml](../mise.toml) and installed with `make install-deps` (see [REQUIREMENTS.md](../REQUIREMENTS.md)).
- Kubernetes >=1.29.0-0.

# Makefile shortcuts

Run `make help` inside `helm-chart/` to list every target. The most used ones:

| Target | What it does |
|--------|--------------|
| `make lint` | `helm lint --strict .` |
| `make template` | Renders the manifests locally |
| `make dry-run` | Server-side dry-run against the current cluster |
| `make install` | `helm upgrade --install` (waits for the pods) |
| `make uninstall` | Removes the release |
| `make gen-docs` | Regenerates this README from `values.yaml` |

Use `RELEASE=<name>` and `NAMESPACE=<ns>` to change the defaults (`gofipe`/`gofipe`).

# Installing the Chart

- Access a Kubernetes cluster.

- Change the values according to the need of the environment in ``gofipe/values.yaml`` file. The [Parameters](#parameters) section lists the parameters that can be configured during installation.

- Test the installation with command:

```bash
helm upgrade --install gofipe -f gofipe/values.yaml gofipe/ -n gofipe --create-namespace --dry-run=server
```

- To install/upgrade the chart with the release name `gofipe`:

```bash
helm upgrade --install gofipe -f gofipe/values.yaml gofipe/ -n gofipe --create-namespace
```

Create a port-forward with the follow command:

```bash
kubectl port-forward svc/gofipe 8080:80 -n gofipe
```

Open the browser and access the URL: http://localhost:8080

## Uninstalling the Chart

To uninstall/delete the `gofipe` deployment:

```bash
helm uninstall gofipe -n gofipe
```

The command removes all the Kubernetes components associated with the chart and deletes the release.

## Parameters

The following tables lists the configurable parameters of the chart and their default values.

Change the values according to the need of the environment in ``gofipe/values.yaml`` file.

| Key | Type | Default | Description |
|-----|------|---------|-------------|
| affinity | object | `{}` | Affinity configurations |
| autoscaling | object | `{"enabled":false,"maxReplicas":20,"minReplicas":1,"targetCPUUtilizationPercentage":80}` | Auto scaling configurations |
| containerPort | int | `8080` | Port the container listens on (passed to the app through the PORT env var) |
| env | list | `[]` | Extra environment variables for the container. Examples: FIPE_API_TOKEN from a Secret (higher FIPE rate limits / longer price history) and OpenTelemetry export of traces and logs to an OTLP/HTTP collector (tracing is disabled while no OTEL_EXPORTER_OTLP_*ENDPOINT is set). |
| extraManifests | list | `[]` | Extra arbitrary Kubernetes manifests to deploy within the release |
| fullnameOverride | string | `""` | Override the full name of the resources |
| image.pullPolicy | string | `"IfNotPresent"` | Pull policy of Docker image |
| image.repository | string | `"aeciopires/gofipe"` | Docker image name |
| image.tag | string | `"4.1.0"` | Docker image tag |
| imagePullSecrets | list | `[]` | List of dockerconfig secrets names to use when pulling images |
| ingress.annotations | string | `nil` | Ingress annotations |
| ingress.certManagerIssueEmail | string | `"CHANGE_HERE"` | Email for Cert Manager Issue |
| ingress.className | string | `""` | IngressClass name (e.g. nginx, alb, gce) |
| ingress.createCertManagerIssuer | bool | `false` | Create Cert Manager Issuer |
| ingress.createGcpManagedCertificate | bool | `false` | Create GCP Managed Certificate |
| ingress.enabled | bool | `false` | Enables Ingress |
| ingress.hosts | list | `[]` | Ingress hosts |
| ingress.tls | list | `[]` | Ingress TLS configuration |
| livenessProbe | object | `{"failureThreshold":3,"initialDelaySeconds":0,"path":"/health/live","periodSeconds":15,"successThreshold":1,"timeoutSeconds":3}` | Liveness probe: Kubernetes restarts the container when it fails. /health/live only checks that the process answers HTTP (it does not call FIPE, so a FIPE outage never restarts the pods). |
| livenessProbe.failureThreshold | int | `3` | Failed checks before the container is restarted |
| livenessProbe.initialDelaySeconds | int | `0` | Number of seconds after the container has started before liveness |
| livenessProbe.path | string | `"/health/live"` | Path of the liveness check |
| livenessProbe.periodSeconds | int | `15` | How often (in seconds) to perform the probe |
| livenessProbe.successThreshold | int | `1` | Minimum consecutive successes for the probe to be considered successful after having failed |
| livenessProbe.timeoutSeconds | int | `3` | Number of seconds after which the probe times out |
| nameOverride | string | `""` | Override the chart name |
| nodeSelector | object | `{}` | Node selector configurations |
| podAnnotations | object | `{}` | Pod annotations configurations |
| podDisruptionBudget | object | `{"enabled":false,"maxUnavailable":"","minAvailable":"25%"}` | Pod Disruption Budget configuration |
| podDisruptionBudget.maxUnavailable | string | `""` | Maximum unavailable pods. Takes precedence over minAvailable when set |
| podDisruptionBudget.minAvailable | string | `"25%"` | Minimum available pods (ignored when maxUnavailable is set) |
| podSecurityContext | object | `{"runAsGroup":10001,"runAsNonRoot":true,"runAsUser":10001,"seccompProfile":{"type":"RuntimeDefault"}}` | Pod security configurations |
| readinessProbe | object | `{"failureThreshold":2,"initialDelaySeconds":0,"path":"/health/ready","periodSeconds":5,"successThreshold":1,"timeoutSeconds":2}` | Readiness probe: while it fails the pod receives no traffic from the Service. /health/ready returns 503 while gofipe starts and after it receives SIGTERM (see shutdownDelay), so rolling updates do not drop requests. |
| readinessProbe.failureThreshold | int | `2` | Failed checks before the pod is marked Unready |
| readinessProbe.initialDelaySeconds | int | `0` | Number of seconds after the container has started before readiness |
| readinessProbe.path | string | `"/health/ready"` | Path of the readiness check |
| readinessProbe.periodSeconds | int | `5` | How often (in seconds) to perform the probe |
| readinessProbe.successThreshold | int | `1` | Minimum consecutive successes for the probe to be considered successful after having failed |
| readinessProbe.timeoutSeconds | int | `2` | Number of seconds after which the probe times out |
| replicaCount | int | `2` | Number of replicas. Used if autoscaling is false |
| resources | object | `{"limits":{"cpu":"200m","memory":"256Mi"},"requests":{"cpu":"5m","memory":"5Mi"}}` | Requests and limits of pod resources. See: [https://kubernetes.io/docs/concepts/configuration/manage-resources-containers](https://kubernetes.io/docs/concepts/configuration/manage-resources-containers) |
| securityContext | object | `{"allowPrivilegeEscalation":false,"capabilities":{"drop":["ALL"]},"readOnlyRootFilesystem":true}` | Container security configurations (the image runs as the non-root UID 10001) |
| service.annotations | object | `{}` | Annotations of service |
| service.createGcpBackendAndFrontendConfig | bool | `false` | Create GCP Backend and Frontend Config |
| service.loadBalancerIP | string | `""` | Load balancer IP (only used when type is LoadBalancer and supported by the cloud provider) |
| service.nodePort | string | `""` | Fixed node port (only used when type is NodePort). Empty = chosen by Kubernetes |
| service.port | int | `80` | Port of service in Kubernetes cluster |
| service.type | string | `"NodePort"` | Type of service in Kubernetes cluster |
| serviceMonitor | object | `{"additionalLabels":{},"enabled":false,"honorLabels":false,"interval":"30s","namespace":"gofipe","namespaceSelector":{},"path":"/metrics","scrapeTimeout":"10s"}` | Service monitor configurations (requires the Prometheus Operator CRDs) |
| shutdownDelay | string | `"5s"` | Time gofipe keeps serving after SIGTERM while /health/ready returns 503 (SHUTDOWN_DELAY), so endpoints are removed before the server stops. Must be smaller than terminationGracePeriodSeconds minus ~15s of HTTP shutdown. |
| startupProbe | object | `{"failureThreshold":15,"path":"/health/live","periodSeconds":2,"timeoutSeconds":2}` | Startup probe: gives the container time to start before liveness/readiness checks begin. Uses the liveness endpoint. Total startup budget = periodSeconds * failureThreshold. |
| startupProbe.failureThreshold | int | `15` | Failed checks before Kubernetes restarts the container (2s * 15 = 30s) |
| startupProbe.path | string | `"/health/live"` | Path of the startup check (liveness endpoint) |
| startupProbe.periodSeconds | int | `2` | Seconds between startup checks |
| startupProbe.timeoutSeconds | int | `2` | Number of seconds after which the probe times out |
| terminationGracePeriodSeconds | int | `30` | Seconds Kubernetes waits after SIGTERM before killing the container |
| tolerations | list | `[]` | Tolerations configurations |
| updateStrategy | object | `{"rollingUpdate":{"maxSurge":6,"maxUnavailable":0},"type":"RollingUpdate"}` | Update strategy configurations |
