# Custom Resources

ansible-operator defines four namespaced Custom Resources, all in the API group
`ansible-operator.lightjack.de`, version `v1alpha1`.

| Kind | Short description |
| --- | --- |
| [`AnsibleHost`](ansiblehost.md) | A single SSH-reachable host, its connection details, and credentials. |
| [`AnsibleGroup`](ansiblegroup.md) | A named group of hosts and/or subgroups, mirroring Ansible inventory groups. |
| [`AnsiblePlaybook`](ansibleplaybook.md) | A playbook stored inline or sourced from a Git repository. |
| [`AnsibleReconcileJob`](ansiblereconcilejob.md) | A cron schedule that executes a playbook against the generated inventory. |

## How they fit together

```
AnsibleReconcileJob ── playbookRef ──▶ AnsiblePlaybook
        │
        │ builds inventory from every host & group in the namespace
        ▼
AnsibleGroup ── hosts / groups ──▶ AnsibleHost ── sshKeySecretRef ──▶ Secret
```

- An **`AnsibleReconcileJob`** references exactly one **`AnsiblePlaybook`**.
- The inventory for a run is assembled from **all** `AnsibleHost` and `AnsibleGroup` resources in the
  same namespace — not by an explicit reference from the job.
- **`AnsibleGroup`s** reference hosts and other groups by name (`LocalObjectReference`), all within the
  same namespace.
- **`AnsibleHost`s** reference two Kubernetes `Secret`s: one for the SSH private key, one for the
  trusted host key.

## Status conditions

Every resource exposes standard Kubernetes conditions under `.status.conditions`. Inspect them with:

```sh
kubectl get <kind> <name> -o jsonpath='{.status.conditions}' | jq
```

Each condition has a `type`, a `status` of `True` / `False` / `Unknown`, plus a `reason` and
`message`. The condition types per kind are documented on each resource's page.
