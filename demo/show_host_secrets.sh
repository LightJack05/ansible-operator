#!/usr/bin/env bash
set -euxo pipefail

kubectl -n demo get secrets

kubectl -n demo get secrets ssh-node-1-host-key -o yaml | yq -r '.data.host_keys' | base64 -d
