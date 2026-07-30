#!/usr/bin/bash
set -euxo pipefail


# Burn the SSH key to the ground if it exists...
rm -f ssh-keys/id_ed25519 ssh-keys/id_ed25519.pub
# ...and create a new one.
ssh-keygen -t ed25519 -N "" -f ssh-keys/id_ed25519

kind create cluster --config kind-config.yaml
docker compose up -d --build


# Generate the necessary secrets
kubectl create secret generic ansible-operator-hosts-private-key \
  --from-file=ssh_key=./ssh-keys/id_ed25519 \

kubectl create secret generic ansible-operator-hosts-public-key \
  --from-file=ssh-publickey=./ssh-keys/id_ed25519.pub \

kubectl create secret generic ansible-ssh-username-password \
  --from-literal=username=ansible \
  --from-literal=password=ansiblepass \

kubectl create secret generic ansible-git-credentials \
  --from-literal=git-username=git \
  --from-literal=git-password=gitpass

# Create out-of-cluster services and endpointslices
kubectl apply -f k8s/git-server.yaml -f k8s/ssh-nodes.yaml

