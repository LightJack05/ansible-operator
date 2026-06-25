#!/usr/bin/env bash
set -euxo pipefail

kubectl -n demo get ansiblegroups -o yaml
