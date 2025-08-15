---
title: Quick Start
description: Get up and running with nauth in 5 minutes
---

Get nauth up and running in your Kubernetes cluster in just a few minutes.

## Prerequisites

- Kubernetes cluster (1.19+)
- Helm 3.x
- `kubectl` configured

## 1. Install nauth

```bash
# Add Helm repository
helm repo add nauth https://wirelesscar.github.io/nauth

# Install nauth
helm install nauth nauth/nauth --create-namespace --namespace nauth-system
```

## 2. Verify Installation

```bash
# Check pods are running
kubectl get pods -n nauth-system

# Verify CRDs are installed
kubectl get crd | grep nauth.io
```

## 3. Create Your First Account

```bash
cat <<EOF | kubectl apply -f -
apiVersion: nauth.io/v1alpha1
kind: Account
metadata:
  name: quickstart-account
  namespace: default
spec:
  accountLimits:
    conn: 100
  natsLimits:
    subs: 1000
    data: 1048576
    payload: 65536
EOF
```

## 4. Create a User

```bash
cat <<EOF | kubectl apply -f -
apiVersion: nauth.io/v1alpha1
kind: User
metadata:
  name: quickstart-user
  namespace: default
spec:
  accountName: quickstart-account
  permissions:
    pub:
      allow: ["quickstart.>"]
    sub:
      allow: ["quickstart.>"]
EOF
```

## 5. Test Your Setup

```bash
# Check account status
kubectl get account quickstart-account

# Check user credentials were created
kubectl get secret user-quickstart-user-creds
```