---
title: Advanced Scenarios  
description: Advanced nauth configurations and use cases
---

This page covers advanced nauth configurations for complex production scenarios.

## Multi-Tenant SaaS Platform

For a SaaS platform with customer isolation:

```yaml
# Template for customer accounts
apiVersion: nauth.io/v1alpha1
kind: Account
metadata:
  name: customer-{{ customer-id }}
  namespace: customers
  labels:
    customer-id: "{{ customer-id }}"
    plan: "{{ subscription-plan }}"
spec:
  accountLimits:
    conn: "{{ plan-connection-limit }}"
    exports: 10
    imports: 5
  jetStreamLimits:
    memStorage: "{{ plan-memory }}"
    diskStorage: "{{ plan-disk }}"
    streams: "{{ plan-streams }}"
    consumer: "{{ plan-consumers }}"
  natsLimits:
    subs: "{{ plan-subscriptions }}"
    data: "{{ plan-data-limit }}"
    payload: 1048576
  imports:
    # Import shared services
    - name: "shared-analytics"
      accountRef:
        name: platform-services
        namespace: platform
      subject: "analytics.>"
      type: "service"
      localSubject: "platform.analytics.>"
---
# Customer application user
apiVersion: nauth.io/v1alpha1
kind: User
metadata:
  name: customer-{{ customer-id }}-app
  namespace: customers
spec:
  accountName: customer-{{ customer-id }}
  permissions:
    pub:
      allow: 
        - "app.{{ customer-id }}.>"
        - "platform.analytics.{{ customer-id }}.>"
      deny: 
        - "platform.>"  # Prevent cross-customer access
    sub:
      allow: 
        - "app.{{ customer-id }}.>"
        - "events.{{ customer-id }}.>"
      deny:
        - "events.*.internal.>"  # No internal events access
  userLimits:
    src: ["{{ customer-ip-ranges }}"]
```

## Event-Driven Architecture with Domain Separation

Complex microservices with domain boundaries:

```yaml
# User domain account
apiVersion: nauth.io/v1alpha1
kind: Account
metadata:
  name: user-domain
  namespace: platform
spec:
  accountLimits:
    conn: 300
    exports: 15
    imports: 10
  jetStreamLimits:
    memStorage: 209715200
    diskStorage: 2147483648
    streams: 20
    consumer: 100
  exports:
    - name: "user-commands"
      subject: "commands.users.>"
      type: "service"
      tokenReq: true
    - name: "user-events"
      subject: "events.users.>"
      type: "stream"
      tokenReq: false
    - name: "user-queries"
      subject: "queries.users.>"
      type: "service"
      tokenReq: true
  imports:
    - name: "notification-commands"
      accountRef:
        name: notification-domain
        namespace: platform
      subject: "commands.notifications.>"
      type: "service"
      localSubject: "external.notifications.>"
---
# Order domain account with user domain imports
apiVersion: nauth.io/v1alpha1
kind: Account
metadata:
  name: order-domain
  namespace: platform
spec:
  accountLimits:
    conn: 500
    exports: 20
    imports: 15
  jetStreamLimits:
    memStorage: 524288000
    diskStorage: 5368709120
    streams: 30
    consumer: 200
  exports:
    - name: "order-commands"
      subject: "commands.orders.>"
      type: "service"
      tokenReq: true
    - name: "order-events"
      subject: "events.orders.>"
      type: "stream"
  imports:
    - name: "user-queries"
      accountRef:
        name: user-domain
        namespace: platform
      subject: "queries.users.>"
      type: "service"
      localSubject: "users.>"
    - name: "user-events"
      accountRef:
        name: user-domain
        namespace: platform
      subject: "events.users.>"
      type: "stream"
      localSubject: "events.users.>"
```

## High-Security Environment

For environments requiring strict security controls:

```yaml
apiVersion: nauth.io/v1alpha1
kind: Account
metadata:
  name: secure-account
  namespace: secure
spec:
  accountLimits:
    conn: 50
    exports: 5
    imports: 3
    wildcards: false  # Disable wildcards
  jetStreamLimits:
    memStorage: 52428800
    diskStorage: 524288000
    streams: 5
    consumer: 20
    maxBytesRequired: true  # Require explicit byte limits
  natsLimits:
    subs: 100
    data: 1048576
    payload: 65536
---
apiVersion: nauth.io/v1alpha1
kind: User
metadata:
  name: secure-user
  namespace: secure
spec:
  accountName: secure-account
  permissions:
    pub:
      allow: 
        - "app.data.create"
        - "app.data.update"
        - "app.audit.log"
      deny: 
        - "app.data.delete"  # Explicit deny for destructive operations
    sub:
      allow: 
        - "app.data.responses"
        - "app.notifications"
      deny: 
        - "admin.>"
        - "system.>"
        - "debug.>"
    resp:
      max: 3       # Limit concurrent responses
      ttl: "10s"   # Short response TTL
  natsLimits:
    subs: 20
    data: 524288
    payload: 32768
  userLimits:
    src: ["10.0.1.0/24"]  # Specific secure subnet
    times:
      - start: "06:00:00"
        end: "22:00:00"
    timesLocation: "UTC"
```

## Global Multi-Region Setup

For applications distributed across regions:

