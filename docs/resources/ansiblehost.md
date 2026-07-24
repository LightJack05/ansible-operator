# AnsibleHost

Represents a single host reachable over SSH that Ansible playbooks can target. On reconcile, the
operator validates the host's SSH credentials and — unless disabled — captures and pins the host's SSH
host key.

- **API group / version:** `ansible-operator.lightjack.de/v1alpha1`
- **Kind:** `AnsibleHost`
- **Scope:** Namespaced

## Example

```yaml
apiVersion: ansible-operator.lightjack.de/v1alpha1
kind: AnsibleHost
metadata:
  name: foobar
  namespace: default
spec:
  ansibleName: foobar
  connection:
    host: ssh-server-1.default.svc.cluster.local
    port: 22
    user: root
  ssh:
    ignoreHostKey: false
    sshKeySecretRef:
      name: foobar-creds
    sshHostKeySecretRef:
      name: foobar-host-key
  privilege:
    become: false
  hostVars: |
    http_port: 8080
    max_clients: 200
```

## Spec

| Field | Type | Required | Default | Description |
| --- | --- | --- | --- | --- |
| `ansibleName` | string | ✅ | — | Name used for this host in the generated Ansible inventory. Must match `^[a-zA-Z0-9_.-]+$`, max 253 chars. |
| `connection` | object | ✅ | — | SSH connection parameters. See below. |
| `ssh` | object | ✅ | — | SSH authentication configuration. See below. |
| `privilege` | object | ❌ | — | Privilege escalation settings. See below. |
| `hostVars` | string | ❌ | — | Arbitrary string inserted verbatim into the host's Ansible `host_vars`. |

### `connection`

| Field | Type | Required | Default | Description |
| --- | --- | --- | --- | --- |
| `host` | string | ✅ | — | Hostname or IP to connect to. Must match `^[a-zA-Z0-9][a-zA-Z0-9._-]*$`, max 253 chars. |
| `port` | integer | ❌ | `22` | SSH port. Range 1–65535. |
| `user` | string | ❌ | `root` | SSH login user. Must match `^[a-z_][a-z0-9_-]*$`, max 32 chars. |

!!! note "No password authentication"
    There is intentionally no `ansible_password` field — the operator only supports key-based SSH auth.

### `ssh`

| Field | Type | Required | Default | Description |
| --- | --- | --- | --- | --- |
| `ignoreHostKey` | boolean | ❌ | `false` | Disable SSH host-key verification. When `false`, the operator trusts the host key on first connect and stores it in `sshHostKeySecretRef`. |
| `sshKeySecretRef` | LocalObjectReference | ✅ | — | Reference to a `Secret` containing the SSH **private key** under the key `ssh_key`. |
| `sshHostKeySecretRef` | LocalObjectReference | ✅ | — | Reference to a `Secret` used as the trusted host-key store. Populated by the operator when `ignoreHostKey` is `false`. |

A `LocalObjectReference` is simply `{ name: <secret-name> }` and always refers to an object in the same
namespace.

### `privilege`

| Field | Type | Required | Default | Description |
| --- | --- | --- | --- | --- |
| `become` | boolean | ❌ | `false` | Enable privilege escalation (`sudo`) for tasks on this host. |

## Status

| Field | Type | Description |
| --- | --- | --- |
| `conditions` | []Condition | Standard Kubernetes conditions reflecting the host's state. |

The `Ready` condition becomes `True` once the credential secret is valid and the host key has been
resolved. It reports `False` with a reason/message when, for example, the key secret is missing or the
host key scan fails.

## The credential secrets

Create the private-key secret with the private key under `ssh_key`:

```sh
kubectl create secret generic foobar-creds \
  --from-file=ssh_key=$HOME/.ssh/id_ed25519
```

Create an (initially empty) host-key store secret; the operator fills it in on first reconcile:

```sh
kubectl create secret generic foobar-host-key
```

!!! tip "Pinning the host key yourself"
    You can pre-populate `sshHostKeySecretRef` with a known host key. When a key is already present, the
    operator leaves it untouched rather than overwriting it.

## Related

- Reference a host from an [`AnsibleGroup`](ansiblegroup.md) to organize your inventory.
- Hosts are automatically included in the inventory built by every
  [`AnsibleReconcileJob`](ansiblereconcilejob.md) in the same namespace.
