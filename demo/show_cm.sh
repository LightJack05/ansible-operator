#!/usr/bin/env bash
set -euxo pipefail

kubectl -n demo get cm example-reconcile-job-inventory -o yaml