```yaml
# US East region account
apiVersion: nauth.io/v1alpha1
kind: Account
metadata:
  name: app-us-east
  namespace: us-east-1
  labels:
    region: us-east-1
    app: global-app
spec:
  accountLimits:
    conn: 1000
    exports: 25
    imports: 25
  jetStreamLimits:
    memStorage: 1073741824
    diskStorage: 10737418240
    streams: 50
    consumer: 300
  exports:
    - name: "regional-data"
      subject: "data.us-east.>"
      type: "stream"
    - name: "regional-services"
      subject: "services.us-east.>"
      type: "service"
  imports:
    - name: "global-config"
      accountRef:
        name: global-services
        namespace: global
      subject: "config.>"
      type: "stream"
      localSubject: "global.config.>"
    - name: "eu-west-data"
      accountRef:
        name: app-eu-west
        namespace: eu-west-1
      subject: "data.eu-west.>"
      type: "stream"
      localSubject: "remote.eu-west.>"
---
# Regional user with global access patterns
apiVersion: nauth.io/v1alpha1
kind: User
metadata:
  name: app-us-east-user
  namespace: us-east-1
spec:
  accountName: app-us-east
  permissions:
    pub:
      allow: 
        - "data.us-east.>"
        - "services.us-east.>"
        - "events.us-east.>"
        - "replication.to-eu-west.>"
    sub:
      allow: 
        - "data.us-east.>"
        - "services.us-east.>"
        - "global.config.>"
        - "remote.eu-west.critical.>"  # Only critical data from other regions
  userLimits:
    src: ["10.1.0.0/16"]  # US East datacenter network
```

## DevOps and CI/CD Integration

For automated deployment pipelines:

```yaml
# CI/CD service account
apiVersion: nauth.io/v1alpha1
kind: Account
metadata:
  name: cicd-account
  namespace: cicd
spec:
  accountLimits:
    conn: 100
    exports: 10
    imports: 5
  natsLimits:
    subs: 200
    data: 5242880  # 5MB for build logs
    payload: 1048576
---
# Deployment automation user
apiVersion: nauth.io/v1alpha1
kind: User
metadata:
  name: deployment-bot
  namespace: cicd
spec:
  accountName: cicd-account
  permissions:
    pub:
      allow: 
        - "deployments.>"
        - "status.>"
        - "metrics.build.>"
        - "notifications.deploy.>"
    sub:
      allow: 
        - "deployments.status.>"
        - "health.>"
  userLimits:
    src: ["10.0.2.0/24", "10.0.3.0/24"]  # CI/CD subnets
    times:
      - start: "00:00:00"  # 24/7 access for automation
        end: "23:59:59"
---
# Developer testing user with time restrictions
apiVersion: nauth.io/v1alpha1
kind: User
metadata:
  name: dev-test-user
  namespace: cicd
spec:
  accountName: cicd-account
  permissions:
    pub:
      allow: 
        - "test.>"
        - "debug.>"
      deny: 
        - "deployments.production.>"
    sub:
      allow: 
        - "test.>"
        - "debug.>"
        - "status.development.>"
  userLimits:
    times:
      - start: "08:00:00"  # Business hours only
        end: "18:00:00"
    timesLocation: "America/New_York"
```

## Message Routing and Transformation

Complex routing between domains:

```yaml
# Message router account
apiVersion: nauth.io/v1alpha1
kind: Account
metadata:
  name: message-router
  namespace: integration
spec:
  accountLimits:
    conn: 200
    exports: 30
    imports: 30
  jetStreamLimits:
    memStorage: 524288000
    diskStorage: 5368709120
    streams: 100
    consumer: 500
  exports:
    - name: "transformed-events"
      subject: "transformed.>"
      type: "stream"
    - name: "routing-service"
      subject: "route.>"
      type: "service"
  imports:
    # Import from multiple source domains
    - name: "legacy-events"
      accountRef:
        name: legacy-system
        namespace: legacy
      subject: "legacy.events.>"
      type: "stream"
      localSubject: "source.legacy.>"
    - name: "modern-events"
      accountRef:
        name: modern-system
        namespace: platform
      subject: "events.>"
      type: "stream"
      localSubject: "source.modern.>"
---
# Router service user
apiVersion: nauth.io/v1alpha1
kind: User
metadata:
  name: message-router-svc
  namespace: integration
spec:
  accountName: message-router
  permissions:
    pub:
      allow: 
        - "transformed.>"
        - "route.responses.>"
        - "dlq.>"  # Dead letter queue
    sub:
      allow: 
        - "source.>"
        - "route.requests.>"
        - "$JS.API.>"  # JetStream management
  natsLimits:
    subs: 1000  # High subscription limit for routing
    data: 10485760
    payload: 2097152
```

## Disaster Recovery Setup

For high-availability scenarios:

```yaml
# Primary datacenter account
apiVersion: nauth.io/v1alpha1
kind: Account
metadata:
  name: app-primary
  namespace: production
  labels:
    datacenter: primary
    role: active
spec:
  accountLimits:
    conn: 2000
    exports: 50
    imports: 20
  jetStreamLimits:
    memStorage: 2147483648   # 2GB
    diskStorage: 21474836480 # 20GB
    streams: 100
    consumer: 1000
  exports:
    - name: "replication-stream"
      subject: "replication.>"
      type: "stream"
      tokenReq: true
  imports:
    - name: "dr-commands"
      accountRef:
        name: app-dr
        namespace: dr
      subject: "dr.commands.>"
      type: "service"
      localSubject: "dr.>"
---
# DR user with replication permissions
apiVersion: nauth.io/v1alpha1
kind: User
metadata:
  name: dr-replication-user
  namespace: production
spec:
  accountName: app-primary
  permissions:
    pub:
      allow: 
        - "replication.>"
        - "health.primary.>"
        - "dr.status.>"
    sub:
      allow: 
        - "app.>"  # Read all app data for replication
        - "dr.commands.>"
        - "$JS.API.STREAM.>"  # JetStream stream management
  natsLimits:
    subs: 500
    data: 52428800  # 50MB for large replication batches
    payload: 10485760  # 10MB payloads
```