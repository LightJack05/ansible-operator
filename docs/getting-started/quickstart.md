# Quick Start

This walkthrough takes you from a freshly installed operator to a playbook that runs on a schedule.
It assumes you've completed the [installation](installation.md) and have a host you can reach over SSH.

All resources in this guide live in the `default` namespace. The operator works per-namespace: an
`AnsibleReconcileJob` builds its inventory from the `AnsibleHost` and `AnsibleGroup` objects **in the
same namespace**.

## 1. Create the SSH credential secrets

Every host references a Kubernetes `Secret` holding its SSH **private key** under the key `ssh_key`,
plus a second secret the operator uses to store the host's trusted host key.

```sh
# Private key the operator uses to authenticate to the host
kubectl create secret generic web1-creds \
  --from-file=ssh_key=$HOME/.ssh/id_ed25519
```

!!! warning "Key must be usable non-interactively"
    The private key must not be passphrase-protected, since Ansible runs unattended inside a pod.

## 2. Define a host

```yaml title="web1-host.yaml"
apiVersion: ansible-operator.lightjack.de/v1alpha1
kind: AnsibleHost
metadata:
  name: web1
  namespace: default
spec:
  ansibleName: web1
  connection:
    host: 10.0.0.11      # hostname or IP the pod can reach
    port: 22
    user: root
  ssh:
    ignoreHostKey: false
    sshKeySecretRef:
      name: web1-creds     # The secret containing the private key
    sshHostKeySecretRef:
      name: web1-host-key  # The secret that will contain the host public key
  privilege:
    become: false
```

```sh
kubectl apply -f web1-host.yaml
kubectl get ansiblehost web1 -o jsonpath='{.status.conditions}'
```

Wait until the host reports `Ready=True`. See [AnsibleHost](../resources/ansiblehost.md) for every
field.

## 3. (Optional) Group your hosts

If you have several hosts, group them so playbooks can target them by name. A group can also contain
other groups as subgroups.

```yaml title="webservers-group.yaml"
apiVersion: ansible-operator.lightjack.de/v1alpha1
kind: AnsibleGroup
metadata:
  name: webservers
  namespace: default
spec:
  ansibleName: webservers
  hosts:
    - name: web1
  groups: []
```

```sh
kubectl apply -f webservers-group.yaml
```

## 4. Define a playbook

Playbooks can be inlined directly into the resource or fetched from Git. Here's an inline example that
pings every host:

```yaml title="ping-playbook.yaml"
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
```

```sh
kubectl apply -f ping-playbook.yaml
```

See [AnsiblePlaybook](../resources/ansibleplaybook.md) for the Git-based variant and `requirements.yml`
handling.

## 5. Schedule the run

An `AnsibleReconcileJob` ties a playbook to a cron schedule. The operator generates the inventory from
your hosts and groups, then creates a Kubernetes `CronJob` that executes the playbook.

```yaml title="nightly-job.yaml"
apiVersion: ansible-operator.lightjack.de/v1alpha1
kind: AnsibleReconcileJob
metadata:
  name: nightly-ping
  namespace: default
spec:
  schedule: "0 0 * * *"    # every day at midnight
  playbookRef:
    name: ping-all
```

```sh
kubectl apply -f nightly-job.yaml
```

## 6. Watch it work

The operator creates a `CronJob` named after the reconcile job:

```sh
kubectl get cronjob nightly-ping
```

To run it immediately instead of waiting for the schedule, trigger a manual job from the CronJob:

```sh
kubectl create job --from=cronjob/nightly-ping ping-now
kubectl logs job/ping-now --all-containers --follow
```

Check the reconcile job's status conditions to see the outcome of the most recent run:

```sh
kubectl get ansiblereconcilejob nightly-ping -o jsonpath='{.status.conditions}'
```

- `Progressing=True` — a run is currently executing.
- `Successful=True` — the last run finished without failed tasks.
- `Ready=True` — the job reconciled correctly and is scheduled.

If something looks wrong, see [Troubleshooting](../reference/troubleshooting.md).

## Recap

You created:

1. Two secrets (private key + host-key store).
2. An `AnsibleHost` describing where and how to connect.
3. An `AnsibleGroup` (optional) to organize hosts.
4. An `AnsiblePlaybook` with the tasks to run.
5. An `AnsibleReconcileJob` binding the playbook to a schedule.

From here, the operator keeps the inventory in sync whenever you add, change, or remove hosts and
groups.
