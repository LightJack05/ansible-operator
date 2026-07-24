# Helm Values

The Helm chart is published as an OCI artifact at
`oci://ghcr.io/lightjack05/charts/ansible-operator`. This page documents its configurable values. See
[Installation](../getting-started/installation.md) for install commands.

## Manager

Settings for the controller-manager `Deployment`.

| Value | Default | Description |
| --- | --- | --- |
| `manager.replicas` | `1` | Number of manager replicas. Use leader election (default) when running more than one. |
| `manager.image.repository` | `controller` | Manager image repository. Set to `ghcr.io/lightjack05/ansible-operator` for the released image. |
| `manager.image.tag` | `latest` | Manager image tag. |
| `manager.image.pullPolicy` | `IfNotPresent` | Image pull policy. |
| `manager.args` | `["--leader-elect"]` | Arguments passed to the manager binary. |
| `manager.env` | `[]` | Extra environment variables. |
| `manager.envOverrides` | `{}` | Per-variable overrides (`--set manager.envOverrides.VAR=value`); takes precedence over `env`. |
| `manager.imagePullSecrets` | `[]` | Image pull secrets for private registries. |
| `manager.resources.limits` | `cpu: 500m`, `memory: 128Mi` | Resource limits. |
| `manager.resources.requests` | `cpu: 10m`, `memory: 64Mi` | Resource requests. |
| `manager.podSecurityContext` | `runAsNonRoot: true`, `seccompProfile.type: RuntimeDefault` | Pod-level security context. |
| `manager.securityContext` | `allowPrivilegeEscalation: false`, `capabilities.drop: [ALL]`, `readOnlyRootFilesystem: true` | Container-level security context. |
| `manager.affinity` | `{}` | Pod affinity rules. |
| `manager.nodeSelector` | `{}` | Node selector. |
| `manager.tolerations` | `[]` | Pod tolerations. |

## CRDs

| Value | Default | Description |
| --- | --- | --- |
| `crd.enable` | `true` | Install the CRDs together with the chart. |
| `crd.keep` | `true` | Keep the CRDs (and thus your custom resources) when the release is uninstalled. |

## Metrics

| Value | Default | Description |
| --- | --- | --- |
| `metrics.enable` | `true` | Expose the RBAC-protected `/metrics` endpoint. |
| `metrics.port` | `8443` | Port the metrics server listens on. |

## Integrations

| Value | Default | Description |
| --- | --- | --- |
| `rbacHelpers.enable` | `false` | Install convenience admin/editor/viewer RBAC roles for the custom resources. |
| `certManager.enable` | `false` | Enable cert-manager integration for TLS certificates. |
| `prometheus.enable` | `false` | Install a Prometheus `ServiceMonitor` for scraping metrics (requires prometheus-operator). |

## Naming overrides

| Value | Default | Description |
| --- | --- | --- |
| `nameOverride` | _(unset)_ | Partially override the generated chart fullname (keeps the release name). |
| `fullnameOverride` | _(unset)_ | Fully override the generated chart fullname. |

## Example values file

```yaml title="values.yaml"
manager:
  replicas: 2
  image:
    repository: ghcr.io/lightjack05/ansible-operator
    tag: latest
  resources:
    limits:
      cpu: "1"
      memory: 256Mi

metrics:
  enable: true

prometheus:
  enable: true
```

```sh
helm install ansible-operator \
  oci://ghcr.io/lightjack05/charts/ansible-operator \
  -n ansible-operator-system --create-namespace \
  -f values.yaml
```
