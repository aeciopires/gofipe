# gofipe

![Type: application](https://img.shields.io/badge/Type-application-informational?style=flat-square)  ![Version: 1.0.0](https://img.shields.io/badge/Version-1.0.0-informational?style=flat-square)

Helm chart of example for deploy gofipe

# Prerequisites

- Install the kubectl, helm and helm-docs commands following the instructions of the file [REQUIREMENTS.md](../REQUIREMENTS.md).

# Installing the Chart

- Access a Kubernetes cluster.

- Change the values according to the need of the environment in ``gofipe/values.yaml`` file. The [Parameters](#parameters) section lists the parameters that can be configured during installation.

- Test the installation with command:

```bash
helm upgrade --install gofipe -f gofipe/values.yaml gofipe/ -n gofipe --create-namespace --dry-run
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
| extraManifests | list | `[]` | Extra arbitrary Kubernetes manifests to deploy within the release |
| fullnameOverride | string | `""` |  |
| image.pullPolicy | string | `"IfNotPresent"` | Pull policy of Docker image |
| image.repository | string | `"aeciopires/gofipe"` | Docker image name |
| image.tag | string | `"2.0.0"` | Docker image tag |
| imagePullSecrets | list | `[]` | List of dockerconfig secrets names to use when pulling images |
| ingress.annotations | string | `nil` | Ingress annotations |
| ingress.certManagerIssueEmail | string | `"CHANGE_HERE"` | Email for Cert Manager Issue |
| ingress.className | string | `""` |  |
| ingress.createCertManagerIssuer | bool | `false` | Create Cert Manager Issuer |
| ingress.createGcpManagedCertificate | bool | `false` | Create GCP Managed Certificate |
| ingress.enabled | bool | `false` | Enables Ingress |
| ingress.hosts | list | `[]` | Ingress hosts |
| ingress.tls | list | `[]` | Ingress TLS configuration |
| livenessProbe | object | `{"failureThreshold":3,"initialDelaySeconds":5,"path":"/health","periodSeconds":30,"successThreshold":1,"timeoutSeconds":5}` | Healh check continuos |
| livenessProbe.failureThreshold | int | `3` | When a probe fails, Kubernetes will try failureThreshold times before giving up. Giving up in case of liveness probe means restarting the container. In case of readiness probe the Pod will be marked Unready |
| livenessProbe.initialDelaySeconds | int | `5` | Number of seconds after the container has started before liveness |
| livenessProbe.path | string | `"/health"` | Path of health check of application |
| livenessProbe.periodSeconds | int | `30` | Specifies that the kubelet should perform a liveness probe every N seconds |
| livenessProbe.successThreshold | int | `1` | Minimum consecutive successes for the probe to be considered successful after having failed |
| livenessProbe.timeoutSeconds | int | `5` | Number of seconds after which the probe times out |
| nameOverride | string | `""` |  |
| nodeSelector | object | `{}` | Node selector configurations |
| pdb.enabled | bool | `false` |  |
| pdb.maxUnavailable | string | `"25%"` |  |
| podAnnotations | object | `{}` | Pod annotations configurations |
| podSecurityContext | object | `{}` | Pod security configurations |
| readinessProbe | object | `{"failureThreshold":3,"initialDelaySeconds":5,"path":"/health","periodSeconds":30,"successThreshold":1,"timeoutSeconds":5}` | Health check on creation pod |
| readinessProbe.failureThreshold | int | `3` | When a probe fails, Kubernetes will try failureThreshold times before giving up. Giving up in case of liveness probe means restarting the container. In case of readiness probe the Pod will be marked Unready |
| readinessProbe.initialDelaySeconds | int | `5` | Number of seconds after the container has started before readiness |
| readinessProbe.path | string | `"/health"` | Path of health check of application |
| readinessProbe.periodSeconds | int | `30` | Specifies that the kubelet should perform a liveness probe every N seconds |
| readinessProbe.successThreshold | int | `1` | Minimum consecutive successes for the probe to be considered successful after having failed |
| readinessProbe.timeoutSeconds | int | `5` | Number of seconds after which the probe times out |
| replicaCount | int | `2` | Number of replicas. Used if autoscaling is false |
| resources | object | `{"limits":{"cpu":"200m","memory":"256Mi"},"requests":{"cpu":"5m","memory":"5Mi"}}` | Requests and limits of pod resources. See: [https://kubernetes.io/docs/concepts/configuration/manage-resources-containers](https://kubernetes.io/docs/concepts/configuration/manage-resources-containers) |
| securityContext | object | `{}` | Security context configurations |
| service.annotations | object | `{}` | Annotations of service |
| service.createGcpBackendAndFrontendConfig | bool | `false` | Create GCP Backend and Frontend Config |
| service.port | int | `80` | Port of service in Kubernetes cluster |
| service.type | string | `"NodePort"` | Type of service in Kubernetes cluster |
| serviceMonitor | object | `{"additionalLabels":{},"enabled":false,"interval":"30s","namespace":"gofipe","namespaceSelector":{},"path":"/metrics","scrapeTimeout":"10s"}` | Service monitor configurations |
| tolerations | list | `[]` | Tolerations configurations |
| updateStrategy | object | `{"rollingUpdate":{"maxSurge":6,"maxUnavailable":0},"type":"RollingUpdate"}` | Update strategy configurations |
