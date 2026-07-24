# ansible-operator

**ansible-operator** is a Kubernetes operator that runs [Ansible](https://www.ansible.com/) playbooks
against SSH-reachable hosts, on a schedule, using nothing but Kubernetes resources.

Instead of maintaining an inventory file, a control node, and a cron entry somewhere, you describe
your hosts, groups, and playbooks as Custom Resources. The operator generates the Ansible inventory,
manages SSH host-key trust, and dispatches playbook runs as Kubernetes `CronJob`s.

```yaml
apiVersion: ansible-operator.lightjack.de/v1alpha1
kind: AnsibleReconcileJob
metadata:
  name: nightly-config
  namespace: default
spec:
  schedule: "0 0 * * *"        # every night at midnight
  playbookRef:
    name: base-config          # an AnsiblePlaybook in the same namespace
```

## Why use it

- **Declarative Ansible** &mdash; hosts, groups, playbooks, and schedules are all Kubernetes objects
  you can `kubectl apply`, GitOps, and RBAC like anything else in your cluster.
- **Automatic inventory generation** &mdash; the operator renders a full Ansible inventory (hosts,
  groups, subgroups, host/group vars) from your resources on every change.
- **SSH host-key management** &mdash; host keys are scanned and stored on first connect, so runs stay
  protected against man-in-the-middle without manual `known_hosts` juggling.
- **Scheduled reconciliation** &mdash; every `AnsibleReconcileJob` becomes a Kubernetes `CronJob`, so
  playbook runs get the reliability, history, and observability of native workloads.
- **Status you can watch** &mdash; each resource reports Kubernetes conditions (`Ready`, `Progressing`,
  `Successful`, ...) so you can tell at a glance whether the last run worked.

## The resources at a glance

| Resource | Purpose |
| --- | --- |
| [`AnsibleHost`](resources/ansiblehost.md) | A single SSH-reachable host and its credentials. |
| [`AnsibleGroup`](resources/ansiblegroup.md) | A named group of hosts and/or subgroups. |
| [`AnsiblePlaybook`](resources/ansibleplaybook.md) | A playbook, stored inline or fetched from Git. |
| [`AnsibleReconcileJob`](resources/ansiblereconcilejob.md) | A schedule that runs a playbook against the inventory. |

## Where to next

<div class="grid cards" markdown>

- :material-download: **[Installation](getting-started/installation.md)** &mdash; install the operator with Helm or plain manifests.
- :material-rocket-launch: **[Quick Start](getting-started/quickstart.md)** &mdash; go from zero to a scheduled playbook run.
- :material-sitemap: **[Architecture](concepts/architecture.md)** &mdash; understand how the pieces fit together.
- :material-book-open-variant: **[Custom Resources](resources/index.md)** &mdash; the full API reference.

</div>

!!! note "Project status"
    ansible-operator is at API version `v1alpha1`. The API may still change between releases.
