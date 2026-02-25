---
title: API Reference
description: API reference for nauth CRDs
---

## Packages
- [nauth.io/v1alpha1](#nauthiov1alpha1)

## nauth.io/v1alpha1

Package `v1alpha1` contains schema definitions for NAuth custom resources (see [Kubernetes API conventions](https://git.k8s.io/community/contributors/devel/sig-architecture/api-conventions.md)).

### Kubernetes Resource Conventions

All NAuth CRDs are standard Kubernetes resources and include:

- `apiVersion`: API group/version for the resource (for example `nauth.io/v1alpha1`)
- `kind`: resource type (for example `Account`, `User`, `NatsCluster`)
- `metadata`: Kubernetes object metadata (`name`, `namespace`, labels, annotations, etc.). See [Kubernetes `ObjectMeta`](https://kubernetes.io/docs/reference/generated/kubernetes-api/v1.32/#objectmeta-v1-meta)

### Resource Types
- [Account](#account)
- [AccountList](#accountlist)
- [User](#user)
- [UserList](#userlist)
- [NatsCluster](#natscluster)
- [NatsClusterList](#natsclusterlist)

## Account

`Account` is the schema for accounts.

| Field | Type | Required | Notes |
| --- | --- | --- | --- |
| `apiVersion` | string | Yes | `nauth.io/v1alpha1` |
| `kind` | string | Yes | `Account` |
| `metadata` | ObjectMeta | Yes | Kubernetes metadata |
| `spec` | [AccountSpec](#accountspec) | No | Desired state |
| `status` | [AccountStatus](#accountstatus) | No | Observed state |

### AccountSpec

| Field | Type | Required | Notes |
| --- | --- | --- | --- |
| `natsClusterRef` | [NatsClusterRef](#natsclusterref) | No | Explicit `NatsCluster` reference for reconciliation |
| `displayName` | string | No | Optional display name for the NATS account |
| `accountLimits` | [AccountLimits](#accountlimits) | No | Account limits |
| `exports` | [Export[]](#export) | No | Account exports |
| `imports` | [Import[]](#import) | No | Account imports |
| `jetStreamLimits` | [JetStreamLimits](#jetstreamlimits) | No | JetStream limits |
| `natsLimits` | [NatsLimits](#natslimits) | No | NATS limits |

### AccountStatus

| Field | Type | Required | Notes |
| --- | --- | --- | --- |
| `claims` | [AccountClaims](#accountclaims) | No | Effective account claims |
| `conditions` | `metav1.Condition[]` | No | Standard Kubernetes conditions |
| `observedGeneration` | int64 | No | Last observed generation |
| `reconcileTimestamp` | `metav1.Time` | No | Last reconcile timestamp |
| `signingKey` | [KeyInfo](#keyinfo) | No | Account signing key metadata |
| `operatorVersion` | string | No | Operator version that reconciled the resource |

### AccountClaims

| Field | Type |
| --- | --- |
| `displayName` | string |
| `accountLimits` | [AccountLimits](#accountlimits) |
| `exports` | [Export[]](#export) |
| `imports` | [Import[]](#import) |
| `jetStreamLimits` | [JetStreamLimits](#jetstreamlimits) |
| `natsLimits` | [NatsLimits](#natslimits) |

### AccountLimits

| Field | Type | Default |
| --- | --- | --- |
| `imports` | int64 | `-1` |
| `exports` | int64 | `-1` |
| `wildcards` | bool | `true` |
| `conn` | int64 | `-1` |
| `leaf` | int64 | `-1` |

### JetStreamLimits

| Field | Type | Default |
| --- | --- | --- |
| `memStorage` | int64 | `-1` |
| `diskStorage` | int64 | `-1` |
| `streams` | int64 | `-1` |
| `consumer` | int64 | `-1` |
| `maxAckPending` | int64 | `-1` |
| `memMaxStreamBytes` | int64 | `-1` |
| `diskMaxStreamBytes` | int64 | `-1` |
| `maxBytesRequired` | bool | `false` |

## User

`User` is the schema for users.

| Field | Type | Required | Notes |
| --- | --- | --- | --- |
| `apiVersion` | string | Yes | `nauth.io/v1alpha1` |
| `kind` | string | Yes | `User` |
| `metadata` | ObjectMeta | Yes | Kubernetes metadata |
| `spec` | [UserSpec](#userspec) | No | Desired state |
| `status` | [UserStatus](#userstatus) | No | Observed state |

### UserSpec

| Field | Type | Required | Notes |
| --- | --- | --- | --- |
| `accountName` | string | Yes | Account name reference |
| `displayName` | string | No | Optional display name for the NATS user |
| `permissions` | [Permissions](#permissions) | No | Publish/subscribe/response permissions |
| `userLimits` | [UserLimits](#userlimits) | No | User limits |
| `natsLimits` | [NatsLimits](#natslimits) | No | NATS limits |

### UserStatus

| Field | Type | Required | Notes |
| --- | --- | --- | --- |
| `claims` | [UserClaims](#userclaims) | No | Effective user claims |
| `conditions` | `metav1.Condition[]` | No | Standard Kubernetes conditions |
| `observedGeneration` | int64 | No | Last observed generation |
| `reconcileTimestamp` | `metav1.Time` | No | Last reconcile timestamp |
| `operatorVersion` | string | No | Operator version that reconciled the resource |

### UserClaims

| Field | Type | Notes |
| --- | --- | --- |
| `accountName` | string | Deprecated |
| `displayName` | string | Effective display name |
| `permissions` | [Permissions](#permissions) | Effective permissions |
| `natsLimits` | [NatsLimits](#natslimits) | Effective NATS limits |
| `userLimits` | [UserLimits](#userlimits) | Effective user limits |

## NatsCluster

`NatsCluster` is an information-bearing resource that defines NATS connection and secret references.

NAuth does not reconcile this resource and there is no status contract for it.

| Field | Type | Required | Notes |
| --- | --- | --- | --- |
| `apiVersion` | string | Yes | `nauth.io/v1alpha1` |
| `kind` | string | Yes | `NatsCluster` |
| `metadata` | ObjectMeta | Yes | Kubernetes metadata |
| `spec` | [NatsClusterSpec](#natsclusterspec) | No | Connection and secret references |

### NatsClusterSpec

Validation rule: exactly one of `url` or `urlFrom` must be specified.

| Field | Type | Required | Notes |
| --- | --- | --- | --- |
| `url` | string | Conditional | Direct NATS URL. Mutually exclusive with `urlFrom` |
| `urlFrom` | [URLFromReference](#urlfromreference) | Conditional | Source reference for URL. Mutually exclusive with `url` |
| `operatorSigningKeySecretRef` | [SecretKeyReference](#secretkeyreference) | Yes | Operator signing key secret ref |
| `systemAccountUserCredsSecretRef` | [SecretKeyReference](#secretkeyreference) | Yes | System account user creds secret ref |

## Shared Types

### NatsClusterRef

| Field | Type | Required | Notes |
| --- | --- | --- | --- |
| `name` | string | Yes | `NatsCluster` name |
| `namespace` | string | No | `NatsCluster` namespace |

### URLFromReference

| Field | Type | Required | Notes |
| --- | --- | --- | --- |
| `kind` | [URLFromKind](#urlfromkind) | Yes | `ConfigMap` or `Secret` |
| `name` | string | Yes | Source object name |
| `namespace` | string | No | Defaults to the `NatsCluster` namespace |
| `key` | string | Yes | Key containing the URL value |

### URLFromKind

Enum values:
- `ConfigMap`
- `Secret`

### SecretKeyReference

| Field | Type | Required |
| --- | --- | --- |
| `name` | string | Yes |
| `key` | string | No |

### NatsLimits

| Field | Type | Default |
| --- | --- | --- |
| `subs` | int64 | `-1` |
| `data` | int64 | `-1` |
| `payload` | int64 | `-1` |

### Permissions

| Field | Type |
| --- | --- |
| `pub` | [Permission](#permission) |
| `sub` | [Permission](#permission) |
| `resp` | [ResponsePermission](#responsepermission) |

### Permission

| Field | Type |
| --- | --- |
| `allow` | string[] |
| `deny` | string[] |

### ResponsePermission

| Field | Type |
| --- | --- |
| `max` | int |
| `ttl` | duration |

### UserLimits

| Field | Type | Notes |
| --- | --- | --- |
| `src` | string[] | CIDR allow list |
| `times` | [TimeRange[]](#timerange) | Allowed time windows |
| `timesLocation` | string | Timezone location |

### TimeRange

| Field | Type |
| --- | --- |
| `start` | string |
| `end` | string |

### Export

| Field | Type |
| --- | --- |
| `name` | string |
| `subject` | string |
| `type` | enum (`stream`, `service`) |
| `tokenReq` | bool |
| `revocations` | map[string]int64 |
| `responseType` | enum (`Singleton`, `Stream`, `Chunked`) |
| `responseThreshold` | duration |
| `serviceLatency` | [ServiceLatency](#servicelatency) |
| `accountTokenPosition` | uint |
| `advertise` | bool |
| `allowTrace` | bool |

### Import

| Field | Type |
| --- | --- |
| `accountRef` | [AccountRef](#accountref) |
| `name` | string |
| `subject` | string |
| `account` | string |
| `localSubject` | string |
| `type` | enum (`stream`, `service`) |
| `share` | bool |
| `allowTrace` | bool |

### AccountRef

| Field | Type | Required |
| --- | --- | --- |
| `name` | string | Yes |
| `namespace` | string | Yes |

### ServiceLatency

| Field | Type |
| --- | --- |
| `sampling` | int |
| `results` | string |

### KeyInfo

| Field | Type |
| --- | --- |
| `name` | string |
| `creationDate` | `metav1.Time` |
| `expirationDate` | `metav1.Time` |

## List Types

### AccountList
Contains a list of [Account](#account).

### UserList
Contains a list of [User](#user).

### NatsClusterList
Contains a list of [NatsCluster](#natscluster).
