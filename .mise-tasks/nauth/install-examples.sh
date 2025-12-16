#!/usr/bin/env bash

NAUTH_EXAMPLES_DIR="$MISE_PROJECT_ROOT/examples/nauth"

echo "Installing NAuth example scenarios from $NAUTH_EXAMPLES_DIR to local cluster..."

kubectl apply -f "$NAUTH_EXAMPLES_DIR/manifests/scenarios/" --recursive
