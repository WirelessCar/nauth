# ADR-9: Require TLS when a NatsCluster sets `spec.tls`

Date: 2026-11-24

## Problem statement

A `NatsCluster` can set `spec.tls` while its URL uses the `nats://` scheme, which by itself does not demand TLS. This ADR decides whether a present `spec.tls` is a requirement for verified TLS or only a set of settings applied if TLS happens to be used.

## Status

Accepted

## Context

NAuth connects to each cluster with the system account credentials. Those credentials can push Account JWTs for every account under the operator, so exposing them to an eavesdropper or a server impersonator compromises the whole operator.

`spec.tls` is optional, and every field inside it is optional, so `tls: {}` is a valid configuration. A user who adds the block has chosen to configure TLS for the cluster, even when they set no fields.

Existing `NatsCluster` resources without `spec.tls` must keep working as they do today.

## Options

### Option 1 - A present `spec.tls` requires verified TLS

Any `spec.tls`, including an empty block, makes TLS mandatory. NAuth verifies the server certificate and refuses to connect when the server does not offer TLS.

- **Pros:** The configuration means what it says: a user who configures TLS gets TLS. The system account credentials never travel in plaintext to a cluster that sets the block. A misconfigured server shows up as a connection failure instead of a silent downgrade.
- **Cons:** A cluster that sets `spec.tls` against a server without TLS stops connecting until one side is fixed. Users must remove the block, not leave it empty, to opt out.

### Option 2 - `spec.tls` holds connection settings only

The block supplies a CA, client certificate, and handshake mode, but whether TLS is used is left to the URL scheme and to what the server offers.

- **Pros:** Adding the block never changes whether a connection succeeds; it only changes how TLS behaves when TLS is used.
- **Cons:** A `nats://` URL with `spec.tls` can still connect in plaintext if the server does not offer TLS, so the system account credentials can be sent unprotected despite explicit TLS configuration. The security of the connection depends on server behavior that NAuth does not control, and a downgrade gives no signal to the user.

## Decision

NAuth uses Option 1.

- A present `spec.tls`, including `tls: {}`, requires TLS. NAuth fails the connection when the server does not offer TLS and never falls back to plaintext.
- NAuth always verifies the server certificate, using the CA source chosen by [ADR-8](0008-natscluster-tls-configuration-and-ca-precedence.md).
- Without `spec.tls`, behavior is unchanged: the URL scheme alone decides whether TLS is required.

## Consequences

- Existing `NatsCluster` resources without `spec.tls` keep their current behavior.
- `tls: {}` is the minimal way to require verified TLS on a `nats://` URL.
- A cluster that sets `spec.tls` against a server without TLS does not connect; the `NatsCluster` `Ready` condition reports the TLS mismatch.
- System account credentials are never sent in plaintext to a cluster that sets `spec.tls`.
