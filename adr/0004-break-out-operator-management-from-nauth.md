# ADR-4: Break out Operator management from Nauth

Date: 2025-08-13

## Problem statement
The life cycle and personas that are involved with Accounts & Users differ vastly from that of the Operator. The
practical & security process of these need to reflect this.

## Status

Accepted

Supersedes [ADR-1: Deploy controller handling NATS operator as separate Kubernetes deployment](0001-nauth-design-for-key-isolation.md)

## Context

The operator is a very sensitive key, which reassembles a Root CA since it is the single source of trust within a NATS
(super) cluster. Nauth needs at least the signing key and a system user in order to manage accounts.

## Options
The options stay the same as for ADR-1

### Operator management is broken out of Nauth and provided as secrets
Since the operator is something handled by the platform engineer persona and inherently does not allow more than one in
a cluster it is natural that the management is moved outside of NAuth.
The process could instead be an offline process similar of how a Root CA would be handled. This could even be done on a
dedicated computer with no internet access.
This eliminates the access to the operator seed and only gives NAuth access to the signing key - which can be rotated
without too much consequence.

## Decision
Break out the Operator management to a separate manual process.

## Consequences
- The NATS cluster will be much more secure, especially in a bigger cluster.
- It will become more intuitive how to work with the Operator
- A separate process needs to be done, involving other tools such as `nsc` or custom built
- There is no automatic creation of an Operator in NAuth
