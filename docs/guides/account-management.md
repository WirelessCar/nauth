---
title: Account Management
description: Managing NATS accounts with nauth
---

NATS accounts provide multi-tenancy by isolating streams, services, and data between different applications or teams. The nauth operator simplifies account management through Kubernetes Custom Resources.

## Account CRD Overview

The `Account` CRD allows you to declaratively manage NATS accounts:

```yaml
apiVersion: nauth.io/v1alpha1
kind: Account
metadata:
  name: team-alpha
  namespace: default
spec:
  accountLimits:
    conn: 100        # Maximum connections
    exports: 10      # Maximum exports
    imports: 10      # Maximum imports
    wildcards: true  # Allow wildcard subjects
  jetStreamLimits:
    memStorage: 1048576    # 1MB memory storage
    diskStorage: 10485760  # 10MB disk storage
    streams: 5             # Maximum streams
    consumer: 10           # Maximum consumers
  natsLimits:
    subs: 1000       # Maximum subscriptions
    data: 1048576    # Maximum data (1MB)
    payload: 65536   # Maximum payload (64KB)
```

## Creating Accounts

### Basic Account

```yaml
apiVersion: nauth.io/v1alpha1
kind: Account
metadata:
  name: simple-account
  namespace: default
spec:
  accountLimits:
    conn: 50
```

### Production Account with JetStream

```yaml
apiVersion: nauth.io/v1alpha1
kind: Account
metadata:
  name: production-account
  namespace: production
spec:
  accountLimits:
    conn: 1000
    exports: 20
    imports: 15
  jetStreamLimits:
    memStorage: 104857600   # 100MB
    diskStorage: 1073741824 # 1GB
    streams: 50
    consumer: 100
  natsLimits:
    subs: 10000
    data: 10485760
    payload: 1048576
```

## Account Exports and Imports

### Exporting Services

```yaml
apiVersion: nauth.io/v1alpha1
kind: Account
metadata:
  name: service-provider
spec:
  exports:
    - name: "user-service"
      subject: "api.users.>"
      type: "service"
      tokenReq: true
```

### Importing Services

```yaml
apiVersion: nauth.io/v1alpha1
kind: Account
metadata:
  name: service-consumer
spec:
  imports:
    - name: "user-service"
      accountRef:
        name: service-provider
        namespace: default
      subject: "api.users.>"
      type: "service"
      localSubject: "users.>"
```

## Account Status

Check account status to see the actual claims and conditions:

```bash
kubectl get accounts -o yaml
```

The status section shows:
- **claims**: The actual NATS account claims
- **conditions**: Current state and any errors
- **signingKey**: Information about the account signing key
- **observedGeneration**: Last processed generation
- **reconcileTimestamp**: Last reconciliation time

## Best Practices

### Resource Limits
- Set appropriate connection limits based on expected load
- Configure JetStream limits for applications using streams
- Use conservative limits initially and adjust based on monitoring

### Namespace Organization
- Use Kubernetes namespaces to organize accounts by team or environment
- Consider one account per application or microservice
- Group related accounts in the same namespace

### Security Considerations
- Enable `tokenReq` for sensitive exports
- Use specific subject patterns instead of wildcards when possible
- Regularly review and audit account configurations

## Troubleshooting

### Account Not Created
```bash
kubectl describe account my-account
```

Check the conditions section for error messages.

### Connection Issues
- Verify account limits haven't been exceeded
- Check if the NATS server recognizes the account
- Ensure proper JWT signing is working

### Export/Import Problems
- Verify account references are correct
- Check subject pattern matching
- Ensure token requirements are met