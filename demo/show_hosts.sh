#!/usr/bin/env bash
set -euxo pipefail

kubectl -n demo get ansiblehosts -o yaml
