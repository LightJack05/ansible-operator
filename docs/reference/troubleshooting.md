# Troubleshooting

A checklist for the most common issues, working from the operator down to the runner pods.

## Inspecting state

Start by reading the status conditions of the resource that isn't behaving:

```sh
kubectl get ansiblehost <name> -o jsonpath='{.status.conditions}' | jq
kubectl get ansiblegroup <name> -o jsonpath='{.status.conditions}' | jq
kubectl get ansiblereconcilejob <name> -o jsonpath='{.status.conditions}' | jq
```

The `reason` and `message` fields usually point straight at the problem. Also check the manager logs:

```sh
kubectl logs -n ansible-operator-system deploy/ansible-operator-controller-manager --follow
```

## AnsibleHost never becomes `Ready`

Likely causes:

- **Missing or malformed private key.** The `sshKeySecretRef` secret must contain a valid SSH private
  key under the key `ssh_key`. Recreate it with:
  ```sh
  kubectl create secret generic <name>-creds \
    --from-file=ssh_key=/path/to/private_key --dry-run=client -o yaml | kubectl apply -f -
  ```
- **Host-key scan failed.** With `ssh.ignoreHostKey: false` the operator connects to `connection.host`
  on `connection.port` to capture the host key. Ensure the address is reachable **from inside the
  cluster** (a `*.svc.cluster.local` name, a routable IP, etc.), not just from your workstation.
- **Passphrase-protected key.** Keys must be usable non-interactively. Provide a key without a
  passphrase.

## AnsibleGroup shows `Healthy=False` or `ReferencesValid=False`

- `ReferencesValid=False` — one of the referenced hosts or subgroups doesn't exist in the namespace.
  Check the names in `spec.hosts` and `spec.groups`.
- `Healthy=False` — the references exist but at least one member isn't `Ready`. Fix the underlying
  [`AnsibleHost`](../resources/ansiblehost.md) or subgroup first; group health propagates automatically.

## AnsibleReconcileJob is `Ready=False` / the CronJob is suspended

On a reconcile error the controller **suspends** the generated `CronJob` and records the reason in the
`Ready` condition. Common causes:

- **Invalid `schedule`.** The cron expression is rejected when the `CronJob` is created. Verify the
  expression.
- **`playbookRef` points at a missing playbook.** Confirm the [`AnsiblePlaybook`](../resources/ansibleplaybook.md)
  exists in the same namespace.

After fixing the cause, the next reconcile un-suspends the CronJob.

## A run fails (`Successful=False`)

Look at the pod logs for the most recent job:

```sh
kubectl get jobs -l  # find the job created by the cronjob
kubectl logs job/<job-name> --all-containers
```

There are two containers to inspect:

- The **init container** prepares the playbook (cloning from Git for Git-sourced playbooks and
  installing requirements). Failures here usually mean an unreachable Git repo, a bad `ref`, or a wrong
  `playbookPath` / `requirementsPath`.
- The **runner container** executes `ansible-playbook`. Failures here are normal Ansible errors:
  unreachable hosts, host-key mismatches, failed tasks, or privilege-escalation problems.

## SSH host-key verification errors at run time

The runner uses the aggregated `known_hosts` built from each host's trusted key. If a host was
re-provisioned its key changes and verification fails. Either:

- Clear the stored key so the operator re-pins it on the next reconcile (delete/empty the
  `sshHostKeySecretRef` secret), or
- Set `ssh.ignoreHostKey: true` on the host if you accept the reduced security.

## Manual run for debugging

You don't have to wait for the schedule:

```sh
kubectl create job --from=cronjob/<reconcile-job-name> debug-run
kubectl logs job/debug-run --all-containers --follow
```

## Still stuck?

Open an issue at [github.com/LightJack05/ansible-operator](https://github.com/LightJack05/ansible-operator/issues)
with the resource YAML, the relevant status conditions, and the failing pod logs.
