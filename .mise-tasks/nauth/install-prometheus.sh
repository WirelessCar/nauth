#!/usr/bin/env bash

# Add the Prometheus Helm repository
helm repo add prometheus-community https://prometheus-community.github.io/helm-charts

# Install Prometheus
helm dependency update "$MISE_PROJECT_ROOT/examples/prometheus"
helm install prometheus "$MISE_PROJECT_ROOT/examples/prometheus" -n monitoring --create-namespace -f "$MISE_PROJECT_ROOT/examples/prometheus/values.yaml"

echo "Waiting for Prometheus to become stable..."
sleep 5
while true; do
  pod_phase=$(kubectl get pods -n "$NATS_NAMESPACE" | grep -v "NAME\|Running" | wc -l)
  if [[ $pod_phase -eq 0 ]] ; then
    echo "Prometheus are running"
    break
  else
    echo "Waiting for Prometheus to become stable..."
    sleep 5
  fi
done
