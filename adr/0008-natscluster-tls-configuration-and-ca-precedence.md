# ADR-8: Configure NATS TLS per NatsCluster with an operator-wide CA fallback

Date: 2026-11-24

## Problem statement

NAuth must connect to NATS clusters that use a private CA, a TLS-first handshake, or mutual TLS. This ADR decides who owns that configuration and which CA bundle NAuth trusts for a cluster.

## Status

Accepted

## Context

Issue [#433](https://github.com/WirelessCar/nauth/issues/433) asks for TLS support. Before this change NAuth could only verify a `tls://` server against the system roots. It had no way to trust a private CA, use a TLS-first handshake, or present a client certificate.

A `NatsCluster` already owns the connection URL and the system account credentials, both as references to objects in its own namespace. TLS settings apply to every connection NAuth makes to that cluster.

Many deployments use one internal CA for all their NATS clusters. Client certificates and the handshake mode differ from cluster to cluster.

## Options

### Option 1 - Configure TLS only on the operator

Configure the CA, client certificate, and handshake mode once, through the operator's deployment.

- **Pros:** One place to configure; matches deployments with a single shared CA.
- **Cons:** Cannot express per-cluster client certificates or handshake modes. Every TLS change needs an operator redeploy. Credentials for all clusters sit in the operator's namespace instead of next to each cluster.

### Option 2 - Configure TLS only on each NatsCluster

Every `NatsCluster` carries its full TLS configuration, including its CA bundle.

- **Pros:** Each cluster is self-contained; TLS material lives in the same namespace as the cluster's other credentials.
- **Cons:** A shared internal CA must be copied into every cluster's namespace and kept in sync during rotation.

### Option 3 - Configure TLS per NatsCluster with an operator-wide CA fallback

`NatsCluster` owns its TLS configuration. The operator supplies a CA bundle that clusters without their own CA fall back to.

- **Pros:** A shared CA is configured once; client certificates and handshake modes stay per cluster.
- **Cons:** Two CA sources, so users need a clear rule for which one applies.

### CA source selection: merge or select one

When more than one CA source is available, NAuth can either merge them into one trust pool or trust only the most specific one.

- **Merge:** Convenient, since a cluster that adds its own CA keeps trusting the operator CA and the system roots. But a cluster owner cannot narrow trust: any CA trusted by the operator or the host can issue a certificate the cluster accepts, so the trust boundary is the union of every source.
- **Select one:** The cluster owner controls exactly which CA certificates can authenticate the server. Users who expect merging may be caught out, and a cluster that needs both a private and a public CA must put both in its own bundle.

## Decision

NAuth uses Option 3, with exclusive CA selection.

- `NatsCluster.spec.tls` owns the cluster's TLS configuration: a CA bundle reference, a client certificate reference for mutual TLS, and the TLS-first handshake mode.
- TLS references point to Secrets or ConfigMaps in the `NatsCluster` namespace, like the system account credentials.
- The operator provides one CA bundle, set through the Helm chart, as a fallback for clusters that request TLS without their own CA.
- CA precedence is the cluster's CA, then the operator-wide CA, then the system roots. NAuth trusts only the first source that is configured and does not merge them.
- NAuth reads TLS material when it connects, so rotated certificates and CA bundles take effect without restarting the operator.
- For connections using TLS, NAuth requires TLS 1.2 or later and negotiates TLS 1.3 when the server supports it. This follows [NIST SP 800-52r2](https://nvlpubs.nist.gov/nistpubs/SpecialPublications/NIST.SP.800-52r2.pdf), which requires TLS 1.2 support and disallows earlier versions, and matches the range Go permits in FIPS 140-3 mode, so the standard and FIPS images accept the same versions. The floor is set explicitly so it does not depend on library defaults.

## Consequences

- A shared internal CA is configured once for the operator; per-cluster client certificates and handshake modes live on each `NatsCluster`.
- Setting a CA on a cluster stops NAuth from trusting the operator-wide CA and the system roots for that cluster. The CRD field description and the TLS guide both state this.
- A bundle may hold several CA certificates, so old and new CA certificates can overlap during rotation.
- Connection failures reported on the `NatsCluster` name the CA source NAuth trusted, so users can see which bundle applied.
- Periodic `NatsCluster` revalidation, for example to detect an expired certificate between reconciles, remains out of scope ([#376](https://github.com/WirelessCar/nauth/issues/376)).
