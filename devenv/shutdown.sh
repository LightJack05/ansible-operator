#!/usr/bin/bash
set -euxo pipefail

K8S_NAMESPACE="${K8S_NAMESPACE:-ansible-operator-system}"

# Burn the SSH key to the ground if it exists...
rm -f ssh-keys/id_ed25519 ssh-keys/id_ed25519.pub

kind delete cluster -n ansible-operator-dev || true
docker compose down || true
