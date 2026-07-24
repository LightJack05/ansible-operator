# AnsibleReconcileJob

Schedules the execution of an [`AnsiblePlaybook`](ansibleplaybook.md) on a cron schedule. This is the
resource that actually makes things happen: the operator generates an Ansible inventory from the hosts
and groups in the namespace and manages a Kubernetes `CronJob` that runs the playbook.

- **API group / version:** `ansible-operator.lightjack.de/v1alpha1`
- **Kind:** `AnsibleReconcileJob`
- **Scope:** Namespaced

## Example

```yaml
apiVersion: ansible-operator.lightjack.de/v1alpha1
kind: AnsibleReconcileJob
metadata:
  name: example-reconcile-job
  namespace: default
spec:
  schedule: "0 0 * * *"
  playbookRef:
    name: some-playbook
```

## Spec

| Field | Type | Required | Description |
| --- | --- | --- | --- |
| `schedule` | string | ✅ | A cron expression defining when the job runs (e.g. `"0 0 * * *"` for daily at midnight). |
| `playbookRef` | LocalObjectReference | ✅ | Reference to the `AnsiblePlaybook` to execute, in the same namespace. |

!!! info "Inventory is built from the namespace, not from `playbookRef`"
    The set of hosts a run targets comes from **all** [`AnsibleHost`](ansiblehost.md) and
    [`AnsibleGroup`](ansiblegroup.md) resources in the same namespace. Which of those the playbook acts
    on is decided inside the playbook itself, via its `hosts:` selector (e.g. `hosts: all` or
    `hosts: webservers`).

!!! warning "Schedule validation"
    The cron `schedule` is validated by Kubernetes when the underlying `CronJob` is created. An invalid
    expression surfaces as a reconcile error and the job's `Ready` condition goes `False`.

## What the operator creates

For a reconcile job named `<name>`, the controller manages several owned objects:

| Object | Name | Purpose |
| --- | --- | --- |
| `ConfigMap` | `<name>-inventory` | The rendered Ansible inventory (`hosts.yaml`). |
| `ConfigMap` | `<name>-known-hosts` | Aggregated trusted SSH host keys. |
| `ConfigMap` | `<name>-job-config` | Inline playbook data or Git repo details. |
| `CronJob` | `<name>` | Runs the playbook on the schedule. |

These are regenerated whenever a relevant `AnsibleHost`, `AnsibleGroup`, or the referenced
`AnsiblePlaybook` changes.

## Status

| Field | Type | Description |
| --- | --- | --- |
| `conditions` | []Condition | Standard Kubernetes conditions reflecting the job's state. |

Condition types:

| Type | Meaning |
| --- | --- |
| `Ready` | `True` when the job reconciled successfully and is scheduled. `False` (with the CronJob suspended) on reconcile errors. |
| `Progressing` | `True` while a run belonging to this job is executing. |
| `Successful` | `True` when the last run completed with no failed tasks; `False` on failure; `Unknown` before the first completed run. |

## Running on demand

The generated `CronJob` runs on its schedule, but you can trigger a run immediately:

```sh
kubectl create job --from=cronjob/<name> <name>-manual
kubectl logs job/<name>-manual --all-containers --follow
```

## Related

- [`AnsiblePlaybook`](ansibleplaybook.md) — what gets executed.
- [`AnsibleHost`](ansiblehost.md) / [`AnsibleGroup`](ansiblegroup.md) — what builds the inventory.
- [Architecture](../concepts/architecture.md) — how the CronJob pod is assembled.
