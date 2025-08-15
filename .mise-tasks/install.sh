#!/usr/bin/env bash
# MISE description="Initialize a local Kubernetes cluster with NATS, NACK, NAUTH and deploy the service"

set -euo pipefail

kind delete cluster
kind create cluster

# Create the NATS namespace
kubectl create namespace "$NATS_NAMESPACE"

# Uncomment if running the stack locally with observability
# $MISE_PROJECT_ROOT/.mise-tasks/install-prometheus.sh
$MISE_PROJECT_ROOT/.mise-tasks/install-nats.sh
$MISE_PROJECT_ROOT/.mise-tasks/install-nauth.sh

