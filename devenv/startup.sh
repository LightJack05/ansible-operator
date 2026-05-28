#!/usr/bin/bash
set -euxo pipefail

K8S_NAMESPACE="${K8S_NAMESPACE:-ansible-operator-system}"

# Burn the SSH key to the ground if it exists...
rm -f ssh-keys/id_ed25519 ssh-keys/id_ed25519.pub
# ...and create a new one.
ssh-keygen -t ed25519 -N "" -f ssh-keys/id_ed25519

kind create cluster --config kind-config.yaml
docker compose up -d --build


kubectl create namespace $K8S_NAMESPACE
# Generate the necessary secrets
kubectl -n $K8S_NAMESPACE create secret generic ansible-operator-hosts-private-key \
  --from-file=ssh-privatekey=./ssh-keys/id_ed25519 \

kubectl -n $K8S_NAMESPACE create secret generic ansible-operator-hosts-public-key \
  --from-file=ssh-publickey=./ssh-keys/id_ed25519.pub \

kubectl -n $K8S_NAMESPACE create secret generic ansible-ssh-username-password \
  --from-literal=username=ansible \
  --from-literal=password=ansiblepass \

kubectl -n $K8S_NAMESPACE create secret generic ansible-git-credentials \
  --from-literal=git-username=git \
  --from-literal=git-password=gitpass

# Create out-of-cluster services and endpointslices
kubectl apply -f k8s/git-server.yaml -f k8s/ssh-nodes.yaml

