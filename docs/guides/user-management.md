---
title: User Management
description: Managing NATS users and credentials with nauth
---

Users in NATS provide authentication and authorization for applications to connect to accounts. The nauth operator manages user creation, credential delivery, and permission management through Kubernetes.

## User CRD Overview

The `User` CRD creates NATS users within accounts:

```yaml
apiVersion: nauth.io/v1alpha1
kind: User
metadata:
  name: app-user
  namespace: default
spec:
  accountName: my-account
  permissions:
    pub:
      allow: ["app.>", "events.user.*"]
      deny: ["admin.>"]
    sub:
      allow: ["app.>", "responses.>"]
      deny: ["internal.>"]
    resp:
      max: 10
      ttl: "5s"
  natsLimits:
    subs: 100
    data: 1048576
    payload: 65536
  userLimits:
    src: ["192.168.1.0/24", "10.0.0.0/8"]
    times:
      - start: "08:00:00"
        end: "18:00:00"
    timesLocation: "UTC"
```

## Creating Users

### Basic User

```yaml
apiVersion: nauth.io/v1alpha1
kind: User
metadata:
  name: simple-user
  namespace: default
spec:
  accountName: simple-account
  permissions:
    pub:
      allow: ["data.>"]
    sub:
      allow: ["data.>"]
```

### Service User with Restricted Permissions

```yaml
apiVersion: nauth.io/v1alpha1
kind: User
metadata:
  name: microservice-user
  namespace: production
spec:
  accountName: production-account
  permissions:
    pub:
      allow: 
        - "service.orders.>"
        - "events.order.*"
      deny: 
        - "admin.>"
        - "system.>"
    sub:
      allow: 
        - "service.orders.requests.>"
        - "events.payment.*"
      deny: 
        - "events.internal.>"
    resp:
      max: 5
      ttl: "30s"
  natsLimits:
    subs: 50
    data: 2097152  # 2MB
    payload: 131072 # 128KB
```

### Time and Location Restricted User

```yaml
apiVersion: nauth.io/v1alpha1
kind: User
metadata:
  name: business-hours-user
  namespace: default
spec:
  accountName: my-account
  permissions:
    pub:
      allow: ["business.>"]
    sub:
      allow: ["business.>"]
  userLimits:
    src: ["192.168.1.0/24"]  # Office network only
    times:
      - start: "09:00:00"
        end: "17:00:00"
    timesLocation: "America/New_York"
```

## Permission Patterns

### Subject Patterns
- `foo`: Exact subject match
- `foo.*`: Single token wildcard
- `foo.>`: Multi-token wildcard  
- `foo.bar.*`: Specific prefix with wildcard

### Common Permission Sets

**Read-Only User:**
```yaml
permissions:
  pub:
    deny: ["*"]
  sub:
    allow: ["data.>", "events.>"]
```

**Service-to-Service:**
```yaml
permissions:
  pub:
    allow: ["requests.myservice.>"]
  sub:
    allow: ["responses.myservice.>", "events.>"]
  resp:
    max: 10
    ttl: "10s"
```

**Admin User:**
```yaml
permissions:
  pub:
    allow: [">"]
    deny: ["system.admin.delete.*"]
  sub:
    allow: [">"]
```

## Credential Management

### Automatic Secret Creation

When a user is created, nauth automatically generates:
- User JWT credentials
- Private key for signing
- Connection information

These are stored in a Kubernetes Secret:

```bash
kubectl get secret user-<username>-creds -o yaml
```

### Using Credentials in Applications

Mount the secret in your application:

```yaml
apiVersion: apps/v1
kind: Deployment
metadata:
  name: my-app
spec:
  template:
    spec:
      containers:
      - name: app
        image: my-app:latest
        env:
        - name: NATS_USER_JWT
          valueFrom:
            secretKeyRef:
              name: user-app-user-creds
              key: user.jwt
        - name: NATS_USER_SEED
          valueFrom:
            secretKeyRef:
              name: user-app-user-creds
              key: user.seed
        volumeMounts:
        - name: nats-creds
          mountPath: /etc/nats
          readOnly: true
      volumes:
      - name: nats-creds
        secret:
          secretName: user-app-user-creds
```

## User Status and Monitoring

Check user status:

```bash
kubectl get users -o wide
kubectl describe user my-user
```

The status shows:
- **claims**: The actual NATS user claims
- **conditions**: Current state and any errors
- **observedGeneration**: Last processed generation
- **reconcileTimestamp**: Last reconciliation time

## Best Practices

### Security
- Use principle of least privilege for permissions
- Implement IP restrictions for sensitive users
- Set appropriate time restrictions for business applications
- Regularly rotate user credentials

### Organization
- Create users in the same namespace as their account
- Use descriptive names that indicate purpose
- Group related users by application or team

### Limits
- Set conservative resource limits initially
- Monitor actual usage and adjust accordingly
- Consider payload sizes for your specific use case

## Troubleshooting

### User Creation Fails
```bash
kubectl describe user my-user
```

Common issues:
- Referenced account doesn't exist
- Invalid permission patterns
- Resource limit conflicts

### Connection Issues
- Verify user credentials are properly mounted
- Check if account limits are exceeded
- Ensure NATS server is accessible

### Permission Denied
- Review allow/deny patterns for conflicts
- Check subject pattern matching
- Verify account-level exports/imports