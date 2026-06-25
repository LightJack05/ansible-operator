#!/usr/bin/env bash
set -euxo pipefail

cd demo

kubectl create namespace demo --dry-run=client -o yaml | kubectl apply -f -

# Create the secret for the SSH key in the correct namespace
kubectl -n demo create secret generic ansible-private-key \
  --from-file=ssh_key=../devenv/ssh-keys/id_ed25519 --dry-run=client -o yaml | kubectl apply -f -

kubectl apply -f resources/
