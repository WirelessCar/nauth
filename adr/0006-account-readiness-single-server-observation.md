# ADR-6: Use a single NATS server observation for Account readiness

Date: 2026-09-21

## Problem statement

NAuth must verify that an Account is complete in NATS after publishing its JWT.
A NATS URL can connect to any server in a large cluster, while querying every server on every reconciliation is unnecessarily expensive.

## Status

Accepted

## Context

Issue [#359](https://github.com/WirelessCar/nauth/issues/359) exposes a false positive: NATS can accept an Account JWT while reporting the Account as incomplete because an import is invalid.

NATS cluster state is expected to converge, but NAuth cannot cheaply prove that all servers agree.
Server identities are also transient in rolling and recreated deployments.

### Compatibility note

As of 2026-09-21, NATS server-targeted `ACCOUNTZ` is available from NATS v2.2.0.
NATS 2.1.x and earlier cannot provide this `ACCOUNTZ`-based completeness observation.
The implementation treats NATS 2.2+ as supported; this is a capability boundary, not a hard NAuth deployment requirement.

Servers outside the supported boundary cannot provide full Account completeness validation, so `Ready` may remain `Unknown` rather than becoming `True` from JWT upload success alone.

## Options

### Option 1 - Observe one connected server

Query the server reached by the NATS connection with an identity-matched ACCOUNTZ request and treat the result as representative of the cluster.

### Option 2 - Observe multiple or all servers

Collect several ACCOUNTZ responses and define a confidence threshold.
This requires server discovery, threshold configuration, larger responses, and additional reconciliation cost.

## Decision

Use Option 1.

NAuth observes one connected NATS server and treats that observation as representative of the cluster.
It does not query all servers or apply a multi-server threshold.

## Consequences

- Account readiness catches the reported JWT-upload false positive.
- Validation remains bounded and works when servers are rolled or recreated.
- Temporary disagreement between NATS servers is not detected by this level; readiness reflects the server observed at validation time.
- NATS versions without the required server-targeted ACCOUNTZ APIs cannot provide full Account completeness validation; in that case, `Ready` may remain `Unknown` instead of becoming `True` from JWT upload success alone.
