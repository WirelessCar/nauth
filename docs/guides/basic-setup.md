---
title: Basic Setup Examples
description: Basic setup scenarios for nauth
---

This page provides practical examples of common nauth setups for different use cases.

## Single Application Setup

Perfect for getting started with a simple application:

```yaml
# Create an account for your application
apiVersion: nauth.io/v1alpha1
kind: Account
metadata:
  name: my-app-account
  namespace: default
spec:
  accountLimits:
    conn: 50
    exports: 5
    imports: 5
  natsLimits:
    subs: 100
    data: 1048576
    payload: 65536
---
# Create a user for the application
apiVersion: nauth.io/v1alpha1
kind: User
metadata:
  name: my-app-user
  namespace: default
spec:
  accountName: my-app-account
  permissions:
    pub:
      allow: ["app.>"]
    sub:
      allow: ["app.>"]
  natsLimits:
    subs: 50
    data: 524288
    payload: 32768
```

Deploy your application with credentials:

```yaml
apiVersion: apps/v1
kind: Deployment
metadata:
  name: my-app
spec:
  replicas: 1
  selector:
    matchLabels:
      app: my-app
  template:
    metadata:
      labels:
        app: my-app
    spec:
      containers:
      - name: app
        image: my-app:latest
        env:
        - name: NATS_URL
          value: "nats://nats.nats-system:4222"
        volumeMounts:
        - name: nats-creds
          mountPath: /etc/nats-creds
          readOnly: true
      volumes:
      - name: nats-creds
        secret:
          secretName: user-my-app-user-creds
```

## Multi-Service Architecture

For microservices that need to communicate:

```yaml
# Shared services account
apiVersion: nauth.io/v1alpha1
kind: Account
metadata:
  name: services-account
  namespace: production
spec:
  accountLimits:
    conn: 200
    exports: 20
    imports: 20
  jetStreamLimits:
    memStorage: 52428800   # 50MB
    diskStorage: 524288000 # 500MB
    streams: 10
    consumer: 50
  exports:
    - name: "user-service"
      subject: "services.users.>"
      type: "service"
    - name: "order-events"
      subject: "events.orders.>"
      type: "stream"
---
# User service user
apiVersion: nauth.io/v1alpha1
kind: User
metadata:
  name: user-service
  namespace: production
spec:
  accountName: services-account
  permissions:
    pub:
      allow: ["services.users.>", "events.users.>"]
    sub:
      allow: ["services.users.requests.>"]
---
# Order service user  
apiVersion: nauth.io/v1alpha1
kind: User
metadata:
  name: order-service
  namespace: production
spec:
  accountName: services-account
  permissions:
    pub:
      allow: ["services.orders.>", "events.orders.>"]
    sub:
      allow: ["services.orders.requests.>", "events.users.>"]
```

## Development vs Production

### Development Environment

```yaml
apiVersion: nauth.io/v1alpha1
kind: Account
metadata:
  name: dev-account
  namespace: development
spec:
  accountLimits:
    conn: 20
    exports: 5
    imports: 5
  natsLimits:
    subs: 50
    data: 524288
    payload: 32768
---
apiVersion: nauth.io/v1alpha1
kind: User
metadata:
  name: dev-user
  namespace: development
spec:
  accountName: dev-account
  permissions:
    pub:
      allow: [">"]  # Allow everything in dev
    sub:
      allow: [">"]
```

### Production Environment

```yaml
apiVersion: nauth.io/v1alpha1
kind: Account
metadata:
  name: prod-account
  namespace: production
spec:
  accountLimits:
    conn: 1000
    exports: 50
    imports: 30
  jetStreamLimits:
    memStorage: 1073741824  # 1GB
    diskStorage: 10737418240 # 10GB
    streams: 100
    consumer: 500
  natsLimits:
    subs: 5000
    data: 10485760
    payload: 1048576
---
apiVersion: nauth.io/v1alpha1
kind: User
metadata:
  name: prod-app-user
  namespace: production
spec:
  accountName: prod-account
  permissions:
    pub:
      allow: ["app.>", "events.app.>"]
      deny: ["admin.>", "system.>"]
    sub:
      allow: ["app.>", "events.>"]
      deny: ["admin.>", "system.>", "internal.>"]
  userLimits:
    src: ["10.0.0.0/8", "172.16.0.0/12"]
```

## JetStream Enabled Application

For applications using NATS JetStream:

```yaml
apiVersion: nauth.io/v1alpha1
kind: Account
metadata:
  name: jetstream-app
  namespace: default
spec:
  accountLimits:
    conn: 100
  jetStreamLimits:
    memStorage: 104857600   # 100MB
    diskStorage: 1073741824 # 1GB
    streams: 10
    consumer: 50
    maxAckPending: 1000
  natsLimits:
    subs: 200
    data: 2097152
    payload: 131072
---
apiVersion: nauth.io/v1alpha1
kind: User
metadata:
  name: jetstream-user
  namespace: default
spec:
  accountName: jetstream-app
  permissions:
    pub:
      allow: ["data.>", "events.>"]
    sub:
      allow: ["data.>", "events.>", "$JS.API.>"]
  natsLimits:
    subs: 100
    data: 1048576
    payload: 65536
```

## Cross-Account Communication

Service provider account:

```yaml
apiVersion: nauth.io/v1alpha1
kind: Account
metadata:
  name: api-provider
  namespace: default
spec:
  accountLimits:
    conn: 100
    exports: 10
  exports:
    - name: "public-api"
      subject: "api.public.>"
      type: "service"
      tokenReq: false
    - name: "authenticated-api"
      subject: "api.auth.>"
      type: "service"
      tokenReq: true
```

Service consumer account:

```yaml
apiVersion: nauth.io/v1alpha1
kind: Account
metadata:
  name: api-consumer
  namespace: default
spec:
  accountLimits:
    conn: 50
    imports: 5
  imports:
    - name: "public-api"
      accountRef:
        name: api-provider
        namespace: default
      subject: "api.public.>"
      type: "service"
      localSubject: "external.api.>"
---
apiVersion: nauth.io/v1alpha1
kind: User
metadata:
  name: consumer-user
  namespace: default
spec:
  accountName: api-consumer
  permissions:
    pub:
      allow: ["external.api.>", "app.>"]
    sub:
      allow: ["app.>"]
```

## Time-Restricted Access

For business-hours applications:

```yaml
apiVersion: nauth.io/v1alpha1
kind: User
metadata:
  name: business-user
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
      - start: "08:00:00"
        end: "18:00:00"
      - start: "13:00:00"    # Lunch break gap
        end: "14:00:00"
    timesLocation: "America/New_York"
```

## Monitoring and Observability

Account with monitoring permissions:

```yaml
apiVersion: nauth.io/v1alpha1
kind: User
metadata:
  name: monitoring-user
  namespace: default
spec:
  accountName: my-account
  permissions:
    pub:
      deny: ["*"]  # Read-only user
    sub:
      allow: ["metrics.>", "health.>", "status.>"]
  natsLimits:
    subs: 500  # Higher subscription limit for monitoring
    data: 2097152
    payload: 65536
```