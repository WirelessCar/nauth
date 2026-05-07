#!/usr/bin/env bash
set -euo pipefail

# Add the NATS Helm repository
helm repo add nats https://nats-io.github.io/k8s/helm/charts/

# This script is used to create a local Kubernetes cluster using KIND
# The intention is that you copy or symlink this into your repository in the same place directory as this.
load_apps() {
    local image=$1
    docker pull "$image"
    kind load docker-image "$image"
}

# Load NATS and NACK
images=(
    "nats:$NATS_VERSION"
    "natsio/nats-server-config-reloader:$NATS_RELOADER_VERSION"
    "natsio/nats-box:$NATS_BOX_VERSION"
    "natsio/jetstream-controller:$NACK_VERSION"
)

pids=()
for image in "${images[@]}"; do
    echo "Preloading $image"
    load_apps "$image" &
    pids+=("$!")
done

failed=0
for index in "${!pids[@]}"; do
    pid="${pids[$index]}"
    image="${images[$index]}"
    if ! wait "$pid"; then
        echo "Failed to preload $image" >&2
        failed=1
    fi
done

if [[ "$failed" -ne 0 ]]; then
    exit 1
fi

# Install NATS and NACK
helm dependency update "$MISE_PROJECT_ROOT/local/nats"
helm install nats "$MISE_PROJECT_ROOT/local/nats" --wait -n "$NATS_NAMESPACE" -f "$MISE_PROJECT_ROOT/local/nats/values.yaml"
