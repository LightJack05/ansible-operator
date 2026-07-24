# AnsiblePlaybook

Stores an Ansible playbook (and optional `requirements.yml`) either **inline** in the resource or by
reference to a **Git** repository. A playbook is consumed at run time by an
[`AnsibleReconcileJob`](ansiblereconcilejob.md).

- **API group / version:** `ansible-operator.lightjack.de/v1alpha1`
- **Kind:** `AnsiblePlaybook`
- **Scope:** Namespaced

!!! warning "`inline` and `git` are mutually exclusive"
    Exactly one of `inline` or `git` must be set. This is enforced by a CEL validation rule on the API
    server (`has(self.inline) != has(self.git)`), so an invalid spec is rejected at apply time.

## Inline example

Inline playbooks are stored as a `ConfigMap` and mounted into the runner pod.

```yaml
apiVersion: ansible-operator.lightjack.de/v1alpha1
kind: AnsiblePlaybook
metadata:
  name: inline-playbook
  namespace: default
spec:
  inline:
    requirements: |
      - name: ansible.builtin
        version: 1.0.0
    playbook: |
      - name: Example Playbook
        hosts: all
        tasks:
          - name: Ping
            ansible.builtin.ping:
```

## Git example

Git playbooks are cloned by an init container in the reconcile job's pod before execution.

```yaml
apiVersion: ansible-operator.lightjack.de/v1alpha1
kind: AnsiblePlaybook
metadata:
  name: http-git-playbook
  namespace: default
spec:
  git:
    repo:
      url: https://example.com/repo.git
      ref: main
    playbookPath: path/to/playbook.yaml
    requirementsPath: path/to/requirements.yml
```

## Spec

| Field | Type | Required | Description |
| --- | --- | --- | --- |
| `inline` | object | one of `inline`/`git` | Playbook (and optional requirements) provided inline. |
| `git` | object | one of `inline`/`git` | Playbook fetched from a Git repository. |

### `inline`

| Field | Type | Required | Description |
| --- | --- | --- | --- |
| `playbook` | string | ✅ | Contents of the Ansible playbook file. |
| `requirements` | string | ❌ | Contents of an Ansible `requirements.yml` file. |

### `git`

| Field | Type | Required | Default | Description |
| --- | --- | --- | --- | --- |
| `repo.url` | string | ✅ | — | Git repository URL. |
| `repo.ref` | string | ❌ | `main` | Git reference (branch, tag, or commit) to check out. |
| `playbookPath` | string | ✅ | — | Path to the playbook file within the repository. |
| `requirementsPath` | string | ❌ | — | Path to a `requirements.yml` within the repository. |

## Status

| Field | Type | Description |
| --- | --- | --- |
| `conditions` | []Condition | Standard Kubernetes conditions reflecting the playbook's state. |

## Related

- Reference this playbook from an [`AnsibleReconcileJob`](ansiblereconcilejob.md) via `spec.playbookRef`
  to schedule it.
- The playbook targets hosts and groups defined by [`AnsibleHost`](ansiblehost.md) and
  [`AnsibleGroup`](ansiblegroup.md) resources, via the inventory the reconcile job generates.
