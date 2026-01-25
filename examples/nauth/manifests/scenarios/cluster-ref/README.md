# Cluster-ref scenario

This scenario demonstrates the **cluster approach**: using `NatsCluster` and `natsClusterRef` instead of the legacy `NATS_URL` environment variable and label-based secrets.

## Overview

- **NatsCluster**: Defines a NATS cluster connection (URL, operator signing key, system account credentials). Supports `url` or `urlFrom` (ConfigMap/Secret).
- **Account** with `spec.natsClusterRef`: Targets a specific NatsCluster; the nauth controller uses it to resolve the provider.
- **User**: Lives in the same namespace as the Account; inherits the cluster via the account reference.

## Prerequisites

Create these secrets in the `cluster-ref-example` namespace before applying the manifests:

1. **Operator signing key** (`my-operator-signing-key`):
   - Key: `seed`
   - Value: The operator's NKey seed (base64-encoded)

2. **System account user credentials** (`my-system-account-creds`):
   - Key: `user.creds`
   - Value: The system account user JWT and seed (NATS user credentials file format)

3. **NATS URL**: The example uses a ConfigMap `nats-url-config` with `url: nats://nats.nats.svc.cluster.local:4222`. Adjust for your environment.

## Apply

```bash
kubectl apply -f cluster-ref.yaml
```

Ensure the operator signing key and system account credentials secrets exist beforehand, or the NatsCluster will not be usable.
