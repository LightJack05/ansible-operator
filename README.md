# ansible-operator

A Kubernetes operator that runs [Ansible](https://www.ansible.com/) playbooks against SSH-reachable
hosts, on a schedule, using nothing but Kubernetes resources.

## Description

ansible-operator lets you manage Ansible-based configuration and automation declaratively from
Kubernetes. Instead of maintaining an inventory file, a control node, and a cron entry somewhere, you
describe your hosts, groups, and playbooks as Custom Resources. The operator generates the Ansible
inventory, manages SSH host-key trust, and dispatches playbook runs as Kubernetes `CronJob`s — so every
run gets native scheduling, retries, logs, and RBAC.

It defines four namespaced Custom Resources in the API group `ansible-operator.lightjack.de/v1alpha1`:

| Kind | Purpose |
| --- | --- |
| `AnsibleHost` | A single SSH-reachable host, its connection details, and credentials. |
| `AnsibleGroup` | A named group of hosts and/or subgroups, mirroring Ansible inventory groups. |
| `AnsiblePlaybook` | A playbook stored inline or sourced from a Git repository. |
| `AnsibleReconcileJob` | A cron schedule that runs a playbook against the generated inventory. |

Ansible runs inside cluster pods, so your target hosts don't need Ansible installed — they only need to
be reachable over SSH and to satisfy Ansible's usual Python-interpreter requirement.

## Documentation

Full documentation — installation, a quick-start walkthrough, architecture, and the complete API
reference — is published at:

**📖 <https://lightjack05.github.io/ansible-operator/>**

The docs are built with [MkDocs](https://www.mkdocs.org/) and
[Material for MkDocs](https://squidfunk.github.io/mkdocs-material/) from the [`docs/`](docs/) directory.

## Quick start

> Assumes a running Kubernetes cluster and `kubectl`/`helm` configured against it.

Install the operator with Helm:

```sh
helm install ansible-operator \
  oci://ghcr.io/lightjack05/charts/ansible-operator \
  --namespace ansible-operator-system --create-namespace
```

Then define a host, a playbook, and a schedule (see the
[Quick Start guide](https://lightjack05.github.io/ansible-operator/getting-started/quickstart/) for the
full walkthrough, including the required SSH secrets):

```yaml
apiVersion: ansible-operator.lightjack.de/v1alpha1
kind: AnsiblePlaybook
metadata:
  name: ping-all
  namespace: default
spec:
  inline:
    playbook: |
      - name: Ping all hosts
        hosts: all
        tasks:
          - name: Ping
            ansible.builtin.ping:
---
apiVersion: ansible-operator.lightjack.de/v1alpha1
kind: AnsibleReconcileJob
metadata:
  name: nightly-ping
  namespace: default
spec:
  schedule: "0 0 * * *"
  playbookRef:
    name: ping-all
```

## Installation options

- **Helm** (recommended): chart published at `oci://ghcr.io/lightjack05/charts/ansible-operator`.
- **Bundled manifests**:
  ```sh
  kubectl apply -f https://raw.githubusercontent.com/LightJack05/ansible-operator/main/dist/install.yaml
  ```
- **From source** (contributors):
  ```sh
  make install                                        # install CRDs
  make deploy IMG=<registry>/ansible-operator:tag     # deploy the manager
  ```

See the [installation docs](https://lightjack05.github.io/ansible-operator/getting-started/installation/)
for details and configuration.

## Container images

- Manager: `ghcr.io/lightjack05/ansible-operator`
- Runner (pulled automatically by scheduled jobs): `ghcr.io/lightjack05/ansible-operator-runner-init`
  and `ghcr.io/lightjack05/ansible-operator-runner-runner`

## Development

This project is scaffolded with [Kubebuilder](https://book.kubebuilder.io/). A [Nix](https://nixos.org/)
flake provides a reproducible dev shell with all tooling (Go, kubebuilder, kind, mkdocs, ...):

```sh
nix develop
```

Common tasks (run `make help` for the full list):

```sh
make manifests    # regenerate CRDs/RBAC after editing api/*_types.go
make generate     # regenerate DeepCopy methods
make test-e2e     # run e2e tests against a Kind cluster
make lint-fix     # auto-fix code style
```

To work on the documentation locally:

```sh
mkdocs serve      # live-reloading preview at http://127.0.0.1:8000
mkdocs build      # build the static site into ./site
```

Docs are automatically published to GitHub Pages on every push to `main` via
[`.github/workflows/docs.yml`](.github/workflows/docs.yml).
