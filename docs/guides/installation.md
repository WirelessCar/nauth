---
title: Installing nauth
description: Complete installation guide for nauth operator
---

## Helm Installation

The recommended way to install nauth is using Helm:

```bash
# Add the nauth Helm repository
helm repo add nauth https://wirelesscar.github.io/nauth

# Update your local Helm chart repository cache
helm repo update

# Install nauth in the nauth-system namespace
helm install nauth nauth/nauth --create-namespace --namespace nauth-system
```

## Prerequisites

Before installing nauth, you need:

### 1. Running NATS Cluster

You need a properly secured NATS cluster with:
- An operator account
- A system account
- Proper JWT authentication configured

If you don't have this setup, use the [operator-bootstrap](https://github.com/wirelesscar/nauth/tree/main/operator-bootstrap) utility included with nauth.

### 2. Kubernetes Requirements

- Kubernetes cluster version 1.19+
- RBAC enabled
- Ability to create custom resources

## Configuration Options

### Basic Configuration

```yaml
# values.yaml
image:
  repository: nauth
  tag: latest
  
resources:
  limits:
    cpu: 500m
    memory: 128Mi
  requests:
    cpu: 100m
    memory: 64Mi
```

### NATS Connection Configuration

```yaml
# values.yaml
nats:
  operatorJWT: "your-operator-jwt"
  systemAccount: "your-system-account"
  endpoint: "nats://nats.nats-system:4222"
```

## Verification

After installation, verify that nauth is running:

```bash
# Check if the operator is running
kubectl get pods -n nauth-system

# Verify CRDs are installed
kubectl get crd | grep nauth.io

# Check logs
kubectl logs -n nauth-system deployment/nauth-controller-manager
```

You should see:
- The `nauth-controller-manager` pod in `Running` state
- CRDs for `accounts.nauth.io` and `users.nauth.io`
- No error messages in the logs

## Troubleshooting

### Common Issues

**Pod not starting:**
```bash
kubectl describe pod -n nauth-system -l app.kubernetes.io/name=nauth
```

**CRDs not found:**
```bash
kubectl apply -f https://raw.githubusercontent.com/wirelesscar/nauth/main/charts/nauth/crds/
```

**RBAC issues:**
Make sure your cluster has RBAC enabled and the service account has proper permissions.