# AnsibleGroup

Represents a group in the Ansible inventory that may contain hosts, subgroups, or both — mirroring
Ansible's inventory group structure. The operator validates that referenced members exist and are
healthy, and rolls their health up into the group's status.

- **API group / version:** `ansible-operator.lightjack.de/v1alpha1`
- **Kind:** `AnsibleGroup`
- **Scope:** Namespaced

## Example

```yaml
apiVersion: ansible-operator.lightjack.de/v1alpha1
kind: AnsibleGroup
metadata:
  name: foo
  namespace: default
spec:
  ansibleName: foo
  hosts:
    - name: baz          # an AnsibleHost in this namespace
  groups:
    - name: bar          # another AnsibleGroup in this namespace (a subgroup)
  groupVars: |
    ntp_server: pool.ntp.org
```

## Spec

| Field | Type | Required | Default | Description |
| --- | --- | --- | --- | --- |
| `ansibleName` | string | ✅ | — | Name of the group within the Ansible inventory. Must match `^[a-zA-Z0-9_.-]+$`, max 253 chars. |
| `hosts` | []LocalObjectReference | ✅ | — | References to `AnsibleHost` objects that are members of this group. |
| `groups` | []LocalObjectReference | ✅ | — | References to other `AnsibleGroup` objects that are subgroups of this group. |
| `groupVars` | string | ❌ | — | Arbitrary string inserted verbatim into this group's Ansible `group_vars`. |

Both `hosts` and `groups` are required fields, but either may be an empty list (`[]`). Each entry is a
`LocalObjectReference` — `{ name: <object-name> }` — pointing at an object in the same namespace.

!!! note "Empty lists are valid"
    A leaf group with only hosts still needs `groups: []`, and a group of subgroups still needs
    `hosts: []`.

## Status

| Field | Type | Description |
| --- | --- | --- |
| `conditions` | []Condition | Standard Kubernetes conditions reflecting the group's state. |

Condition types:

| Type | Meaning |
| --- | --- |
| `ReferencesValid` | `True` when every referenced host and subgroup exists. |
| `Healthy` | `True` when all referenced hosts and subgroups are themselves healthy. |
| `Ready` | `True` only when references are valid **and** all members are healthy. |

Because the controller indexes host and subgroup references, changing a referenced `AnsibleHost` or
`AnsibleGroup` automatically re-reconciles the groups that depend on it — health propagates up the tree.

## Related

- Members are defined by [`AnsibleHost`](ansiblehost.md) resources.
- Groups (and their nesting) shape the inventory produced by
  [`AnsibleReconcileJob`](ansiblereconcilejob.md), letting playbooks target `hosts: <groupName>`.
