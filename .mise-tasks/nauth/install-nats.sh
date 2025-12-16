#!/usr/bin/env bash

# Add the NATS Helm repository
helm repo add nats https://nats-io.github.io/k8s/helm/charts/

# This script is used to create a local Kubernetes cluster using KIND
# The intention is that you copy or symlink this into your repository in the same place directory as this.
load_apps() {
    local image=$1
    docker pull $image
    kind load docker-image $image
}

# Load NATS and NACK
load_apps nats:$NATS_VERSION > /dev/null 2>&1 &
load_apps docker.io/library/nats:2.11.3-alpine$NATS_VERSION > /dev/null 2>&1 &
load_apps natsio/nats-server-config-reloader:$NATS_RELOADER_VERSION > /dev/null 2>&1 &
load_apps natsio/nats-box:$NATS_BOX_VERSION  > /dev/null 2>&1 &
load_apps natsio/jetstream-controller:$NACK_VERSION > /dev/null 2>&1 &

# Install NATS and NACK
helm dependency update "$MISE_PROJECT_ROOT/local/nats"
helm install nats "$MISE_PROJECT_ROOT/local/nats" --wait -n "$NATS_NAMESPACE" -f "$MISE_PROJECT_ROOT/local/nats/values.yaml"
