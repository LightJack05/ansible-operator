# Architecture

ansible-operator follows the standard Kubernetes operator pattern: a **manager** process runs a set of
**controllers**, each watching one Custom Resource type and reconciling the cluster toward the declared
state. Actual playbook execution is delegated to Kubernetes `CronJob`s so that runs get native
scheduling, retries, and log history.

## Controllers

The manager runs four controllers, one per resource kind.

### AnsibleHostReconciler

Prepares a host for use:

1. Verifies the referenced SSH private-key `Secret` exists and contains a valid key under `ssh_key`.
2. If host-key verification is enabled (`ssh.ignoreHostKey: false`), performs an SSH key scan of the
   host and stores the trusted host key in the referenced host-key secret. It only writes the key when
   one isn't already present, so the trust is pinned on first connect.
3. Materializes the host's `hostVars` into a `ConfigMap`.
4. Reports a `Ready` condition summarizing the result.

### AnsibleGroupReconciler

Validates group membership and rolls up health:

1. Materializes `groupVars` into a `ConfigMap`.
2. Checks that every referenced host and subgroup exists (`ReferencesValid`) and is itself healthy
   (`Healthy`).
3. Sets `Ready` only when references are valid **and** all members are healthy.

It uses field indexers on host and subgroup references, so a change to any referenced `AnsibleHost` or
`AnsibleGroup` automatically re-triggers reconciliation of the groups that depend on it.

### AnsiblePlaybookReconciler

Currently a lightweight controller. Playbook specs are validated by the API server's schema (including
the CEL rule that `inline` and `git` are mutually exclusive). The stored playbook is consumed at
run time by the reconcile job.

### AnsibleReconcileJobReconciler

The workhorse. On each reconcile it:

1. **Builds the inventory.** It lists all `AnsibleHost` and `AnsibleGroup` objects in the namespace and
   renders an Ansible inventory (`hosts.yaml`) with per-host `ansible_host`, `ansible_port`,
   `ansible_user`, `ansible_become`, and SSH key path variables, plus group and subgroup structure.
2. **Aggregates known hosts.** It collects the trusted SSH host keys from every host into a
   `known_hosts` `ConfigMap`.
3. **Stages the playbook.** For inline playbooks it stores the playbook and requirements as ConfigMap
   data; for Git playbooks it records the repo URL, ref, and paths for the init container to clone.
4. **Creates/updates a `CronJob`** named after the reconcile job. The job's pod template has:
     - an **init container** that fetches the playbook (cloning from Git when needed) and prepares
       Ansible dependencies into shared volumes, and
     - a **runner container** that executes `ansible-playbook` with all of the above mounted in.
5. **Reports status** from the newest child `Job` — `Progressing`, `Successful`, and `Ready`.

If reconciliation fails, the controller suspends the CronJob and sets `Ready=False` with a message, so
a broken configuration can't keep firing runs.

Like the group controller, it watches the resources a run depends on: changes to any `AnsibleHost`,
`AnsibleGroup`, or the referenced `AnsiblePlaybook` re-trigger reconciliation and regenerate the
inventory.

## Data flow into a run

When a scheduled job fires, the runner pod receives everything it needs as mounted volumes:

| Mount | Source | Contents |
| --- | --- | --- |
| `/inventory/hosts.yaml` | generated ConfigMap | The rendered Ansible inventory |
| `/inventory/group_vars/<group>` | group ConfigMaps | Per-group `group_vars` |
| `/inventory/host_vars/<host>` | host ConfigMaps | Per-host `host_vars` |
| `/ssh/known_hosts` | generated ConfigMap | Trusted host keys |
| `/ssh/keys/<host>` | per-host Secrets | SSH private keys |
| `/config/` | playbook ConfigMap / env | Inline playbook or Git repo details |

## Security model

- **Host-key trust** is pinned on first connect and reused, rather than blindly accepting keys on every
  run. You can opt out per host with `ssh.ignoreHostKey: true`.
- **Credentials never leave Secrets.** Private keys are referenced by name and mounted into runner pods,
  not copied into specs.
- **No SSH passwords.** The API deliberately has no password field — authentication is key-based.
- The manager runs as a **non-root, read-only-root-filesystem** container with all Linux capabilities
  dropped (see the [Helm values](../reference/helm-values.md)).

!!! note "`hostVars` / `groupVars` are inserted verbatim"
    These fields are injected directly into the generated Ansible vars. Treat them as trusted input and
    avoid templating untrusted data into them.
