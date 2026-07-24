# Installation

## Prerequisites

- A Kubernetes cluster, v1.11.3+ (a recent version is recommended for full CRD/CEL validation support).
- `kubectl` configured against the cluster.
- Permissions to install CRDs and cluster/namespaced RBAC (typically `cluster-admin`).
- For Helm installs: [Helm](https://helm.sh/) v3.8+ (OCI registry support).

The operator itself is published to the GitHub Container Registry:

- Manager image: `ghcr.io/lightjack05/ansible-operator`
- Runner images (pulled automatically by scheduled jobs):
  `ghcr.io/lightjack05/ansible-operator-runner-init` and
  `ghcr.io/lightjack05/ansible-operator-runner-runner`

!!! info "Target host requirements"
    The operator runs Ansible *inside* cluster pods, so your nodes don't need Ansible installed.
    Each target host only needs to be reachable over SSH and satisfy Ansible's usual requirement of a
    Python interpreter. See [Architecture](../concepts/architecture.md) for details.

## Option 1: Helm (recommended)

The chart is published as an OCI artifact:

```sh
helm install ansible-operator \
  oci://ghcr.io/lightjack05/charts/ansible-operator \
  --namespace ansible-operator-system \
  --create-namespace
```

To pin a specific chart version, add `--version <x.y.z>`.

### Overriding values

Pass `--set` flags or a values file to customize the deployment. For example, to run two replicas
with leader election:

```sh
helm install ansible-operator \
  oci://ghcr.io/lightjack05/charts/ansible-operator \
  --namespace ansible-operator-system --create-namespace \
  --set manager.replicas=2
```

The most useful values are the manager image and resources; the full list is documented in the
[Helm Values reference](../reference/helm-values.md).

!!! tip "Pinning the image"
    The chart defaults the manager image to `controller:latest`. When installing from the published
    chart you normally want to pin it to the released image:
    ```sh
    --set manager.image.repository=ghcr.io/lightjack05/ansible-operator \
    --set manager.image.tag=latest
    ```

### Upgrading

```sh
helm upgrade ansible-operator \
  oci://ghcr.io/lightjack05/charts/ansible-operator \
  --namespace ansible-operator-system
```

By default the chart keeps the CRDs on uninstall (`crd.keep=true`) so your custom resources survive an
accidental `helm uninstall`.

### Uninstalling

```sh
helm uninstall ansible-operator --namespace ansible-operator-system
```

## Option 2: Bundled manifests

Every release also ships a single, self-contained manifest bundle. Apply it directly with `kubectl`:

```sh
kubectl apply -f https://raw.githubusercontent.com/LightJack05/ansible-operator/main/dist/install.yaml
```

Replace `main` with a release tag to install a specific version. This installs the CRDs, RBAC, and the
manager `Deployment` into the `ansible-operator-system` namespace.

To uninstall:

```sh
kubectl delete -f https://raw.githubusercontent.com/LightJack05/ansible-operator/main/dist/install.yaml
```

## Option 3: From source (development)

Clone the repository and use the provided `Makefile` targets. This is aimed at contributors and local
testing.

```sh
# Install the CRDs into the cluster your kubeconfig points at
make install

# Build and push your own manager image
make docker-build docker-push IMG=<your-registry>/ansible-operator:dev

# Deploy the manager using that image
make deploy IMG=<your-registry>/ansible-operator:dev
```

To tear things down again:

```sh
make undeploy   # remove the manager Deployment
make uninstall  # remove the CRDs
```

Run `make help` for the full list of targets.

## Verifying the install

Check that the manager pod is running:

```sh
kubectl get pods -n ansible-operator-system
```

And that the CRDs are registered:

```sh
kubectl get crds | grep ansible-operator.lightjack.de
```

You should see `ansiblehosts`, `ansiblegroups`, `ansibleplaybooks`, and `ansiblereconcilejobs`.

Next, head to the [Quick Start](quickstart.md).
