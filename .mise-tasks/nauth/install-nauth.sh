#!/usr/bin/env bash

NAUTH_CHART_DIR="$MISE_PROJECT_ROOT/charts/nauth"
NAUTH_LOCAL_DEV_DIR="$MISE_PROJECT_ROOT/local/nauth"

docker build -t local/nauth:test $MISE_PROJECT_ROOT
kind load docker-image local/nauth:test

echo "Installing NAUTH in namespace $NATS_NAMESPACE"
helm dependency update "$NAUTH_CHART_DIR"
helm install nauth "$NAUTH_CHART_DIR" --wait -n "$NATS_NAMESPACE" -f "$NAUTH_CHART_DIR/values.yaml" -f "$NAUTH_LOCAL_DEV_DIR/values.yaml"

kubectl apply -n "$NATS_NAMESPACE" -f "$NAUTH_LOCAL_DEV_DIR/manifests/operator.yaml"

echo "Waiting for NAUTH pods to become stable in namespace $NATS_NAMESPACE..."
if kubectl rollout status deployment nauth -n $NATS_NAMESPACE --timeout=300s; then
  echo "✅ NAUTH deployment succeeded"
else
  echo "❌ NAUTH deployment failed"
  kubectl get pods -n $NATS_NAMESPACE
  kubectl describe deployment nauth -n $NATS_NAMESPACE
  exit 1
fi